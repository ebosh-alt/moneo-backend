package transactions

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var (
	ErrTransactionIDRequired                 = errors.New("transaction id required")
	ErrTransactionUserIDRequired             = errors.New("transaction user id required")
	ErrInvalidTransactionType                = errors.New("invalid transaction type")
	ErrInvalidTransactionStatus              = errors.New("invalid transaction status")
	ErrTransactionAmountMustBeNonNegative    = errors.New("transaction amount must be non-negative")
	ErrTransactionAccountFromRequired        = errors.New("transaction account_from is required")
	ErrTransactionAccountToRequired          = errors.New("transaction account_to is required")
	ErrTransactionAccountFromMustBeEmpty     = errors.New("transaction account_from must be empty")
	ErrTransactionAccountToMustBeEmpty       = errors.New("transaction account_to must be empty")
	ErrTransactionCategoryRequired           = errors.New("transaction category is required")
	ErrTransactionTransferAccountsMustDiffer = errors.New("transfer accounts must be different")
	ErrTransactionAlreadyPosted              = errors.New("transaction is already posted")
	ErrTransactionAlreadyCancelled           = errors.New("transaction is already cancelled")
)

type TransactionType string

const (
	TransactionTypeIncome     TransactionType = "income"
	TransactionTypeExpense    TransactionType = "expense"
	TransactionTypeTransfer   TransactionType = "transfer"
	TransactionTypeInvestment TransactionType = "investment"
	TransactionTypeSaving     TransactionType = "saving"
)

type TransactionStatus string

const (
	TransactionStatusPlanned   TransactionStatus = "planned"
	TransactionStatusPosted    TransactionStatus = "posted"
	TransactionStatusCancelled TransactionStatus = "cancelled"
)

type Transaction struct {
	id              shared.TransactionID
	userID          shared.UserID
	transactionType TransactionType
	status          TransactionStatus
	amount          shared.Money
	accountFromID   *shared.AccountID
	accountToID     *shared.AccountID
	categoryID      *shared.CategoryID
	subcategoryID   *shared.SubcategoryID
	incomeSourceID  *shared.IncomeSourceID
	occurredAt      *time.Time
	plannedAt       *time.Time
	postedAt        *time.Time
	cancelledAt     *time.Time
	createdAt       time.Time
	updatedAt       time.Time
}

