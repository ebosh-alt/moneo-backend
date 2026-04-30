package accounting

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"moneo/internal/app"
	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
)

type TransactionIDGenerator interface {
	NewTransactionID() shared.TransactionID
}

type TransactionRepository interface {
	Create(ctx context.Context, transaction domaintransactions.Transaction) error
	FindByID(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID) (domaintransactions.Transaction, error)
	ListByUserID(ctx context.Context, input ListTransactionsQuery) ([]domaintransactions.Transaction, error)
	UpdateByID(ctx context.Context, transaction domaintransactions.Transaction, expectedUpdatedAt time.Time) error
	DeleteByID(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID) error
}

type TransactionAccountRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
	UpdateByID(ctx context.Context, account domainaccounting.Account, expectedUpdatedAt time.Time) error
}

type ListTransactionsQuery struct {
	UserID        shared.UserID
	Type          *domaintransactions.TransactionType
	Status        *domaintransactions.TransactionStatus
	AccountID     *shared.AccountID
	CategoryID    *shared.CategoryID
	SubcategoryID *shared.SubcategoryID
	OccurredFrom  *time.Time
	OccurredTo    *time.Time
	PlannedFrom   *time.Time
	PlannedTo     *time.Time
	Search        *string
	Limit         int
	Offset        int
	Sort          TransactionsSort
}

type TransactionsSort string

const (
	TransactionsSortEffectiveAtDesc TransactionsSort = "effective_at:desc"
	TransactionsSortEffectiveAtAsc  TransactionsSort = "effective_at:asc"
	TransactionsSortCreatedAtDesc   TransactionsSort = "created_at:desc"
	TransactionsSortCreatedAtAsc    TransactionsSort = "created_at:asc"
	TransactionsSortAmountDesc      TransactionsSort = "amount:desc"
	TransactionsSortAmountAsc       TransactionsSort = "amount:asc"
)

type CreateTransactionInput struct {
	UserID         shared.UserID
	Type           domaintransactions.TransactionType
	Status         domaintransactions.TransactionStatus
	Amount         shared.Money
	AccountFromID  *shared.AccountID
	AccountToID    *shared.AccountID
	CategoryID     *shared.CategoryID
	SubcategoryID  *shared.SubcategoryID
	IncomeSourceID *shared.IncomeSourceID
	OccurredAt     *time.Time
	PlannedAt      *time.Time
}

type CreateTransactionService struct {
	repo     TransactionRepository
	accounts TransactionAccountRepository
	txm      app.TxManager
	idgen    TransactionIDGenerator
	clock    AccountClock
}

func NewCreateTransactionService(
	repo TransactionRepository,
	accounts TransactionAccountRepository,
	txm app.TxManager,
	idgen TransactionIDGenerator,
	clock AccountClock,
) *CreateTransactionService {
	return &CreateTransactionService{
		repo:     repo,
		accounts: accounts,
		txm:      txm,
		idgen:    idgen,
		clock:    clock,
	}
}

func (s *CreateTransactionService) Create(
	ctx context.Context,
	input CreateTransactionInput,
) (domaintransactions.Transaction, error) {
	now := s.clock.Now().UTC()
	status := input.Status
	if status == "" {
		status = domaintransactions.TransactionStatusPlanned
	}

	occurredAt := cloneTimePtr(input.OccurredAt)
	postedAt := (*time.Time)(nil)
	if status == domaintransactions.TransactionStatusPosted {
		if occurredAt == nil {
			occurredAt = &now
		}
		postedAt = cloneTimePtr(occurredAt)
	}

	transaction, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             s.idgen.NewTransactionID(),
		UserID:         input.UserID,
		Type:           input.Type,
		Status:         status,
		Amount:         input.Amount,
		AccountFromID:  cloneAccountIDPtr(input.AccountFromID),
		AccountToID:    cloneAccountIDPtr(input.AccountToID),
		CategoryID:     cloneCategoryIDPtr(input.CategoryID),
		SubcategoryID:  cloneSubcategoryIDPtr(input.SubcategoryID),
		IncomeSourceID: cloneIncomeSourceIDPtr(input.IncomeSourceID),
		OccurredAt:     occurredAt,
		PlannedAt:      cloneTimePtr(input.PlannedAt),
		PostedAt:       postedAt,
		CancelledAt:    nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	if err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		if transaction.Status() == domaintransactions.TransactionStatusPosted {
			if applyErr := applyTransactionBalanceEffect(
				txCtx,
				s.accounts,
				transaction.UserID(),
				transaction,
				1,
				now,
			); applyErr != nil {
				return applyErr
			}
		}

		if createErr := s.repo.Create(txCtx, transaction); createErr != nil {
			return fmt.Errorf("create transaction: %w", createErr)
		}
		return nil
	}); err != nil {
		return domaintransactions.Transaction{}, err
	}

	return transaction, nil
}

