package accounting

import (
	"context"
	"errors"
	"testing"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

func TestArchiveAccountServiceIsIdempotent(t *testing.T) {
	now := time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)
	account := mustBuildArchiveRestoreAccount(t, archiveRestoreFixture{
		ID:      "acc-1",
		UserID:  "user-1",
		Name:    "Main card",
		Type:    domainaccounting.AccountTypeDebitCard,
		Balance: 100_00,
	})

	repo := &stubArchiveAccountRepository{
		accounts: map[shared.AccountID]domainaccounting.Account{
			account.ID(): account,
		},
	}
	service := NewArchiveAccountService(repo, stubAccountClock{now: now})

	archived, err := service.Archive(context.Background(), account.UserID(), account.ID())
	if err != nil {
		t.Fatalf("archive account: %v", err)
	}
	if archived.ArchivedAt() == nil {
		t.Fatal("expected archived account to have archivedAt")
	}
	if !archived.UpdatedAt().Equal(now) {
		t.Fatalf("expected updatedAt %s, got %s", now, archived.UpdatedAt())
	}

	archivedAgain, err := service.Archive(context.Background(), account.UserID(), account.ID())
	if err != nil {
		t.Fatalf("archive account again: %v", err)
	}
	if archivedAgain.ArchivedAt() == nil {
		t.Fatal("expected archived account to keep archivedAt")
	}
	if !archivedAgain.ArchivedAt().Equal(*archived.ArchivedAt()) {
		t.Fatalf("expected idempotent archivedAt %s, got %s", archived.ArchivedAt(), archivedAgain.ArchivedAt())
	}
	if repo.archiveCalls != 1 {
		t.Fatalf("expected archive repository call once, got %d", repo.archiveCalls)
	}
}

func TestRestoreAccountServiceClearsArchivedAt(t *testing.T) {
	archivedAt := time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)
	restoreAt := time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC)
	account := mustBuildArchiveRestoreAccount(t, archiveRestoreFixture{
		ID:         "acc-1",
		UserID:     "user-1",
		Name:       "Main card",
		Type:       domainaccounting.AccountTypeDebitCard,
		Balance:    100_00,
		ArchivedAt: &archivedAt,
		UpdatedAt:  archivedAt,
	})

	repo := &stubRestoreAccountRepository{
		accounts: map[shared.AccountID]domainaccounting.Account{
			account.ID(): account,
		},
	}
	service := NewRestoreAccountService(repo, stubAccountClock{now: restoreAt})

	restored, err := service.Restore(context.Background(), account.UserID(), account.ID())
	if err != nil {
		t.Fatalf("restore account: %v", err)
	}
	if restored.ArchivedAt() != nil {
		t.Fatal("expected restored account to have nil archivedAt")
	}
	if !restored.UpdatedAt().Equal(restoreAt) {
		t.Fatalf("expected updatedAt %s, got %s", restoreAt, restored.UpdatedAt())
	}
	if repo.restoreCalls != 1 {
		t.Fatalf("expected restore repository call once, got %d", repo.restoreCalls)
	}
}

func TestArchiveAccountServiceReturnsNotFoundForForeignOrMissingAccount(t *testing.T) {
	repo := &stubArchiveAccountRepository{
		findErr: ErrAccountNotFound,
	}
	service := NewArchiveAccountService(repo, stubAccountClock{now: time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)})

	_, err := service.Archive(context.Background(), shared.UserID("user-1"), shared.AccountID("acc-1"))
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

type stubArchiveAccountRepository struct {
	accounts     map[shared.AccountID]domainaccounting.Account
	findErr      error
	archiveErr   error
	archiveCalls int
}

func (r *stubArchiveAccountRepository) FindByID(
	_ context.Context,
	_ shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	if r.findErr != nil {
		return domainaccounting.Account{}, r.findErr
	}

	account, ok := r.accounts[accountID]
	if !ok {
		return domainaccounting.Account{}, ErrAccountNotFound
	}
	return account, nil
}

func (r *stubArchiveAccountRepository) ArchiveByID(
	_ context.Context,
	_ shared.UserID,
	accountID shared.AccountID,
	archivedAt time.Time,
) error {
	r.archiveCalls++
	if r.archiveErr != nil {
		return r.archiveErr
	}

	account, ok := r.accounts[accountID]
	if !ok {
		return ErrAccountNotFound
	}
	updated, err := buildArchivedAccount(account, archivedAt)
	if err != nil {
		return err
	}
	r.accounts[accountID] = updated
	return nil
}

type stubRestoreAccountRepository struct {
	accounts     map[shared.AccountID]domainaccounting.Account
	findErr      error
	restoreErr   error
	restoreCalls int
}

func (r *stubRestoreAccountRepository) FindByID(
	_ context.Context,
	_ shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	if r.findErr != nil {
		return domainaccounting.Account{}, r.findErr
	}

	account, ok := r.accounts[accountID]
	if !ok {
		return domainaccounting.Account{}, ErrAccountNotFound
	}
	return account, nil
}

func (r *stubRestoreAccountRepository) RestoreByID(
	_ context.Context,
	_ shared.UserID,
	accountID shared.AccountID,
	updatedAt time.Time,
) error {
	r.restoreCalls++
	if r.restoreErr != nil {
		return r.restoreErr
	}

	account, ok := r.accounts[accountID]
	if !ok {
		return ErrAccountNotFound
	}
	updated, err := buildRestoredAccount(account, updatedAt)
	if err != nil {
		return err
	}
	r.accounts[accountID] = updated
	return nil
}

type archiveRestoreFixture struct {
	ID         shared.AccountID
	UserID     shared.UserID
	Name       string
	Type       domainaccounting.AccountType
	Balance    int64
	ArchivedAt *time.Time
	UpdatedAt  time.Time
}

func mustBuildArchiveRestoreAccount(t *testing.T, fixture archiveRestoreFixture) domainaccounting.Account {
	t.Helper()

	createdAt := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	updatedAt := fixture.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   fixture.ID,
		UserID:               fixture.UserID,
		Name:                 fixture.Name,
		Type:                 fixture.Type,
		Balance:              shared.NewMoney(fixture.Balance, shared.CurrencyRUB),
		InitialBalance:       shared.NewMoney(fixture.Balance, shared.CurrencyRUB),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
		ArchivedAt:           fixture.ArchivedAt,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	})
	if err != nil {
		t.Fatalf("build account fixture: %v", err)
	}

	return account
}
