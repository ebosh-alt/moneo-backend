package accounting

import (
	"context"
	"fmt"
	"slices"
	"strings"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type ListAccountsRepository interface {
	ListByUserID(ctx context.Context, userID shared.UserID, includeArchived bool) ([]domainaccounting.Account, error)
}

type ListAccountsInput struct {
	UserID          shared.UserID
	IncludeArchived bool
	Type            *domainaccounting.AccountType
	Currency        *shared.Currency
	Sort            AccountsSort
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

	filtered := make([]domainaccounting.Account, 0, len(accounts))
	for _, account := range accounts {
		if input.Type != nil && account.Type() != *input.Type {
			continue
		}
		if input.Currency != nil && account.Balance().Currency() != *input.Currency {
			continue
		}
		filtered = append(filtered, account)
	}

	sortMode := input.Sort
	if sortMode == "" {
		sortMode = AccountsSortCreatedAtDesc
	}

	slices.SortFunc(filtered, func(left, right domainaccounting.Account) int {
		switch sortMode {
		case AccountsSortNameAsc:
			leftName := strings.ToLower(left.Name())
			rightName := strings.ToLower(right.Name())
			if leftName < rightName {
				return -1
			}
			if leftName > rightName {
				return 1
			}
		case AccountsSortBalanceDesc:
			if left.Balance().MinorUnits() > right.Balance().MinorUnits() {
				return -1
			}
			if left.Balance().MinorUnits() < right.Balance().MinorUnits() {
				return 1
			}
		default:
			if left.CreatedAt().After(right.CreatedAt()) {
				return -1
			}
			if left.CreatedAt().Before(right.CreatedAt()) {
				return 1
			}
		}

		leftID := string(left.ID())
		rightID := string(right.ID())
		if leftID < rightID {
			return -1
		}
		if leftID > rightID {
			return 1
		}
		return 0
	})

	return filtered, nil
}

type AccountsSort string

const (
	AccountsSortCreatedAtDesc AccountsSort = "createdAt:desc"
	AccountsSortNameAsc       AccountsSort = "name:asc"
	AccountsSortBalanceDesc   AccountsSort = "balance:desc"
)