type GetTransactionService struct {
	repo TransactionRepository
}

func NewGetTransactionService(repo TransactionRepository) *GetTransactionService {
	return &GetTransactionService{repo: repo}
}

func (s *GetTransactionService) GetByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	transaction, err := s.repo.FindByID(ctx, userID, transactionID)
	if err != nil {
		return domaintransactions.Transaction{}, fmt.Errorf("find transaction by id: %w", err)
	}
	return transaction, nil
}

type ListTransactionsService struct {
	repo TransactionRepository
}

func NewListTransactionsService(repo TransactionRepository) *ListTransactionsService {
	return &ListTransactionsService{repo: repo}
}

func (s *ListTransactionsService) ListByUser(
	ctx context.Context,
	input ListTransactionsQuery,
) ([]domaintransactions.Transaction, error) {
	sortMode := input.Sort
	if sortMode == "" {
		sortMode = TransactionsSortEffectiveAtDesc
	}
	input.Sort = sortMode
	input.Search = normalizeSearchValue(input.Search)

	transactions, err := s.repo.ListByUserID(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list transactions by user id: %w", err)
	}
	return transactions, nil
}

type PatchTransactionInput struct {
	UserID         shared.UserID
	TransactionID  shared.TransactionID
	Type           *domaintransactions.TransactionType
	Amount         *shared.Money
	AccountFromID  *shared.AccountID
	AccountToID    *shared.AccountID
	CategoryID     *shared.CategoryID
	SubcategoryID  *shared.SubcategoryID
	IncomeSourceID *shared.IncomeSourceID
	OccurredAt     *time.Time
	PlannedAt      *time.Time
}

type PatchTransactionService struct {
	repo  TransactionRepository
	txm   app.TxManager
	clock AccountClock
}

func NewPatchTransactionService(
	repo TransactionRepository,
	txm app.TxManager,
	clock AccountClock,
) *PatchTransactionService {
	return &PatchTransactionService{
		repo:  repo,
		txm:   txm,
		clock: clock,
	}
}

func (s *PatchTransactionService) Patch(
	ctx context.Context,
	input PatchTransactionInput,
) (domaintransactions.Transaction, error) {
	var patched domaintransactions.Transaction
	err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		current, findErr := s.repo.FindByID(txCtx, input.UserID, input.TransactionID)
		if findErr != nil {
			return fmt.Errorf("find transaction by id: %w", findErr)
		}
		if current.Status() == domaintransactions.TransactionStatusPosted {
			return ErrPostedTransactionPatchConflict
		}
		if current.Status() == domaintransactions.TransactionStatusCancelled {
			return ErrCancelledTransactionPatchConflict
		}

		nextType := current.Type()
		if input.Type != nil {
			nextType = *input.Type
		}
		nextAmount := current.Amount()
		if input.Amount != nil {
			nextAmount = *input.Amount
		}
		nextAccountFromID := current.AccountFromID()
		if input.AccountFromID != nil {
			nextAccountFromID = cloneAccountIDPtr(input.AccountFromID)
		}
		nextAccountToID := current.AccountToID()
		if input.AccountToID != nil {
			nextAccountToID = cloneAccountIDPtr(input.AccountToID)
		}
		nextCategoryID := current.CategoryID()
		if input.CategoryID != nil {
			nextCategoryID = cloneCategoryIDPtr(input.CategoryID)
		}
		nextSubcategoryID := current.SubcategoryID()
		if input.SubcategoryID != nil {
			nextSubcategoryID = cloneSubcategoryIDPtr(input.SubcategoryID)
		}
		nextIncomeSourceID := current.IncomeSourceID()
		if input.IncomeSourceID != nil {
			nextIncomeSourceID = cloneIncomeSourceIDPtr(input.IncomeSourceID)
		}
		nextOccurredAt := current.OccurredAt()
		if input.OccurredAt != nil {
			nextOccurredAt = cloneTimePtr(input.OccurredAt)
		}
		nextPlannedAt := current.PlannedAt()
		if input.PlannedAt != nil {
			nextPlannedAt = cloneTimePtr(input.PlannedAt)
		}

		updatedAt := s.clock.Now().UTC()
		next, buildErr := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
			ID:             current.ID(),
			UserID:         current.UserID(),
			Type:           nextType,
			Status:         current.Status(),
			Amount:         nextAmount,
			AccountFromID:  nextAccountFromID,
			AccountToID:    nextAccountToID,
			CategoryID:     nextCategoryID,
			SubcategoryID:  nextSubcategoryID,
			IncomeSourceID: nextIncomeSourceID,
			OccurredAt:     nextOccurredAt,
			PlannedAt:      nextPlannedAt,
			PostedAt:       current.PostedAt(),
			CancelledAt:    current.CancelledAt(),
			CreatedAt:      current.CreatedAt(),
			UpdatedAt:      updatedAt,
		})
		if buildErr != nil {
			return buildErr
		}

		if updateErr := s.repo.UpdateByID(txCtx, next, current.UpdatedAt()); updateErr != nil {
			return fmt.Errorf("update transaction by id: %w", updateErr)
		}
		patched = next
		return nil
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	return patched, nil
}

