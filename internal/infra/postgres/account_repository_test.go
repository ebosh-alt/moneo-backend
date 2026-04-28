package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	appaccounting "moneo/internal/app/accounting"
	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
	opsmigrate "moneo/internal/ops/migrate"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var migrateOnce sync.Once
var migrateErr error

func TestAccountRepositoryCreateFindAndListScopedByUserID(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewAccountRepository(pool)
	ctx := context.Background()

	userA := insertAccountTestUser(t, pool, "user-a@example.com")
	userB := insertAccountTestUser(t, pool, "user-b@example.com")

	first := newAccountFixture(t, userA, "Main card", domainaccounting.AccountTypeDebitCard, 100_00, nil)
	second := newAccountFixture(t, userB, "Savings", domainaccounting.AccountTypeSavings, 200_00, nil)

	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first account: %v", err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second account: %v", err)
	}

	found, err := repo.FindByID(ctx, userA, first.ID())
	if err != nil {
		t.Fatalf("find first account by owner: %v", err)
	}
	if found.UserID() != userA {
		t.Fatalf("expected user %q, got %q", userA, found.UserID())
	}
	if found.Name() != "Main card" {
		t.Fatalf("expected account name Main card, got %q", found.Name())
	}

	_, err = repo.FindByID(ctx, userA, second.ID())
	if !errors.Is(err, appaccounting.ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound for foreign account, got %v", err)
	}

	list, err := repo.ListByUserID(ctx, userA, false)
	if err != nil {
		t.Fatalf("list accounts by user id: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 account for userA, got %d", len(list))
	}
	if list[0].ID() != first.ID() {
		t.Fatalf("expected account %q, got %q", first.ID(), list[0].ID())
	}
}

func TestAccountRepositoryCreateRejectsDuplicateActiveNameForSameUser(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewAccountRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "duplicate@example.com")

	first := newAccountFixture(t, userID, "Main card", domainaccounting.AccountTypeDebitCard, 100_00, nil)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first account: %v", err)
	}

	second := newAccountFixture(t, userID, "main card", domainaccounting.AccountTypeCash, 50_00, nil)
	err := repo.Create(ctx, second)
	if !errors.Is(err, appaccounting.ErrDuplicateActiveAccountName) {
		t.Fatalf("expected ErrDuplicateActiveAccountName, got %v", err)
	}
}

func TestAccountRepositoryListByUserIDSupportsArchiveFiltering(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewAccountRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "archive@example.com")

	active := newAccountFixture(t, userID, "Active account", domainaccounting.AccountTypeCash, 10_00, nil)
	archived := newAccountFixture(t, userID, "Archived account", domainaccounting.AccountTypeSavings, 20_00, nil)

	if err := repo.Create(ctx, active); err != nil {
		t.Fatalf("create active account: %v", err)
	}
	if err := repo.Create(ctx, archived); err != nil {
		t.Fatalf("create archived account: %v", err)
	}

	archivedAt := time.Now().UTC()
	if err := repo.ArchiveByID(ctx, userID, archived.ID(), archivedAt); err != nil {
		t.Fatalf("archive account: %v", err)
	}

	activeOnly, err := repo.ListByUserID(ctx, userID, false)
	if err != nil {
		t.Fatalf("list active accounts: %v", err)
	}
	if len(activeOnly) != 1 {
		t.Fatalf("expected 1 active account, got %d", len(activeOnly))
	}
	if activeOnly[0].ID() != active.ID() {
		t.Fatalf("expected active account %q, got %q", active.ID(), activeOnly[0].ID())
	}

	withArchived, err := repo.ListByUserID(ctx, userID, true)
	if err != nil {
		t.Fatalf("list accounts with archived: %v", err)
	}
	if len(withArchived) != 2 {
		t.Fatalf("expected 2 accounts with archived, got %d", len(withArchived))
	}
}

func TestAccountRepositoryRestoreByIDClearsArchivedAt(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewAccountRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "restore@example.com")

	account := newAccountFixture(t, userID, "Archived account", domainaccounting.AccountTypeCash, 10_00, nil)
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account: %v", err)
	}

	archivedAt := time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)
	if err := repo.ArchiveByID(ctx, userID, account.ID(), archivedAt); err != nil {
		t.Fatalf("archive account: %v", err)
	}

	restoreAt := time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC)
	if err := repo.RestoreByID(ctx, userID, account.ID(), restoreAt); err != nil {
		t.Fatalf("restore account: %v", err)
	}

	found, err := repo.FindByID(ctx, userID, account.ID())
	if err != nil {
		t.Fatalf("find restored account: %v", err)
	}
	if found.ArchivedAt() != nil {
		t.Fatalf("expected archivedAt nil after restore, got %v", found.ArchivedAt())
	}
	if !found.UpdatedAt().Equal(restoreAt) {
		t.Fatalf("expected updatedAt %s, got %s", restoreAt, found.UpdatedAt())
	}
}

