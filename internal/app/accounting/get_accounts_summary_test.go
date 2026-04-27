package accounting

import (
	"context"
	"testing"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
)

func TestGetAccountsSummaryServiceCalculatesBucketsByCurrency(t *testing.T) {
	userID := shared.UserID("user-1")
	archivedAt := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)

	repo := &stubGetAccountsSummaryRepository{
		accounts: []domainaccounting.Account{
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-main",
				UserID:               userID,
				Name:                 "Main card",
				Type:                 domainaccounting.AccountTypeDebitCard,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         120_000_00,
				IncludeInNetWorth:    true,
				IncludeInDailyBudget: true,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-cash",
				UserID:               userID,
				Name:                 "Cash",
				Type:                 domainaccounting.AccountTypeCash,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         60_000_00,
				IncludeInNetWorth:    false,
				IncludeInDailyBudget: true,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-savings",
				UserID:               userID,
				Name:                 "Savings",
				Type:                 domainaccounting.AccountTypeSavings,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         50_000_00,
				IncludeInNetWorth:    true,
				IncludeInDailyBudget: false,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-deposit",
				UserID:               userID,
				Name:                 "Deposit",
				Type:                 domainaccounting.AccountTypeDeposit,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         30_000_00,
				IncludeInNetWorth:    true,
				IncludeInDailyBudget: true,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-credit",
				UserID:               userID,
				Name:                 "Credit",
				Type:                 domainaccounting.AccountTypeCreditCard,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         40_000_00,
				IncludeInNetWorth:    true,
				IncludeInDailyBudget: false,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-debt",
				UserID:               userID,
				Name:                 "Debt",
				Type:                 domainaccounting.AccountTypeDebt,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         10_000_00,
				IncludeInNetWorth:    false,
				IncludeInDailyBudget: false,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-archived",
				UserID:               userID,
				Name:                 "Archived",
				Type:                 domainaccounting.AccountTypeCash,
				Currency:             shared.CurrencyRUB,
				BalanceMinor:         999_00,
				IncludeInNetWorth:    true,
				IncludeInDailyBudget: true,
				ArchivedAt:           &archivedAt,
			}),
			mustBuildSummaryAccount(t, summaryAccountFixture{
				ID:                   "acc-usd",
				UserID:               userID,
				Name:                 "USD",
				Type:                 domainaccounting.AccountTypeDebitCard,
				Currency:             shared.CurrencyUSD,
				BalanceMinor:         777_00,
				IncludeInNetWorth:    true,
				IncludeInDailyBudget: true,
			}),
		},
	}

	service := NewGetAccountsSummaryService(repo)
	summary, err := service.GetByUserAndCurrency(context.Background(), GetAccountsSummaryInput{
		UserID:   userID,
		Currency: shared.CurrencyRUB,
	})
	if err != nil {
		t.Fatalf("get accounts summary: %v", err)
	}

	if summary.Currency != shared.CurrencyRUB {
		t.Fatalf("expected RUB summary currency, got %s", summary.Currency)
	}
	if summary.NetWorth.MinorUnits() != 190_000_00 {
		t.Fatalf("expected net worth 19000000, got %d", summary.NetWorth.MinorUnits())
	}
	if summary.CashBalance.MinorUnits() != 260_000_00 {
		t.Fatalf("expected cash balance 26000000, got %d", summary.CashBalance.MinorUnits())
	}
	if summary.AvailableForDailyBudget.MinorUnits() != 210_000_00 {
		t.Fatalf("expected available for daily budget 21000000, got %d", summary.AvailableForDailyBudget.MinorUnits())
	}
	if summary.CreditLiabilities.MinorUnits() != 50_000_00 {
		t.Fatalf("expected credit liabilities 5000000, got %d", summary.CreditLiabilities.MinorUnits())
	}
	if len(summary.Accounts) != 6 {
		t.Fatalf("expected 6 active RUB accounts, got %d", len(summary.Accounts))
	}
}

type stubGetAccountsSummaryRepository struct {
	accounts []domainaccounting.Account
}

func (r *stubGetAccountsSummaryRepository) ListByUserID(
	_ context.Context,
	_ shared.UserID,
	_ bool,
) ([]domainaccounting.Account, error) {
	return r.accounts, nil
}

type summaryAccountFixture struct {
	ID                   string
	UserID               shared.UserID
	Name                 string
	Type                 domainaccounting.AccountType
	Currency             shared.Currency
	BalanceMinor         int64
	IncludeInNetWorth    bool
	IncludeInDailyBudget bool
	ArchivedAt           *time.Time
}

func mustBuildSummaryAccount(t *testing.T, fixture summaryAccountFixture) domainaccounting.Account {
	t.Helper()

	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   shared.AccountID(fixture.ID),
		UserID:               fixture.UserID,
		Name:                 fixture.Name,
		Type:                 fixture.Type,
		Balance:              shared.NewMoney(fixture.BalanceMinor, fixture.Currency),
		InitialBalance:       shared.NewMoney(fixture.BalanceMinor, fixture.Currency),
		IncludeInNetWorth:    fixture.IncludeInNetWorth,
		IncludeInDailyBudget: fixture.IncludeInDailyBudget,
		ArchivedAt:           fixture.ArchivedAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		t.Fatalf("build account fixture: %v", err)
	}

	return account
}
