package accounting

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type ArchiveAccountRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
	ArchiveByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID, archivedAt time.Time) error
}

type ArchiveAccountService struct {
	repo  ArchiveAccountRepository
	clock AccountClock
}

func NewArchiveAccountService(repo ArchiveAccountRepository, clock AccountClock) *ArchiveAccountService {
	return &ArchiveAccountService{
		repo:  repo,
		clock: clock,
	}
}

func (s *ArchiveAccountService) Archive(
	ctx context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, err := s.repo.FindByID(ctx, userID, accountID)
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("find account by id: %w", err)
	}
	if account.ArchivedAt() != nil {
		return account, nil
	}

	archivedAt := s.clock.Now().UTC()
	if err := s.repo.ArchiveByID(ctx, userID, accountID, archivedAt); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			current, findErr := s.repo.FindByID(ctx, userID, accountID)
			if findErr == nil && current.ArchivedAt() != nil {
				return current, nil
			}
		}
		return domainaccounting.Account{}, fmt.Errorf("archive account by id: %w", err)
	}

	return buildArchivedAccount(account, archivedAt)
}

func buildArchivedAccount(
	account domainaccounting.Account,
	archivedAt time.Time,
) (domainaccounting.Account, error) {
	archivedAtCopy := archivedAt
	archivedAccount, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   account.ID(),
		UserID:               account.UserID(),
		Name:                 account.Name(),
		Type:                 account.Type(),
		Balance:              account.Balance(),
		InitialBalance:       account.InitialBalance(),
		IncludeInNetWorth:    account.IncludeInNetWorth(),
		IncludeInDailyBudget: account.IncludeInDailyBudget(),
		ArchivedAt:           &archivedAtCopy,
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            archivedAtCopy,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	return archivedAccount, nil
}