type NewTransactionParams struct {
	ID             shared.TransactionID
	UserID         shared.UserID
	Type           TransactionType
	Status         TransactionStatus
	Amount         shared.Money
	AccountFromID  *shared.AccountID
	AccountToID    *shared.AccountID
	CategoryID     *shared.CategoryID
	SubcategoryID  *shared.SubcategoryID
	IncomeSourceID *shared.IncomeSourceID
	OccurredAt     *time.Time
	PlannedAt      *time.Time
	PostedAt       *time.Time
	CancelledAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewTransaction(params NewTransactionParams) (Transaction, error) {
	if strings.TrimSpace(string(params.ID)) == "" {
		return Transaction{}, ErrTransactionIDRequired
	}
	if strings.TrimSpace(string(params.UserID)) == "" {
		return Transaction{}, ErrTransactionUserIDRequired
	}
	if !params.Type.IsSupported() {
		return Transaction{}, ErrInvalidTransactionType
	}
	if !params.Status.IsSupported() {
		return Transaction{}, ErrInvalidTransactionStatus
	}
	if params.Amount.MinorUnits() < 0 {
		return Transaction{}, ErrTransactionAmountMustBeNonNegative
	}
	if hasSubcategoryWithoutCategory(params.SubcategoryID, params.CategoryID) {
		return Transaction{}, ErrTransactionCategoryRequired
	}
	if err := validateTypeInvariants(params); err != nil {
		return Transaction{}, err
	}

	return Transaction{
		id:              params.ID,
		userID:          params.UserID,
		transactionType: params.Type,
		status:          params.Status,
		amount:          params.Amount,
		accountFromID:   cloneAccountID(params.AccountFromID),
		accountToID:     cloneAccountID(params.AccountToID),
		categoryID:      cloneCategoryID(params.CategoryID),
		subcategoryID:   cloneSubcategoryID(params.SubcategoryID),
		incomeSourceID:  cloneIncomeSourceID(params.IncomeSourceID),
		occurredAt:      cloneTime(params.OccurredAt),
		plannedAt:       cloneTime(params.PlannedAt),
		postedAt:        cloneTime(params.PostedAt),
		cancelledAt:     cloneTime(params.CancelledAt),
		createdAt:       params.CreatedAt,
		updatedAt:       params.UpdatedAt,
	}, nil
}

func validateTypeInvariants(params NewTransactionParams) error {
	switch params.Type {
	case TransactionTypeIncome:
		if !hasAccountID(params.AccountToID) {
			return ErrTransactionAccountToRequired
		}
		if hasAccountID(params.AccountFromID) {
			return ErrTransactionAccountFromMustBeEmpty
		}
	case TransactionTypeExpense:
		if !hasAccountID(params.AccountFromID) {
			return ErrTransactionAccountFromRequired
		}
		if hasAccountID(params.AccountToID) {
			return ErrTransactionAccountToMustBeEmpty
		}
		if !hasCategoryID(params.CategoryID) {
			return ErrTransactionCategoryRequired
		}
	case TransactionTypeTransfer:
		if !hasAccountID(params.AccountFromID) {
			return ErrTransactionAccountFromRequired
		}
		if !hasAccountID(params.AccountToID) {
			return ErrTransactionAccountToRequired
		}
		if normalizeID(*params.AccountFromID) == normalizeID(*params.AccountToID) {
			return ErrTransactionTransferAccountsMustDiffer
		}
	case TransactionTypeInvestment, TransactionTypeSaving:
		if !hasAccountID(params.AccountFromID) {
			return ErrTransactionAccountFromRequired
		}
		if !hasCategoryID(params.CategoryID) {
			return ErrTransactionCategoryRequired
		}
		if hasAccountID(params.AccountToID) && normalizeID(*params.AccountFromID) == normalizeID(*params.AccountToID) {
			return ErrTransactionTransferAccountsMustDiffer
		}
	default:
		return ErrInvalidTransactionType
	}

	return nil
}

func ParseTransactionType(value string) (TransactionType, error) {
	transactionType := TransactionType(strings.TrimSpace(value))
	if !transactionType.IsSupported() {
		return "", ErrInvalidTransactionType
	}

	return transactionType, nil
}

func (t TransactionType) IsSupported() bool {
	switch t {
	case TransactionTypeIncome,
		TransactionTypeExpense,
		TransactionTypeTransfer,
		TransactionTypeInvestment,
		TransactionTypeSaving:
		return true
	default:
		return false
	}
}

func ParseTransactionStatus(value string) (TransactionStatus, error) {
	status := TransactionStatus(strings.TrimSpace(value))
	if !status.IsSupported() {
		return "", ErrInvalidTransactionStatus
	}

	return status, nil
}

func (s TransactionStatus) IsSupported() bool {
	switch s {
	case TransactionStatusPlanned,
		TransactionStatusPosted,
		TransactionStatusCancelled:
		return true
	default:
		return false
	}
}

func (t *Transaction) Post(postedAt time.Time) error {
	switch t.status {
	case TransactionStatusPosted:
		return ErrTransactionAlreadyPosted
	case TransactionStatusCancelled:
		return ErrTransactionAlreadyCancelled
	case TransactionStatusPlanned:
		t.status = TransactionStatusPosted
		t.postedAt = cloneTime(&postedAt)
		t.updatedAt = postedAt
		return nil
	default:
		return ErrInvalidTransactionStatus
	}
}

func (t *Transaction) Cancel(cancelledAt time.Time) error {
	switch t.status {
	case TransactionStatusCancelled:
		return ErrTransactionAlreadyCancelled
	case TransactionStatusPlanned, TransactionStatusPosted:
		t.status = TransactionStatusCancelled
		t.cancelledAt = cloneTime(&cancelledAt)
		t.updatedAt = cancelledAt
		return nil
	default:
		return ErrInvalidTransactionStatus
	}
}

func (t Transaction) ID() shared.TransactionID {
	return t.id
}

func (t Transaction) UserID() shared.UserID {
	return t.userID
}

func (t Transaction) Type() TransactionType {
	return t.transactionType
}

func (t Transaction) Status() TransactionStatus {
	return t.status
}

func (t Transaction) Amount() shared.Money {
	return t.amount
}

func (t Transaction) AccountFromID() *shared.AccountID {
	return cloneAccountID(t.accountFromID)
}

func (t Transaction) AccountToID() *shared.AccountID {
	return cloneAccountID(t.accountToID)
}

func (t Transaction) CategoryID() *shared.CategoryID {
	return cloneCategoryID(t.categoryID)
}

func (t Transaction) SubcategoryID() *shared.SubcategoryID {
	return cloneSubcategoryID(t.subcategoryID)
}

func (t Transaction) IncomeSourceID() *shared.IncomeSourceID {
	return cloneIncomeSourceID(t.incomeSourceID)
}

func (t Transaction) OccurredAt() *time.Time {
	return cloneTime(t.occurredAt)
}

func (t Transaction) PlannedAt() *time.Time {
	return cloneTime(t.plannedAt)
}

func (t Transaction) PostedAt() *time.Time {
	return cloneTime(t.postedAt)
}

func (t Transaction) CancelledAt() *time.Time {
	return cloneTime(t.cancelledAt)
}

func (t Transaction) CreatedAt() time.Time {
	return t.createdAt
}

func (t Transaction) UpdatedAt() time.Time {
	return t.updatedAt
}

func hasAccountID(value *shared.AccountID) bool {
	return value != nil && strings.TrimSpace(string(*value)) != ""
}

func hasCategoryID(value *shared.CategoryID) bool {
	return value != nil && strings.TrimSpace(string(*value)) != ""
}

func hasSubcategoryWithoutCategory(subcategoryID *shared.SubcategoryID, categoryID *shared.CategoryID) bool {
	return subcategoryID != nil && strings.TrimSpace(string(*subcategoryID)) != "" && !hasCategoryID(categoryID)
}

func normalizeID[T ~string](value T) string {
	return strings.TrimSpace(string(value))
}

func cloneAccountID(value *shared.AccountID) *shared.AccountID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCategoryID(value *shared.CategoryID) *shared.CategoryID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSubcategoryID(value *shared.SubcategoryID) *shared.SubcategoryID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIncomeSourceID(value *shared.IncomeSourceID) *shared.IncomeSourceID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