type DeleteTransactionService struct {
	repo     TransactionRepository
	accounts TransactionAccountRepository
	txm      app.TxManager
	clock    AccountClock
}

func NewDeleteTransactionService(
	repo TransactionRepository,
	accounts TransactionAccountRepository,
	txm app.TxManager,
	clock AccountClock,
) *DeleteTransactionService {
	return &DeleteTransactionService{
		repo:     repo,
		accounts: accounts,
		txm:      txm,
		clock:    clock,
	}
}

func (s *DeleteTransactionService) DeleteByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	var deleted domaintransactions.Transaction
	now := s.clock.Now().UTC()

	err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		current, findErr := s.repo.FindByID(txCtx, userID, transactionID)
		if findErr != nil {
			return fmt.Errorf("find transaction by id: %w", findErr)
		}

		if current.Status() == domaintransactions.TransactionStatusPosted {
			if applyErr := applyTransactionBalanceEffect(
				txCtx,
				s.accounts,
				userID,
				current,
				-1,
				now,
			); applyErr != nil {
				return applyErr
			}
		}

		if deleteErr := s.repo.DeleteByID(txCtx, userID, transactionID); deleteErr != nil {
			return fmt.Errorf("delete transaction by id: %w", deleteErr)
		}

		deleted = current
		return nil
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	return deleted, nil
}

type PostTransactionService struct {
	repo     TransactionRepository
	accounts TransactionAccountRepository
	txm      app.TxManager
	clock    AccountClock
}

func NewPostTransactionService(
	repo TransactionRepository,
	accounts TransactionAccountRepository,
	txm app.TxManager,
	clock AccountClock,
) *PostTransactionService {
	return &PostTransactionService{
		repo:     repo,
		accounts: accounts,
		txm:      txm,
		clock:    clock,
	}
}

func (s *PostTransactionService) PostByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	now := s.clock.Now().UTC()
	var posted domaintransactions.Transaction

	err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		current, findErr := s.repo.FindByID(txCtx, userID, transactionID)
		if findErr != nil {
			return fmt.Errorf("find transaction by id: %w", findErr)
		}

		candidate, buildErr := transactionWithPostDate(current, now)
		if buildErr != nil {
			return buildErr
		}

		if postErr := (&candidate).Post(now); postErr != nil {
			return mapTransactionStatusError(postErr)
		}

		if applyErr := applyTransactionBalanceEffect(txCtx, s.accounts, userID, candidate, 1, now); applyErr != nil {
			return applyErr
		}

		if updateErr := s.repo.UpdateByID(txCtx, candidate, current.UpdatedAt()); updateErr != nil {
			return fmt.Errorf("update transaction by id: %w", updateErr)
		}

		posted = candidate
		return nil
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	return posted, nil
}

type CancelTransactionService struct {
	repo     TransactionRepository
	accounts TransactionAccountRepository
	txm      app.TxManager
	clock    AccountClock
}

