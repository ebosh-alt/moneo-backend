package accounting

import (
	"context"
	"fmt"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type RestoreAccountRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
	RestoreByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID, updatedAt time.Time) error
}

type RestoreAccountService struct {
	repo  RestoreAccountRepository
	clock AccountClock
}

func NewRestoreAccountService(repo RestoreAccountRepository, clock AccountClock) *RestoreAccountService {
	return &RestoreAccountService{
		repo:  repo,
		clock: clock,
	}
}

func (s *RestoreAccountService) Restore(
	ctx context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, err := s.repo.FindByID(ctx, userID, accountID)
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("find account by id: %w", err)
	}
	if account.ArchivedAt() == nil {
		return account, nil
	}

	updatedAt := s.clock.Now().UTC()
	if err := s.repo.RestoreByID(ctx, userID, accountID, updatedAt); err != nil {
		return domainaccounting.Account{}, fmt.Errorf("restore account by id: %w", err)
	}

	return buildRestoredAccount(account, updatedAt)
}

func buildRestoredAccount(
	account domainaccounting.Account,
	updatedAt time.Time,
) (domainaccounting.Account, error) {
	restoredAccount, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   account.ID(),
		UserID:               account.UserID(),
		Name:                 account.Name(),
		Type:                 account.Type(),
		Balance:              account.Balance(),
		InitialBalance:       account.InitialBalance(),
		IncludeInNetWorth:    account.IncludeInNetWorth(),
		IncludeInDailyBudget: account.IncludeInDailyBudget(),
		ArchivedAt:           nil,
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            updatedAt,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	return restoredAccount, nil
}
