package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	appaccounting "moneo/internal/app/accounting"
	domainaccounting "moneo/internal/domain/accounting"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTransactionRepositoryCreateFindAndListScopedByUserID(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetTransactionsFixtures(t, pool)

	accountRepo := NewAccountRepository(pool)
	categoryRepo := NewCategoryRepository(pool)
	repo := NewTransactionRepository(pool)
	ctx := context.Background()

	userA := insertAccountTestUser(t, pool, "tx-user-a@example.com")
	userB := insertAccountTestUser(t, pool, "tx-user-b@example.com")

	accountsA := createTransactionTestAccounts(t, ctx, accountRepo, userA)
	accountsB := createTransactionTestAccounts(t, ctx, accountRepo, userB)
	categoriesA := createTransactionTestCategories(t, ctx, categoryRepo, userA)
	categoriesB := createTransactionTestCategories(t, ctx, categoryRepo, userB)

	now := time.Date(2026, 4, 30, 13, 0, 0, 0, time.UTC)
	first := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000001"),
		UserID:      userA,
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 120_00,
		AccountFrom: &accountsA.from,
		CategoryID:  &categoriesA.expense,
		OccurredAt:  ptrTime(now),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	second := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000002"),
		UserID:      userB,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 400_00,
		AccountTo:   &accountsB.to,
		CategoryID:  &categoriesB.income,
		OccurredAt:  ptrTime(now.Add(time.Minute)),
		CreatedAt:   now.Add(time.Minute),
		UpdatedAt:   now.Add(time.Minute),
	})

	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first transaction: %v", err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second transaction: %v", err)
	}

	found, err := repo.FindByID(ctx, userA, first.ID())
	if err != nil {
		t.Fatalf("find first transaction by owner: %v", err)
	}
	if found.UserID() != userA {
		t.Fatalf("expected user %q, got %q", userA, found.UserID())
	}
	if found.Type() != domaintransactions.TransactionTypeExpense {
		t.Fatalf("expected expense type, got %q", found.Type())
	}
	if found.Amount().MinorUnits() != 120_00 {
		t.Fatalf("expected amount 12000, got %d", found.Amount().MinorUnits())
	}

	_, err = repo.FindByID(ctx, userA, second.ID())
	if !errors.Is(err, appaccounting.ErrTransactionNotFound) {
		t.Fatalf("expected ErrTransactionNotFound for foreign transaction, got %v", err)
	}

	list, err := repo.ListByUserID(ctx, TransactionListInput{
		UserID: userA,
	})
	if err != nil {
		t.Fatalf("list transactions by user id: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 transaction for userA, got %d", len(list))
	}
	if list[0].ID() != first.ID() {
		t.Fatalf("expected transaction %q, got %q", first.ID(), list[0].ID())
	}
}

func TestTransactionRepositoryUpdateByIDDetectsConcurrentUpdate(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetTransactionsFixtures(t, pool)

	accountRepo := NewAccountRepository(pool)
	categoryRepo := NewCategoryRepository(pool)
	repo := NewTransactionRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "tx-update-concurrent@example.com")

	accounts := createTransactionTestAccounts(t, ctx, accountRepo, userID)
	categories := createTransactionTestCategories(t, ctx, categoryRepo, userID)

	createdAt := time.Date(2026, 4, 30, 14, 0, 0, 0, time.UTC)
	base := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000003"),
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusPlanned,
		AmountMinor: 99_00,
		AccountFrom: &accounts.from,
		CategoryID:  &categories.expense,
		PlannedAt:   ptrTime(createdAt.Add(24 * time.Hour)),
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	})
	if err := repo.Create(ctx, base); err != nil {
		t.Fatalf("create base transaction: %v", err)
	}

	current, err := repo.FindByID(ctx, userID, base.ID())
	if err != nil {
		t.Fatalf("find current transaction: %v", err)
	}

	firstUpdateAt := createdAt.Add(2 * time.Hour)
	firstUpdated := newTransactionFixture(t, transactionFixtureParams{
		ID:          current.ID(),
		UserID:      current.UserID(),
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: current.Amount().MinorUnits(),
		AccountFrom: current.AccountFromID(),
		CategoryID:  current.CategoryID(),
		OccurredAt:  ptrTime(firstUpdateAt),
		PlannedAt:   current.PlannedAt(),
		CreatedAt:   current.CreatedAt(),
		UpdatedAt:   firstUpdateAt,
	})
	if err := repo.UpdateByID(ctx, firstUpdated, current.UpdatedAt()); err != nil {
		t.Fatalf("first update transaction: %v", err)
	}

	staleUpdateAt := firstUpdateAt.Add(time.Second)
	staleUpdated := newTransactionFixture(t, transactionFixtureParams{
		ID:          current.ID(),
		UserID:      current.UserID(),
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusCancelled,
		AmountMinor: current.Amount().MinorUnits(),
		AccountFrom: current.AccountFromID(),
		CategoryID:  current.CategoryID(),
		OccurredAt:  ptrTime(firstUpdateAt),
		CreatedAt:   current.CreatedAt(),
		UpdatedAt:   staleUpdateAt,
	})

	err = repo.UpdateByID(ctx, staleUpdated, current.UpdatedAt())
	if !errors.Is(err, appaccounting.ErrConcurrentTransactionUpdate) {
		t.Fatalf("expected ErrConcurrentTransactionUpdate, got %v", err)
	}
}

