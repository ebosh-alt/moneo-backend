package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appaccounting "moneo/internal/app/accounting"
	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountRepository struct {
	pool *pgxpool.Pool
}

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

func (r *AccountRepository) Create(ctx context.Context, account domainaccounting.Account) error {
	const query = `
INSERT INTO accounts (
	id,
	user_id,
	name,
	type,
	currency,
	balance_minor,
	initial_balance_minor,
	include_in_net_worth,
	include_in_daily_budget,
	archived_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(
		ctx,
		query,
		string(account.ID()),
		string(account.UserID()),
		account.Name(),
		string(account.Type()),
		account.Balance().Currency().String(),
		account.Balance().MinorUnits(),
		account.InitialBalance().MinorUnits(),
		account.IncludeInNetWorth(),
		account.IncludeInDailyBudget(),
		account.ArchivedAt(),
		account.CreatedAt(),
		account.UpdatedAt(),
	); err != nil {
		if isUniqueViolation(err, "ux_accounts_user_name_active") {
			return appaccounting.ErrDuplicateActiveAccountName
		}

		return fmt.Errorf("insert account: %w", err)
	}

	return nil
}

func (r *AccountRepository) FindByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	name,
	type,
	currency,
	balance_minor,
	initial_balance_minor,
	include_in_net_worth,
	include_in_daily_budget,
	archived_at,
	created_at,
	updated_at
FROM accounts
WHERE id = $1
  AND user_id = $2
LIMIT 1
`

	db := databaseFromContext(ctx, r.pool)
	account, err := scanAccount(db.QueryRow(ctx, query, string(accountID), string(userID)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainaccounting.Account{}, appaccounting.ErrAccountNotFound
		}
		return domainaccounting.Account{}, fmt.Errorf("select account by id: %w", err)
	}

	return account, nil
}

func (r *AccountRepository) ListByUserID(ctx context.Context, userID shared.UserID, includeArchived bool) ([]domainaccounting.Account, error) {
	query := `
SELECT
	id::text,
	user_id::text,
	name,
	type,
	currency,
	balance_minor,
	initial_balance_minor,
	include_in_net_worth,
	include_in_daily_budget,
	archived_at,
	created_at,
	updated_at
FROM accounts
WHERE user_id = $1
`
	if !includeArchived {
		query += "  AND archived_at IS NULL\n"
	}
	query += "ORDER BY created_at DESC, id"

	db := databaseFromContext(ctx, r.pool)
	rows, err := db.Query(ctx, query, string(userID))
	if err != nil {
		return nil, fmt.Errorf("select accounts by user id: %w", err)
	}
	defer rows.Close()

	accounts := make([]domainaccounting.Account, 0, 8)
	for rows.Next() {
		account, scanErr := scanAccount(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan account row: %w", scanErr)
		}
		accounts = append(accounts, account)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate account rows: %w", rows.Err())
	}

	return accounts, nil
}

func (r *AccountRepository) ArchiveByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID, archivedAt time.Time) error {
	const query = `
UPDATE accounts
SET archived_at = $3,
    updated_at = $3
WHERE id = $1
  AND user_id = $2
  AND archived_at IS NULL
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(accountID), string(userID), archivedAt)
	if err != nil {
		return fmt.Errorf("archive account by id: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appaccounting.ErrAccountNotFound
	}

	return nil
}

func (r *AccountRepository) UpdateByID(ctx context.Context, account domainaccounting.Account) error {
	const query = `
UPDATE accounts
SET name = $3,
    type = $4,
    include_in_net_worth = $5,
    include_in_daily_budget = $6,
    updated_at = $7
WHERE id = $1
  AND user_id = $2
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(
		ctx,
		query,
		string(account.ID()),
		string(account.UserID()),
		account.Name(),
		string(account.Type()),
		account.IncludeInNetWorth(),
		account.IncludeInDailyBudget(),
		account.UpdatedAt(),
	)
	if err != nil {
		if isUniqueViolation(err, "ux_accounts_user_name_active") {
			return appaccounting.ErrDuplicateActiveAccountName
		}

		return fmt.Errorf("update account by id: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appaccounting.ErrAccountNotFound
	}

	return nil
}

type accountScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row accountScanner) (domainaccounting.Account, error) {
	var (
		id                   string
		userID               string
		name                 string
		accountTypeRaw       string
		currencyRaw          string
		balanceMinor         int64
		initialBalanceMinor  int64
		includeInNetWorth    bool
		includeInDailyBudget bool
		archivedAt           *time.Time
		createdAt            time.Time
		updatedAt            time.Time
	)

	if err := row.Scan(
		&id,
		&userID,
		&name,
		&accountTypeRaw,
		&currencyRaw,
		&balanceMinor,
		&initialBalanceMinor,
		&includeInNetWorth,
		&includeInDailyBudget,
		&archivedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domainaccounting.Account{}, err
	}

	accountType, err := domainaccounting.ParseAccountType(strings.TrimSpace(accountTypeRaw))
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("parse account type %q: %w", accountTypeRaw, err)
	}
	currency, err := shared.ParseCurrency(strings.TrimSpace(currencyRaw))
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("parse currency %q: %w", currencyRaw, err)
	}

	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   shared.AccountID(id),
		UserID:               shared.UserID(userID),
		Name:                 name,
		Type:                 accountType,
		Balance:              shared.NewMoney(balanceMinor, currency),
		InitialBalance:       shared.NewMoney(initialBalanceMinor, currency),
		IncludeInNetWorth:    includeInNetWorth,
		IncludeInDailyBudget: includeInDailyBudget,
		ArchivedAt:           archivedAt,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	})
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("build account from row: %w", err)
	}

	return account, nil
}
