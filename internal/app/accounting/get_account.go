package accounting

import (
	"context"
	"fmt"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type GetAccountRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
}

type GetAccountService struct {
	repo GetAccountRepository
}

func NewGetAccountService(repo GetAccountRepository) *GetAccountService {
	return &GetAccountService{repo: repo}
}

func (s *GetAccountService) GetByID(
	ctx context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, err := s.repo.FindByID(ctx, userID, accountID)
	if err != nil {
		return domainaccounting.Account{}, fmt.Errorf("find account by id: %w", err)
	}

	return account, nil
}