func TestTransactionRepositoryDeleteByIDEnforcesOwnership(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetTransactionsFixtures(t, pool)

	accountRepo := NewAccountRepository(pool)
	categoryRepo := NewCategoryRepository(pool)
	repo := NewTransactionRepository(pool)
	ctx := context.Background()

	userA := insertAccountTestUser(t, pool, "tx-delete-user-a@example.com")
	userB := insertAccountTestUser(t, pool, "tx-delete-user-b@example.com")
	accountsA := createTransactionTestAccounts(t, ctx, accountRepo, userA)
	accountsB := createTransactionTestAccounts(t, ctx, accountRepo, userB)
	categoriesA := createTransactionTestCategories(t, ctx, categoryRepo, userA)
	categoriesB := createTransactionTestCategories(t, ctx, categoryRepo, userB)

	now := time.Date(2026, 4, 30, 16, 0, 0, 0, time.UTC)
	first := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000004"),
		UserID:      userA,
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 10_00,
		AccountFrom: &accountsA.from,
		CategoryID:  &categoriesA.expense,
		OccurredAt:  ptrTime(now),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	second := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000005"),
		UserID:      userB,
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 20_00,
		AccountFrom: &accountsB.from,
		CategoryID:  &categoriesB.expense,
		OccurredAt:  ptrTime(now),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first transaction: %v", err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second transaction: %v", err)
	}

	if err := repo.DeleteByID(ctx, userA, first.ID()); err != nil {
		t.Fatalf("delete own transaction: %v", err)
	}
	err := repo.DeleteByID(ctx, userA, first.ID())
	if !errors.Is(err, appaccounting.ErrTransactionNotFound) {
		t.Fatalf("expected ErrTransactionNotFound for repeated delete, got %v", err)
	}

	err = repo.DeleteByID(ctx, userA, second.ID())
	if !errors.Is(err, appaccounting.ErrTransactionNotFound) {
		t.Fatalf("expected ErrTransactionNotFound for foreign delete, got %v", err)
	}

	if _, err := repo.FindByID(ctx, userB, second.ID()); err != nil {
		t.Fatalf("expected second transaction to exist for owner, got %v", err)
	}
}

func TestTransactionRepositoryListByUserIDSupportsFilters(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetTransactionsFixtures(t, pool)

	accountRepo := NewAccountRepository(pool)
	categoryRepo := NewCategoryRepository(pool)
	repo := NewTransactionRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "tx-filters@example.com")

	accounts := createTransactionTestAccounts(t, ctx, accountRepo, userID)
	categories := createTransactionTestCategories(t, ctx, categoryRepo, userID)

	base := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	expense := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000006"),
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeExpense,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 150_00,
		AccountFrom: &accounts.from,
		CategoryID:  &categories.expense,
		OccurredAt:  ptrTime(base),
		CreatedAt:   base,
		UpdatedAt:   base,
	})
	transfer := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000007"),
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeTransfer,
		Status:      domaintransactions.TransactionStatusPlanned,
		AmountMinor: 50_00,
		AccountFrom: &accounts.from,
		AccountTo:   &accounts.to,
		PlannedAt:   ptrTime(base.Add(24 * time.Hour)),
		CreatedAt:   base.Add(24 * time.Hour),
		UpdatedAt:   base.Add(24 * time.Hour),
	})
	income := newTransactionFixture(t, transactionFixtureParams{
		ID:          shared.TransactionID("00000000-0000-0000-0000-000000000008"),
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 500_00,
		AccountTo:   &accounts.to,
		CategoryID:  &categories.income,
		OccurredAt:  ptrTime(base.Add(48 * time.Hour)),
		CreatedAt:   base.Add(48 * time.Hour),
		UpdatedAt:   base.Add(48 * time.Hour),
	})

	for _, transaction := range []domaintransactions.Transaction{expense, transfer, income} {
		if err := repo.Create(ctx, transaction); err != nil {
			t.Fatalf("create transaction %q: %v", transaction.ID(), err)
		}
	}

	statusPlanned := domaintransactions.TransactionStatusPlanned
	plannedOnly, err := repo.ListByUserID(ctx, TransactionListInput{
		UserID: userID,
		Status: &statusPlanned,
	})
	if err != nil {
		t.Fatalf("list planned transactions: %v", err)
	}
	if len(plannedOnly) != 1 || plannedOnly[0].ID() != transfer.ID() {
		t.Fatalf("expected only transfer planned transaction, got %+v", plannedOnly)
	}

	accountTo := accounts.to
	byAccount, err := repo.ListByUserID(ctx, TransactionListInput{
		UserID:    userID,
		AccountID: &accountTo,
	})
	if err != nil {
		t.Fatalf("list by account: %v", err)
	}
	if len(byAccount) != 2 {
		t.Fatalf("expected 2 transactions by account_to, got %d", len(byAccount))
	}

	byCategory, err := repo.ListByUserID(ctx, TransactionListInput{
		UserID:     userID,
		CategoryID: &categories.expense,
	})
	if err != nil {
		t.Fatalf("list by category: %v", err)
	}
	if len(byCategory) != 1 || byCategory[0].ID() != expense.ID() {
		t.Fatalf("expected only expense transaction by category, got %+v", byCategory)
	}

	typeIncome := domaintransactions.TransactionTypeIncome
	byType, err := repo.ListByUserID(ctx, TransactionListInput{
		UserID: userID,
		Type:   &typeIncome,
	})
	if err != nil {
		t.Fatalf("list by type: %v", err)
	}
	if len(byType) != 1 || byType[0].ID() != income.ID() {
		t.Fatalf("expected only income transaction by type, got %+v", byType)
	}

	rangeFrom := base.Add(36 * time.Hour)
	rangeTo := base.Add(72 * time.Hour)
	occurredInRange, err := repo.ListByUserID(ctx, TransactionListInput{
		UserID:       userID,
		OccurredFrom: &rangeFrom,
		OccurredTo:   &rangeTo,
	})
	if err != nil {
		t.Fatalf("list by occurred range: %v", err)
	}
	if len(occurredInRange) != 1 || occurredInRange[0].ID() != income.ID() {
		t.Fatalf("expected only income transaction in occurred range, got %+v", occurredInRange)
	}
}

