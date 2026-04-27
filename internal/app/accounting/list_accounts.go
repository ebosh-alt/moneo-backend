package accounting

import (
	"context"
	"fmt"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type ListAccountsRepository interface {
	ListByUserID(ctx context.Context, userID shared.UserID, includeArchived bool) ([]domainaccounting.Account, error)
}

type ListAccountsInput struct {
	UserID          shared.UserID
	IncludeArchived bool
}

type ListAccountsService struct {
	repo ListAccountsRepository
}

func NewListAccountsService(repo ListAccountsRepository) *ListAccountsService {
	return &ListAccountsService{repo: repo}
}

func (s *ListAccountsService) ListByUser(ctx context.Context, input ListAccountsInput) ([]domainaccounting.Account, error) {
	accounts, err := s.repo.ListByUserID(ctx, input.UserID, input.IncludeArchived)
	if err != nil {
		return nil, fmt.Errorf("list accounts by user id: %w", err)
	}

	return accounts, nil
}
