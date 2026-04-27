package accounting

import (
	"context"
	"fmt"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

type GetAccountsSummaryRepository interface {
	ListByUserID(ctx context.Context, userID shared.UserID, includeArchived bool) ([]domainaccounting.Account, error)
}

type GetAccountsSummaryInput struct {
	UserID   shared.UserID
	Currency shared.Currency
}

type AccountSummary struct {
	Currency                shared.Currency
	NetWorth                shared.Money
	CashBalance             shared.Money
	AvailableForDailyBudget shared.Money
	CreditLiabilities       shared.Money
	Accounts                []AccountSummaryAccount
}

type AccountSummaryAccount struct {
	ID                   shared.AccountID
	Name                 string
	Type                 domainaccounting.AccountType
	Balance              shared.Money
	IncludeInNetWorth    bool
	IncludeInDailyBudget bool
}

type GetAccountsSummaryService struct {
	repo GetAccountsSummaryRepository
}

func NewGetAccountsSummaryService(repo GetAccountsSummaryRepository) *GetAccountsSummaryService {
	return &GetAccountsSummaryService{repo: repo}
}

func (s *GetAccountsSummaryService) GetByUserAndCurrency(
	ctx context.Context,
	input GetAccountsSummaryInput,
) (AccountSummary, error) {
	accounts, err := s.repo.ListByUserID(ctx, input.UserID, false)
	if err != nil {
		return AccountSummary{}, fmt.Errorf("list accounts by user id: %w", err)
	}

	result := AccountSummary{
		Currency:                input.Currency,
		NetWorth:                shared.NewMoney(0, input.Currency),
		CashBalance:             shared.NewMoney(0, input.Currency),
		AvailableForDailyBudget: shared.NewMoney(0, input.Currency),
		CreditLiabilities:       shared.NewMoney(0, input.Currency),
		Accounts:                make([]AccountSummaryAccount, 0, len(accounts)),
	}

	var netWorthBase int64
	var cashBalance int64
	var availableForDailyBudget int64
	var creditLiabilities int64

	for _, account := range accounts {
		if account.ArchivedAt() != nil {
			continue
		}
		if account.Balance().Currency() != input.Currency {
			continue
		}

		balanceMinor := account.Balance().MinorUnits()
		if account.IncludeInNetWorth() {
			netWorthBase += balanceMinor
		}
		if account.IncludeInDailyBudget() {
			availableForDailyBudget += balanceMinor
		}
		if isCashBalanceAccountType(account.Type()) {
			cashBalance += balanceMinor
		}
		if isLiabilityAccountType(account.Type()) {
			creditLiabilities += balanceMinor
		}

		result.Accounts = append(result.Accounts, AccountSummaryAccount{
			ID:                   account.ID(),
			Name:                 account.Name(),
			Type:                 account.Type(),
			Balance:              account.Balance(),
			IncludeInNetWorth:    account.IncludeInNetWorth(),
			IncludeInDailyBudget: account.IncludeInDailyBudget(),
		})
	}

	result.CreditLiabilities = shared.NewMoney(creditLiabilities, input.Currency)
	result.CashBalance = shared.NewMoney(cashBalance, input.Currency)
	result.AvailableForDailyBudget = shared.NewMoney(availableForDailyBudget, input.Currency)
	result.NetWorth = shared.NewMoney(netWorthBase-creditLiabilities, input.Currency)

	return result, nil
}

func isCashBalanceAccountType(accountType domainaccounting.AccountType) bool {
	switch accountType {
	case domainaccounting.AccountTypeCash,
		domainaccounting.AccountTypeDebitCard,
		domainaccounting.AccountTypeSavings,
		domainaccounting.AccountTypeDeposit:
		return true
	default:
		return false
	}
}

func isLiabilityAccountType(accountType domainaccounting.AccountType) bool {
	switch accountType {
	case domainaccounting.AccountTypeCreditCard, domainaccounting.AccountTypeDebt:
		return true
	default:
		return false
	}
}