type transactionTestAccounts struct {
	from shared.AccountID
	to   shared.AccountID
}

type transactionTestCategories struct {
	expense shared.CategoryID
	income  shared.CategoryID
}

func createTransactionTestAccounts(
	t *testing.T,
	ctx context.Context,
	repo *AccountRepository,
	userID shared.UserID,
) transactionTestAccounts {
	t.Helper()

	from := newAccountFixture(t, userID, "Wallet "+string(userID), domainaccounting.AccountTypeCash, 1_000_00, nil)
	to := newAccountFixture(t, userID, "Bank "+string(userID), domainaccounting.AccountTypeSavings, 2_000_00, nil)
	if err := repo.Create(ctx, from); err != nil {
		t.Fatalf("create from account: %v", err)
	}
	if err := repo.Create(ctx, to); err != nil {
		t.Fatalf("create to account: %v", err)
	}

	return transactionTestAccounts{
		from: from.ID(),
		to:   to.ID(),
	}
}

func createTransactionTestCategories(
	t *testing.T,
	ctx context.Context,
	repo *CategoryRepository,
	userID shared.UserID,
) transactionTestCategories {
	t.Helper()

	expense := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Food " + string(userID),
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 10,
	})
	income := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Salary " + string(userID),
		Type:      domaincatalog.CategoryTypeIncome,
		SortOrder: 20,
	})
	if err := repo.Create(ctx, expense); err != nil {
		t.Fatalf("create expense category: %v", err)
	}
	if err := repo.Create(ctx, income); err != nil {
		t.Fatalf("create income category: %v", err)
	}

	return transactionTestCategories{
		expense: expense.ID(),
		income:  income.ID(),
	}
}

func resetTransactionsFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(context.Background(), `TRUNCATE TABLE transactions, subcategories, categories, accounts, auth_one_time_tokens, sessions, users CASCADE`); err != nil {
		t.Fatalf("truncate transaction fixtures: %v", err)
	}
}

type transactionFixtureParams struct {
	ID           shared.TransactionID
	UserID       shared.UserID
	Type         domaintransactions.TransactionType
	Status       domaintransactions.TransactionStatus
	AmountMinor  int64
	AccountFrom  *shared.AccountID
	AccountTo    *shared.AccountID
	CategoryID   *shared.CategoryID
	Subcategory  *shared.SubcategoryID
	IncomeSource *shared.IncomeSourceID
	OccurredAt   *time.Time
	PlannedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func newTransactionFixture(t *testing.T, params transactionFixtureParams) domaintransactions.Transaction {
	t.Helper()

	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := params.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	if params.AmountMinor == 0 {
		params.AmountMinor = 1
	}

	transaction, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             params.ID,
		UserID:         params.UserID,
		Type:           params.Type,
		Status:         params.Status,
		Amount:         shared.NewMoney(params.AmountMinor, shared.CurrencyRUB),
		AccountFromID:  params.AccountFrom,
		AccountToID:    params.AccountTo,
		CategoryID:     params.CategoryID,
		SubcategoryID:  params.Subcategory,
		IncomeSourceID: params.IncomeSource,
		OccurredAt:     params.OccurredAt,
		PlannedAt:      params.PlannedAt,
		PostedAt:       params.OccurredAt,
		CancelledAt:    nil,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	})
	if err != nil {
		t.Fatalf("build transaction fixture: %v", err)
	}

	return transaction
}

func ptrTime(value time.Time) *time.Time {
	cloned := value
	return &cloned
}
