package accounting

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var (
	ErrInvalidAccountName      = errors.New("invalid account name")
	ErrInvalidAccountType      = errors.New("invalid account type")
	ErrAccountCurrencyMismatch = errors.New("account currency mismatch")
)

type AccountType string

const (
	AccountTypeCash       AccountType = "cash"
	AccountTypeDebitCard  AccountType = "debit_card"
	AccountTypeSavings    AccountType = "savings"
	AccountTypeBrokerage  AccountType = "brokerage"
	AccountTypeCreditCard AccountType = "credit_card"
	AccountTypeDeposit    AccountType = "deposit"
	AccountTypeDebt       AccountType = "debt"
	AccountTypeOther      AccountType = "other"
)

type Account struct {
	id                   shared.AccountID
	userID               shared.UserID
	name                 string
	accountType          AccountType
	balance              shared.Money
	initialBalance       shared.Money
	includeInNetWorth    bool
	includeInDailyBudget bool
	archivedAt           *time.Time
	createdAt            time.Time
	updatedAt            time.Time
}

type NewAccountParams struct {
	ID                   shared.AccountID
	UserID               shared.UserID
	Name                 string
	Type                 AccountType
	Balance              shared.Money
	InitialBalance       shared.Money
	IncludeInNetWorth    bool
	IncludeInDailyBudget bool
	ArchivedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func NewAccount(params NewAccountParams) (Account, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return Account{}, ErrInvalidAccountName
	}
	if !params.Type.IsSupported() {
		return Account{}, ErrInvalidAccountType
	}
	if params.Balance.Currency() != params.InitialBalance.Currency() {
		return Account{}, ErrAccountCurrencyMismatch
	}

	return Account{
		id:                   params.ID,
		userID:               params.UserID,
		name:                 name,
		accountType:          params.Type,
		balance:              params.Balance,
		initialBalance:       params.InitialBalance,
		includeInNetWorth:    params.IncludeInNetWorth,
		includeInDailyBudget: params.IncludeInDailyBudget,
		archivedAt:           params.ArchivedAt,
		createdAt:            params.CreatedAt,
		updatedAt:            params.UpdatedAt,
	}, nil
}

func ParseAccountType(value string) (AccountType, error) {
	accountType := AccountType(value)
	if !accountType.IsSupported() {
		return "", ErrInvalidAccountType
	}

	return accountType, nil
}

func (t AccountType) IsSupported() bool {
	switch t {
	case AccountTypeCash,
		AccountTypeDebitCard,
		AccountTypeSavings,
		AccountTypeBrokerage,
		AccountTypeCreditCard,
		AccountTypeDeposit,
		AccountTypeDebt,
		AccountTypeOther:
		return true
	default:
		return false
	}
}

func (a Account) ID() shared.AccountID {
	return a.id
}

func (a Account) UserID() shared.UserID {
	return a.userID
}

func (a Account) Name() string {
	return a.name
}

func (a Account) Type() AccountType {
	return a.accountType
}

func (a Account) Balance() shared.Money {
	return a.balance
}

func (a Account) InitialBalance() shared.Money {
	return a.initialBalance
}

func (a Account) IncludeInNetWorth() bool {
	return a.includeInNetWorth
}

func (a Account) IncludeInDailyBudget() bool {
	return a.includeInDailyBudget
}

func (a Account) ArchivedAt() *time.Time {
	return a.archivedAt
}

func (a Account) CreatedAt() time.Time {
	return a.createdAt
}

func (a Account) UpdatedAt() time.Time {
	return a.updatedAt
}
