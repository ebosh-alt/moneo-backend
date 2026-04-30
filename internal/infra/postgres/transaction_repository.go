package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appaccounting "moneo/internal/app/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionRepository struct {
	pool *pgxpool.Pool
}

func NewTransactionRepository(pool *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{pool: pool}
}

func (r *TransactionRepository) Create(ctx context.Context, transaction domaintransactions.Transaction) error {
	const query = `
INSERT INTO transactions (
	id,
	user_id,
	type,
	status,
	amount_minor,
	currency,
	occurred_at,
	planned_at,
	account_from_id,
	account_to_id,
	category_id,
	subcategory_id,
	budget_member_id,
	income_source_id,
	debt_id,
	goal_id,
	investment_id,
	recurring_payment_id,
	comment,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(
		ctx,
		query,
		string(transaction.ID()),
		string(transaction.UserID()),
		string(transaction.Type()),
		string(transaction.Status()),
		transaction.Amount().MinorUnits(),
		transaction.Amount().Currency().String(),
		transaction.OccurredAt(),
		transaction.PlannedAt(),
		nullableAccountID(transaction.AccountFromID()),
		nullableAccountID(transaction.AccountToID()),
		nullableCategoryID(transaction.CategoryID()),
		nullableSubcategoryID(transaction.SubcategoryID()),
		nil, // budget_member_id (reserved for MVP2)
		nullableIncomeSourceID(transaction.IncomeSourceID()),
		nil, // debt_id (reserved for MVP2)
		nil, // goal_id (reserved for MVP2)
		nil, // investment_id (reserved for MVP2)
		nil, // recurring_payment_id (reserved for MVP2)
		transaction.Comment(),
		transaction.CreatedAt(),
		transaction.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	return nil
}

func (r *TransactionRepository) FindByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	type,
	status,
	amount_minor,
	currency,
	occurred_at,
	planned_at,
	account_from_id::text,
	account_to_id::text,
	category_id::text,
	subcategory_id::text,
	income_source_id::text,
	comment,
	created_at,
	updated_at
FROM transactions
WHERE id = $1
  AND user_id = $2
LIMIT 1
`

	db := databaseFromContext(ctx, r.pool)
	transaction, err := scanTransaction(db.QueryRow(ctx, query, string(transactionID), string(userID)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
		}
		return domaintransactions.Transaction{}, fmt.Errorf("select transaction by id: %w", err)
	}

	return transaction, nil
}

func (r *TransactionRepository) ListByUserID(
	ctx context.Context,
	input appaccounting.ListTransactionsQuery,
) ([]domaintransactions.Transaction, error) {
	query := `
SELECT
	id::text,
	user_id::text,
	type,
	status,
	amount_minor,
	currency,
	occurred_at,
	planned_at,
	account_from_id::text,
	account_to_id::text,
	category_id::text,
	subcategory_id::text,
	income_source_id::text,
	comment,
	created_at,
	updated_at
FROM transactions
WHERE user_id = $1
`

	args := []any{string(input.UserID)}
	if input.Type != nil {
		query += fmt.Sprintf("  AND type = $%d\n", len(args)+1)
		args = append(args, string(*input.Type))
	}
	if input.Status != nil {
		query += fmt.Sprintf("  AND status = $%d\n", len(args)+1)
		args = append(args, string(*input.Status))
	}
	if input.AccountID != nil {
		query += fmt.Sprintf("  AND (account_from_id = $%d OR account_to_id = $%d)\n", len(args)+1, len(args)+1)
		args = append(args, string(*input.AccountID))
	}
	if input.CategoryID != nil {
		query += fmt.Sprintf("  AND category_id = $%d\n", len(args)+1)
		args = append(args, string(*input.CategoryID))
	}
	if input.SubcategoryID != nil {
		query += fmt.Sprintf("  AND subcategory_id = $%d\n", len(args)+1)
		args = append(args, string(*input.SubcategoryID))
	}
	if input.OccurredFrom != nil {
		query += fmt.Sprintf("  AND occurred_at >= $%d\n", len(args)+1)
		args = append(args, *input.OccurredFrom)
	}
	if input.OccurredTo != nil {
		query += fmt.Sprintf("  AND occurred_at <= $%d\n", len(args)+1)
		args = append(args, *input.OccurredTo)
	}
	if input.PlannedFrom != nil {
		query += fmt.Sprintf("  AND planned_at >= $%d\n", len(args)+1)
		args = append(args, *input.PlannedFrom)
	}
	if input.PlannedTo != nil {
		query += fmt.Sprintf("  AND planned_at <= $%d\n", len(args)+1)
		args = append(args, *input.PlannedTo)
	}
	if input.Search != nil && strings.TrimSpace(*input.Search) != "" {
		query += fmt.Sprintf("  AND lower(COALESCE(comment, '')) LIKE $%d\n", len(args)+1)
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(*input.Search))+"%")
	}

	query += "ORDER BY " + listSortExpression(input.Sort) + ", id\n"
	if input.Limit > 0 {
		query += fmt.Sprintf("LIMIT $%d\n", len(args)+1)
		args = append(args, input.Limit)
	}
	if input.Offset > 0 {
		query += fmt.Sprintf("OFFSET $%d\n", len(args)+1)
		args = append(args, input.Offset)
	}

	db := databaseFromContext(ctx, r.pool)
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select transactions by user id: %w", err)
	}
	defer rows.Close()

	transactions := make([]domaintransactions.Transaction, 0, 16)
	for rows.Next() {
		transaction, scanErr := scanTransaction(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan transaction row: %w", scanErr)
		}
		transactions = append(transactions, transaction)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate transaction rows: %w", rows.Err())
	}

	return transactions, nil
}

