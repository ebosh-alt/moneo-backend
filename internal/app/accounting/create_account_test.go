package accounting

import (
	"context"
	"errors"
	"testing"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

func TestCreateAccountServiceMapsDuplicateActiveNameToBusinessError(t *testing.T) {
	repo := &stubCreateAccountRepository{createErr: ErrDuplicateActiveAccountName}
	service := NewCreateAccountService(
		repo,
		stubAccountIDGenerator{id: shared.AccountID("acc-1")},
		stubAccountClock{now: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)},
	)

	_, err := service.Create(context.Background(), CreateAccountInput{
		UserID:               shared.UserID("user-1"),
		Name:                 "Main card",
		Type:                 domainaccounting.AccountTypeDebitCard,
		InitialBalance:       shared.NewMoney(100_00, shared.CurrencyRUB),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
	})
	if !errors.Is(err, ErrAccountNameAlreadyExists) {
		t.Fatalf("expected ErrAccountNameAlreadyExists, got %v", err)
	}
}

func TestCreateAccountServiceRejectsNegativeInitialBalance(t *testing.T) {
	repo := &stubCreateAccountRepository{}
	service := NewCreateAccountService(
		repo,
		stubAccountIDGenerator{id: shared.AccountID("acc-1")},
		stubAccountClock{now: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)},
	)

	_, err := service.Create(context.Background(), CreateAccountInput{
		UserID:               shared.UserID("user-1"),
		Name:                 "Main card",
		Type:                 domainaccounting.AccountTypeDebitCard,
		InitialBalance:       shared.NewMoney(-1, shared.CurrencyRUB),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
	})
	if !errors.Is(err, ErrNegativeInitialBalance) {
		t.Fatalf("expected ErrNegativeInitialBalance, got %v", err)
	}
}

func TestCreateAccountServiceCreatesAccountWithBalanceFromInitialBalance(t *testing.T) {
	repo := &stubCreateAccountRepository{}
	service := NewCreateAccountService(
		repo,
		stubAccountIDGenerator{id: shared.AccountID("acc-1")},
		stubAccountClock{now: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)},
	)

	account, err := service.Create(context.Background(), CreateAccountInput{
		UserID:               shared.UserID("user-1"),
		Name:                 "Main card",
		Type:                 domainaccounting.AccountTypeDebitCard,
		InitialBalance:       shared.NewMoney(100_50, shared.CurrencyRUB),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if account.Balance().MinorUnits() != account.InitialBalance().MinorUnits() {
		t.Fatalf("expected balance and initial balance to match, got %d and %d", account.Balance().MinorUnits(), account.InitialBalance().MinorUnits())
	}
	if account.Balance().Currency() != shared.CurrencyRUB {
		t.Fatalf("expected RUB currency, got %s", account.Balance().Currency())
	}
}

type stubCreateAccountRepository struct {
	createErr error
	created   []domainaccounting.Account
}

func (r *stubCreateAccountRepository) Create(_ context.Context, account domainaccounting.Account) error {
	if r.createErr != nil {
		return r.createErr
	}

	r.created = append(r.created, account)
	return nil
}

type stubAccountIDGenerator struct {
	id shared.AccountID
}

func (g stubAccountIDGenerator) NewAccountID() shared.AccountID {
	return g.id
}

type stubAccountClock struct {
	now time.Time
}

func (c stubAccountClock) Now() time.Time {
	return c.now
}