func NewCancelTransactionService(
	repo TransactionRepository,
	accounts TransactionAccountRepository,
	txm app.TxManager,
	clock AccountClock,
) *CancelTransactionService {
	return &CancelTransactionService{
		repo:     repo,
		accounts: accounts,
		txm:      txm,
		clock:    clock,
	}
}

func (s *CancelTransactionService) CancelByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	now := s.clock.Now().UTC()
	var cancelled domaintransactions.Transaction

	err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		current, findErr := s.repo.FindByID(txCtx, userID, transactionID)
		if findErr != nil {
			return fmt.Errorf("find transaction by id: %w", findErr)
		}

		candidate := current
		wasPosted := current.Status() == domaintransactions.TransactionStatusPosted
		if cancelErr := (&candidate).Cancel(now); cancelErr != nil {
			return mapTransactionStatusError(cancelErr)
		}

		if wasPosted {
			if applyErr := applyTransactionBalanceEffect(txCtx, s.accounts, userID, current, -1, now); applyErr != nil {
				return applyErr
			}
		}

		if updateErr := s.repo.UpdateByID(txCtx, candidate, current.UpdatedAt()); updateErr != nil {
			return fmt.Errorf("update transaction by id: %w", updateErr)
		}

		cancelled = candidate
		return nil
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	return cancelled, nil
}

type DuplicateTransactionService struct {
	repo  TransactionRepository
	txm   app.TxManager
	idgen TransactionIDGenerator
	clock AccountClock
}

func NewDuplicateTransactionService(
	repo TransactionRepository,
	txm app.TxManager,
	idgen TransactionIDGenerator,
	clock AccountClock,
) *DuplicateTransactionService {
	return &DuplicateTransactionService{
		repo:  repo,
		txm:   txm,
		idgen: idgen,
		clock: clock,
	}
}

func (s *DuplicateTransactionService) DuplicateByID(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	now := s.clock.Now().UTC()
	var duplicated domaintransactions.Transaction

	err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		source, findErr := s.repo.FindByID(txCtx, userID, transactionID)
		if findErr != nil {
			return fmt.Errorf("find source transaction by id: %w", findErr)
		}

		plannedAt := source.PlannedAt()
		if plannedAt == nil {
			plannedAt = source.OccurredAt()
		}

		copyTx, buildErr := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
			ID:             s.idgen.NewTransactionID(),
			UserID:         userID,
			Type:           source.Type(),
			Status:         domaintransactions.TransactionStatusPlanned,
			Amount:         source.Amount(),
			AccountFromID:  source.AccountFromID(),
			AccountToID:    source.AccountToID(),
			CategoryID:     source.CategoryID(),
			SubcategoryID:  source.SubcategoryID(),
			IncomeSourceID: source.IncomeSourceID(),
			OccurredAt:     nil,
			PlannedAt:      plannedAt,
			PostedAt:       nil,
			CancelledAt:    nil,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if buildErr != nil {
			return buildErr
		}

		if createErr := s.repo.Create(txCtx, copyTx); createErr != nil {
			return fmt.Errorf("create duplicated transaction: %w", createErr)
		}

		duplicated = copyTx
		return nil
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	return duplicated, nil
}

func applyTransactionBalanceEffect(
	ctx context.Context,
	accounts TransactionAccountRepository,
	userID shared.UserID,
	transaction domaintransactions.Transaction,
	sign int64,
	updatedAt time.Time,
) error {
	deltas, err := transactionAccountDeltas(transaction)
	if err != nil {
		return err
	}
	if sign < 0 {
		for accountID, delta := range deltas {
			deltas[accountID] = -delta
		}
	}

	accountIDs := make([]string, 0, len(deltas))
	byID := make(map[string]shared.AccountID, len(deltas))
	for accountID := range deltas {
		accountIDs = append(accountIDs, string(accountID))
		byID[string(accountID)] = accountID
	}
	sort.Strings(accountIDs)

	for _, accountIDRaw := range accountIDs {
		accountID := byID[accountIDRaw]
		delta := deltas[accountID]
		if delta == 0 {
			continue
		}

		account, findErr := accounts.FindByID(ctx, userID, accountID)
		if findErr != nil {
			return fmt.Errorf("find account by id for balance adjustment: %w", findErr)
		}

		updatedBalance := shared.NewMoney(account.Balance().MinorUnits()+delta, account.Balance().Currency())
		updatedAccount, buildErr := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
			ID:                   account.ID(),
			UserID:               account.UserID(),
			Name:                 account.Name(),
			Type:                 account.Type(),
			Balance:              updatedBalance,
			InitialBalance:       account.InitialBalance(),
			IncludeInNetWorth:    account.IncludeInNetWorth(),
			IncludeInDailyBudget: account.IncludeInDailyBudget(),
			ArchivedAt:           account.ArchivedAt(),
			CreatedAt:            account.CreatedAt(),
			UpdatedAt:            updatedAt,
		})
		if buildErr != nil {
			return buildErr
		}

		if updateErr := accounts.UpdateByID(ctx, updatedAccount, account.UpdatedAt()); updateErr != nil {
			return fmt.Errorf("update account balance: %w", updateErr)
		}
	}

	return nil
}