func (r *TransactionRepository) UpdateByID(
	ctx context.Context,
	transaction domaintransactions.Transaction,
	expectedUpdatedAt time.Time,
) error {
	const query = `
UPDATE transactions
SET type = $3,
    status = $4,
    amount_minor = $5,
    currency = $6,
    occurred_at = $7,
    planned_at = $8,
    account_from_id = $9,
    account_to_id = $10,
    category_id = $11,
    subcategory_id = $12,
    income_source_id = $13,
    comment = $14,
    updated_at = $15
WHERE id = $1
  AND user_id = $2
  AND updated_at = $16
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(
		ctx,
		query,
		string(transaction.ID()),
		string(transaction.UserID()),
		string(transaction.Type()),
		string(transaction.Status()),
		transaction.Amount().MinorUnits(),
		transaction.Amount().Currency().String(),
		transaction.OccurredAt(),
		transaction.PlannedAt(),
		nullableAccountID(transaction.AccountFromID()),
		nullableAccountID(transaction.AccountToID()),
		nullableCategoryID(transaction.CategoryID()),
		nullableSubcategoryID(transaction.SubcategoryID()),
		nullableIncomeSourceID(transaction.IncomeSourceID()),
		transaction.Comment(),
		transaction.UpdatedAt(),
		expectedUpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update transaction by id: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		exists, resolveErr := r.existsByID(ctx, transaction.UserID(), transaction.ID())
		if resolveErr != nil {
			return resolveErr
		}
		if exists {
			return appaccounting.ErrConcurrentTransactionUpdate
		}
		return appaccounting.ErrTransactionNotFound
	}

	return nil
}

func (r *TransactionRepository) DeleteByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) error {
	const query = `
DELETE FROM transactions
WHERE id = $1
  AND user_id = $2
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(transactionID), string(userID))
	if err != nil {
		return fmt.Errorf("delete transaction by id: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appaccounting.ErrTransactionNotFound
	}

	return nil
}

func (r *TransactionRepository) existsByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (bool, error) {
	const query = `
SELECT 1
FROM transactions
WHERE id = $1
  AND user_id = $2
LIMIT 1
`

	db := databaseFromContext(ctx, r.pool)
	var marker int
	if err := db.QueryRow(ctx, query, string(transactionID), string(userID)).Scan(&marker); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("resolve transaction existence: %w", err)
	}

	return true, nil
}

type transactionScanner interface {
	Scan(dest ...any) error
}

