package accounting

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

var (
	ErrDuplicateActiveAccountName = errors.New("duplicate active account name")
	ErrAccountNameAlreadyExists   = errors.New("account name already exists")
	ErrNegativeInitialBalance     = errors.New("initial balance must be non-negative")
)

type CreateAccountRepository interface {
	Create(ctx context.Context, account domainaccounting.Account) error
}

type AccountIDGenerator interface {
	NewAccountID() shared.AccountID
}

type AccountClock interface {
	Now() time.Time
}

type CreateAccountInput struct {
	UserID               shared.UserID
	Name                 string
	Type                 domainaccounting.AccountType
	InitialBalance       shared.Money
	IncludeInNetWorth    bool
	IncludeInDailyBudget bool
}

type CreateAccountService struct {
	repo  CreateAccountRepository
	idgen AccountIDGenerator
	clock AccountClock
}

func NewCreateAccountService(
	repo CreateAccountRepository,
	idgen AccountIDGenerator,
	clock AccountClock,
) *CreateAccountService {
	return &CreateAccountService{
		repo:  repo,
		idgen: idgen,
		clock: clock,
	}
}

func (s *CreateAccountService) Create(ctx context.Context, input CreateAccountInput) (domainaccounting.Account, error) {
	if input.InitialBalance.MinorUnits() < 0 {
		return domainaccounting.Account{}, ErrNegativeInitialBalance
	}

	now := s.clock.Now().UTC()
	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   s.idgen.NewAccountID(),
		UserID:               input.UserID,
		Name:                 input.Name,
		Type:                 input.Type,
		Balance:              input.InitialBalance,
		InitialBalance:       input.InitialBalance,
		IncludeInNetWorth:    input.IncludeInNetWorth,
		IncludeInDailyBudget: input.IncludeInDailyBudget,
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	if err := s.repo.Create(ctx, account); err != nil {
		if errors.Is(err, ErrDuplicateActiveAccountName) {
			return domainaccounting.Account{}, ErrAccountNameAlreadyExists
		}

		return domainaccounting.Account{}, fmt.Errorf("create account: %w", err)
	}

	return account, nil
}