func TestAccountRepositoryRestoreByIDMapsUniqueConflict(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewAccountRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "restore-conflict@example.com")

	active := newAccountFixture(t, userID, "Main card", domainaccounting.AccountTypeDebitCard, 100_00, nil)
	if err := repo.Create(ctx, active); err != nil {
		t.Fatalf("create active account: %v", err)
	}

	archivedAt := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	archived := newAccountFixture(t, userID, "main card", domainaccounting.AccountTypeCash, 50_00, &archivedAt)
	if err := repo.Create(ctx, archived); err != nil {
		t.Fatalf("create archived account with duplicate name: %v", err)
	}

	err := repo.RestoreByID(ctx, userID, archived.ID(), time.Date(2026, 4, 28, 13, 0, 0, 0, time.UTC))
	if !errors.Is(err, appaccounting.ErrDuplicateActiveAccountName) {
		t.Fatalf("expected ErrDuplicateActiveAccountName on restore conflict, got %v", err)
	}
}

func TestAccountRepositoryUpdateByIDDetectsConcurrentUpdate(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewAccountRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "update-concurrent-account@example.com")

	account := newAccountFixture(t, userID, "Main card", domainaccounting.AccountTypeDebitCard, 100_00, nil)
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account: %v", err)
	}

	current, err := repo.FindByID(ctx, userID, account.ID())
	if err != nil {
		t.Fatalf("find account: %v", err)
	}

	firstUpdateAt := time.Date(2026, 4, 28, 13, 0, 0, 0, time.UTC)
	firstUpdated, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   current.ID(),
		UserID:               current.UserID(),
		Name:                 "Primary card",
		Type:                 current.Type(),
		Balance:              current.Balance(),
		InitialBalance:       current.InitialBalance(),
		IncludeInNetWorth:    current.IncludeInNetWorth(),
		IncludeInDailyBudget: current.IncludeInDailyBudget(),
		ArchivedAt:           current.ArchivedAt(),
		CreatedAt:            current.CreatedAt(),
		UpdatedAt:            firstUpdateAt,
	})
	if err != nil {
		t.Fatalf("build first updated account: %v", err)
	}
	if err := repo.UpdateByID(ctx, firstUpdated, current.UpdatedAt()); err != nil {
		t.Fatalf("first update account: %v", err)
	}

	staleUpdateAt := firstUpdateAt.Add(time.Second)
	staleUpdated, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   current.ID(),
		UserID:               current.UserID(),
		Name:                 "Wallet",
		Type:                 current.Type(),
		Balance:              current.Balance(),
		InitialBalance:       current.InitialBalance(),
		IncludeInNetWorth:    current.IncludeInNetWorth(),
		IncludeInDailyBudget: current.IncludeInDailyBudget(),
		ArchivedAt:           current.ArchivedAt(),
		CreatedAt:            current.CreatedAt(),
		UpdatedAt:            staleUpdateAt,
	})
	if err != nil {
		t.Fatalf("build stale updated account: %v", err)
	}

	err = repo.UpdateByID(ctx, staleUpdated, current.UpdatedAt())
	if !errors.Is(err, appaccounting.ErrConcurrentAccountUpdate) {
		t.Fatalf("expected ErrConcurrentAccountUpdate, got %v", err)
	}
}

func openPostgresForAccountRepoTests(t *testing.T) *pgxpool.Pool {
	t.Helper()

	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		t.Skip("POSTGRES_URL is not set; skipping postgres repository integration tests")
	}

	migrateOnce.Do(func() {
		root := repoRootFromFile(t)
		t.Setenv(opsmigrate.EnvPostgresURL, postgresURL)
		t.Setenv(opsmigrate.EnvMigrationsDir, filepath.Join(root, "db", "migrations"))

		runner := opsmigrate.NewRunner()
		migrateErr = runner.Run(context.Background(), []string{"up"})
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}

	pool, err := pgxpool.New(context.Background(), postgresURL)
	if err != nil {
		t.Fatalf("open postgres pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	return pool
}

func resetAccountsFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(context.Background(), `TRUNCATE TABLE accounts, auth_one_time_tokens, sessions, users CASCADE`); err != nil {
		t.Fatalf("truncate fixtures: %v", err)
	}
}

func insertAccountTestUser(t *testing.T, pool *pgxpool.Pool, email string) shared.UserID {
	t.Helper()

	id := shared.UserID(uuid.NewString())
	_, err := pool.Exec(context.Background(), `
INSERT INTO users (id, email, password_hash, email_verified, created_at, updated_at)
VALUES ($1, $2, $3, TRUE, now(), now())
`, string(id), email, "hashed-password")
	if err != nil {
		t.Fatalf("insert test user %q: %v", email, err)
	}

	return id
}

func newAccountFixture(
	t *testing.T,
	userID shared.UserID,
	name string,
	accountType domainaccounting.AccountType,
	initialMinor int64,
	archivedAt *time.Time,
) domainaccounting.Account {
	t.Helper()

	now := time.Now().UTC()
	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   shared.AccountID(uuid.NewString()),
		UserID:               userID,
		Name:                 name,
		Type:                 accountType,
		Balance:              shared.NewMoney(initialMinor, shared.CurrencyRUB),
		InitialBalance:       shared.NewMoney(initialMinor, shared.CurrencyRUB),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
		ArchivedAt:           archivedAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		t.Fatalf("build account fixture: %v", err)
	}

	return account
}

func repoRootFromFile(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve current file path")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("cannot resolve repository root from %q: %v", root, err)
	}

	return root
}