func scanTransaction(row transactionScanner) (domaintransactions.Transaction, error) {
	var (
		id             string
		userID         string
		typeRaw        string
		statusRaw      string
		amountMinor    int64
		currencyRaw    string
		occurredAt     *time.Time
		plannedAt      *time.Time
		accountFromID  *string
		accountToID    *string
		categoryID     *string
		subcategoryID  *string
		incomeSourceID *string
		comment        *string
		createdAt      time.Time
		updatedAt      time.Time
	)

	if err := row.Scan(
		&id,
		&userID,
		&typeRaw,
		&statusRaw,
		&amountMinor,
		&currencyRaw,
		&occurredAt,
		&plannedAt,
		&accountFromID,
		&accountToID,
		&categoryID,
		&subcategoryID,
		&incomeSourceID,
		&comment,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domaintransactions.Transaction{}, err
	}

	transactionType, err := domaintransactions.ParseTransactionType(strings.TrimSpace(typeRaw))
	if err != nil {
		return domaintransactions.Transaction{}, fmt.Errorf("parse transaction type %q: %w", typeRaw, err)
	}
	status, err := domaintransactions.ParseTransactionStatus(strings.TrimSpace(statusRaw))
	if err != nil {
		return domaintransactions.Transaction{}, fmt.Errorf("parse transaction status %q: %w", statusRaw, err)
	}
	currency, err := shared.ParseCurrency(strings.TrimSpace(currencyRaw))
	if err != nil {
		return domaintransactions.Transaction{}, fmt.Errorf("parse currency %q: %w", currencyRaw, err)
	}

	accountFrom := optionalAccountID(accountFromID)
	accountTo := optionalAccountID(accountToID)
	category := optionalCategoryID(categoryID)
	subcategory := optionalSubcategoryID(subcategoryID)
	incomeSource := optionalIncomeSourceID(incomeSourceID)

	transaction, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             shared.TransactionID(id),
		UserID:         shared.UserID(userID),
		Type:           transactionType,
		Status:         status,
		Amount:         shared.NewMoney(amountMinor, currency),
		AccountFromID:  accountFrom,
		AccountToID:    accountTo,
		CategoryID:     category,
		SubcategoryID:  subcategory,
		IncomeSourceID: incomeSource,
		Comment:        comment,
		OccurredAt:     occurredAt,
		PlannedAt:      plannedAt,
		PostedAt:       occurredAt,
		CancelledAt:    nil,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	})
	if err != nil {
		return domaintransactions.Transaction{}, fmt.Errorf("build transaction from row: %w", err)
	}

	return transaction, nil
}

func nullableAccountID(value *shared.AccountID) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func nullableCategoryID(value *shared.CategoryID) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func nullableSubcategoryID(value *shared.SubcategoryID) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func nullableIncomeSourceID(value *shared.IncomeSourceID) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func optionalAccountID(value *string) *shared.AccountID {
	if value == nil {
		return nil
	}
	accountID := shared.AccountID(*value)
	return &accountID
}

func optionalCategoryID(value *string) *shared.CategoryID {
	if value == nil {
		return nil
	}
	categoryID := shared.CategoryID(*value)
	return &categoryID
}

func optionalSubcategoryID(value *string) *shared.SubcategoryID {
	if value == nil {
		return nil
	}
	subcategoryID := shared.SubcategoryID(*value)
	return &subcategoryID
}

func optionalIncomeSourceID(value *string) *shared.IncomeSourceID {
	if value == nil {
		return nil
	}
	incomeSourceID := shared.IncomeSourceID(*value)
	return &incomeSourceID
}

func listSortExpression(sort appaccounting.TransactionsSort) string {
	switch sort {
	case appaccounting.TransactionsSortEffectiveAtAsc:
		return "COALESCE(occurred_at, planned_at) ASC NULLS LAST, created_at ASC"
	case appaccounting.TransactionsSortCreatedAtDesc:
		return "created_at DESC"
	case appaccounting.TransactionsSortCreatedAtAsc:
		return "created_at ASC"
	case appaccounting.TransactionsSortAmountDesc:
		return "amount_minor DESC, created_at DESC"
	case appaccounting.TransactionsSortAmountAsc:
		return "amount_minor ASC, created_at DESC"
	default:
		return "COALESCE(occurred_at, planned_at) DESC NULLS LAST, created_at DESC"
	}
}