func transactionAccountDeltas(transaction domaintransactions.Transaction) (map[shared.AccountID]int64, error) {
	deltas := make(map[shared.AccountID]int64, 2)
	amount := transaction.Amount().MinorUnits()

	switch transaction.Type() {
	case domaintransactions.TransactionTypeIncome:
		if transaction.AccountToID() == nil {
			return nil, errors.New("income transaction has no account_to")
		}
		deltas[*transaction.AccountToID()] += amount
	case domaintransactions.TransactionTypeExpense:
		if transaction.AccountFromID() == nil {
			return nil, errors.New("expense transaction has no account_from")
		}
		deltas[*transaction.AccountFromID()] -= amount
	case domaintransactions.TransactionTypeTransfer:
		if transaction.AccountFromID() == nil || transaction.AccountToID() == nil {
			return nil, errors.New("transfer transaction has incomplete accounts")
		}
		deltas[*transaction.AccountFromID()] -= amount
		deltas[*transaction.AccountToID()] += amount
	case domaintransactions.TransactionTypeSaving, domaintransactions.TransactionTypeInvestment:
		if transaction.AccountFromID() == nil {
			return nil, errors.New("saving/investment transaction has no account_from")
		}
		deltas[*transaction.AccountFromID()] -= amount
		if transaction.AccountToID() != nil {
			deltas[*transaction.AccountToID()] += amount
		}
	default:
		return nil, fmt.Errorf("unsupported transaction type %q", transaction.Type())
	}

	return deltas, nil
}

func transactionWithPostDate(
	transaction domaintransactions.Transaction,
	postTime time.Time,
) (domaintransactions.Transaction, error) {
	occurredAt := transaction.OccurredAt()
	if occurredAt == nil {
		occurredAt = &postTime
	}

	rebuilt, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             transaction.ID(),
		UserID:         transaction.UserID(),
		Type:           transaction.Type(),
		Status:         transaction.Status(),
		Amount:         transaction.Amount(),
		AccountFromID:  transaction.AccountFromID(),
		AccountToID:    transaction.AccountToID(),
		CategoryID:     transaction.CategoryID(),
		SubcategoryID:  transaction.SubcategoryID(),
		IncomeSourceID: transaction.IncomeSourceID(),
		OccurredAt:     occurredAt,
		PlannedAt:      transaction.PlannedAt(),
		PostedAt:       transaction.PostedAt(),
		CancelledAt:    transaction.CancelledAt(),
		CreatedAt:      transaction.CreatedAt(),
		UpdatedAt:      postTime,
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	return rebuilt, nil
}

func mapTransactionStatusError(err error) error {
	switch {
	case errors.Is(err, domaintransactions.ErrTransactionAlreadyPosted):
		return ErrTransactionAlreadyPosted
	case errors.Is(err, domaintransactions.ErrTransactionAlreadyCancelled):
		return ErrTransactionAlreadyCancelled
	default:
		return err
	}
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneAccountIDPtr(value *shared.AccountID) *shared.AccountID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCategoryIDPtr(value *shared.CategoryID) *shared.CategoryID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSubcategoryIDPtr(value *shared.SubcategoryID) *shared.SubcategoryID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIncomeSourceIDPtr(value *shared.IncomeSourceID) *shared.IncomeSourceID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func normalizeSearchValue(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
