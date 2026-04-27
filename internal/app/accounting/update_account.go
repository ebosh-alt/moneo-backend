package accounting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type UpdateAccountRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
	UpdateByID(ctx context.Context, account domainaccounting.Account) error
}

type UpdateAccountInput struct {
	UserID               shared.UserID
	AccountID            shared.AccountID
	Name                 *string
	Type                 *domainaccounting.AccountType
	IncludeInNetWorth    *bool
	IncludeInDailyBudget *bool
}

type UpdateAccountService struct {
	repo  UpdateAccountRepository
	clock AccountClock
}

func NewUpdateAccountService(repo UpdateAccountRepository, clock AccountClock) *UpdateAccountService {
	return &UpdateAccountService{
		repo:  repo,
		clock: clock,
	}
}

func (s *UpdateAccountService) Update(ctx context.Context, input UpdateAccountInput) (domainaccounting.Account, error) {
	account, err := s.repo.FindByID(ctx, input.UserID, input.AccountID)
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("find account by id: %w", err)
	}

	name := account.Name()
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}

	accountType := account.Type()
	if input.Type != nil {
		accountType = *input.Type
	}

	includeInNetWorth := account.IncludeInNetWorth()
	if input.IncludeInNetWorth != nil {
		includeInNetWorth = *input.IncludeInNetWorth
	}

	includeInDailyBudget := account.IncludeInDailyBudget()
	if input.IncludeInDailyBudget != nil {
		includeInDailyBudget = *input.IncludeInDailyBudget
	}

	updatedAt := s.clock.Now().UTC()
	updated, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   account.ID(),
		UserID:               account.UserID(),
		Name:                 name,
		Type:                 accountType,
		Balance:              account.Balance(),
		InitialBalance:       account.InitialBalance(),
		IncludeInNetWorth:    includeInNetWorth,
		IncludeInDailyBudget: includeInDailyBudget,
		ArchivedAt:           account.ArchivedAt(),
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            updatedAt,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	if err := s.repo.UpdateByID(ctx, updated); err != nil {
		if errors.Is(err, ErrDuplicateActiveAccountName) {
			return domainaccounting.Account{}, ErrAccountNameAlreadyExists
		}
		return domainaccounting.Account{}, fmt.Errorf("update account by id: %w", err)
	}

	return updated, nil
}
