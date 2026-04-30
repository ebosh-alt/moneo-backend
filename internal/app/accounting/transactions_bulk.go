package accounting

import (
	"context"
	"errors"
	"fmt"

	"moneo/internal/app"
	domaintransactions "moneo/internal/domain/transactions"
)

const transactionsBulkMaxItems = 100

type BulkCreateTransactionsInput struct {
	Items []CreateTransactionInput
}

type BulkPatchTransactionsInput struct {
	Items []PatchTransactionInput
}

type BulkCreateTransactionsService struct {
	txm    app.TxManager
	create *CreateTransactionService
}

func NewBulkCreateTransactionsService(
	repo TransactionRepository,
	accounts TransactionAccountRepository,
	txm app.TxManager,
	idgen TransactionIDGenerator,
	clock AccountClock,
) *BulkCreateTransactionsService {
	return &BulkCreateTransactionsService{
		txm: txm,
		create: NewCreateTransactionService(
			repo,
			accounts,
			txm,
			idgen,
			clock,
		),
	}
}

func (s *BulkCreateTransactionsService) CreateBulk(
	ctx context.Context,
	input BulkCreateTransactionsInput,
) ([]domaintransactions.Transaction, error) {
	if len(input.Items) == 0 {
		return nil, &BulkItemError{Index: 0, Field: "items", Err: errors.New("items must contain at least one item")}
	}
	if len(input.Items) > transactionsBulkMaxItems {
		return nil, &BulkItemError{Index: 0, Field: "items", Err: errors.New("items must not exceed 100")}
	}

	created := make([]domaintransactions.Transaction, 0, len(input.Items))
	if err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		for idx, item := range input.Items {
			transaction, createErr := s.create.createInTx(txCtx, item)
			if createErr != nil {
				return &BulkItemError{
					Index: idx,
					Field: inferCreateField(createErr),
					Err:   createErr,
				}
			}
			created = append(created, transaction)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return created, nil
}

type BulkPatchTransactionsService struct {
	txm    app.TxManager
	get    *GetTransactionService
	patch  *PatchTransactionService
	post   *PostTransactionService
	cancel *CancelTransactionService
}

func NewBulkPatchTransactionsService(
	repo TransactionRepository,
	accounts TransactionAccountRepository,
	txm app.TxManager,
	clock AccountClock,
) *BulkPatchTransactionsService {
	return &BulkPatchTransactionsService{
		txm:    txm,
		get:    NewGetTransactionService(repo),
		patch:  NewPatchTransactionService(repo, txm, clock),
		post:   NewPostTransactionService(repo, accounts, txm, clock),
		cancel: NewCancelTransactionService(repo, accounts, txm, clock),
	}
}

func (s *BulkPatchTransactionsService) PatchBulk(
	ctx context.Context,
	input BulkPatchTransactionsInput,
) ([]domaintransactions.Transaction, error) {
	if len(input.Items) == 0 {
		return nil, &BulkItemError{Index: 0, Field: "items", Err: errors.New("items must contain at least one item")}
	}
	if len(input.Items) > transactionsBulkMaxItems {
		return nil, &BulkItemError{Index: 0, Field: "items", Err: errors.New("items must not exceed 100")}
	}

	updated := make([]domaintransactions.Transaction, 0, len(input.Items))
	if err := s.txm.WithinTx(ctx, func(txCtx context.Context) error {
		for idx, item := range input.Items {
			current, getErr := s.get.GetByID(txCtx, item.UserID, item.TransactionID)
			if getErr != nil {
				return &BulkItemError{
					Index: idx,
					Field: "id",
					Err:   getErr,
				}
			}

			patched, patchErr := s.patchWithoutStatus(txCtx, item)
			if patchErr != nil {
				return &BulkItemError{
					Index: idx,
					Field: inferPatchField(item, patchErr),
					Err:   patchErr,
				}
			}

			next := patched
			if item.StatusSet {
				if item.Status == nil {
					return &BulkItemError{
						Index: idx,
						Field: "status",
						Err:   ErrPostedTransactionPatchConflict,
					}
				}
				statusErr := validateBulkPatchStatusTransition(current.Status(), *item.Status)
				if statusErr != nil {
					return &BulkItemError{
						Index: idx,
						Field: "status",
						Err:   statusErr,
					}
				}
				switch *item.Status {
				case domaintransactions.TransactionStatusPosted:
					posted, postErr := s.post.postInTx(txCtx, item.UserID, item.TransactionID)
					if postErr != nil {
						return &BulkItemError{
							Index: idx,
							Field: "status",
							Err:   postErr,
						}
					}
					next = posted
				case domaintransactions.TransactionStatusCancelled:
					cancelled, cancelErr := s.cancel.cancelInTx(txCtx, item.UserID, item.TransactionID)
					if cancelErr != nil {
						return &BulkItemError{
							Index: idx,
							Field: "status",
							Err:   cancelErr,
						}
					}
					next = cancelled
				}
			}
			updated = append(updated, next)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *BulkPatchTransactionsService) patchWithoutStatus(
	ctx context.Context,
	item PatchTransactionInput,
) (domaintransactions.Transaction, error) {
	withoutStatus := item
	withoutStatus.StatusSet = false
	withoutStatus.Status = nil
	return s.patch.patchInTx(ctx, withoutStatus)
}

func validateBulkPatchStatusTransition(
	current domaintransactions.TransactionStatus,
	target domaintransactions.TransactionStatus,
) error {
	switch current {
	case domaintransactions.TransactionStatusPlanned:
		if target == domaintransactions.TransactionStatusPlanned ||
			target == domaintransactions.TransactionStatusPosted ||
			target == domaintransactions.TransactionStatusCancelled {
			return nil
		}
	case domaintransactions.TransactionStatusPosted:
		if target == domaintransactions.TransactionStatusCancelled {
			return nil
		}
	case domaintransactions.TransactionStatusCancelled:
		return ErrTransactionAlreadyCancelled
	}
	return ErrPostedTransactionPatchConflict
}

func inferCreateField(err error) string {
	switch {
	case errors.Is(err, domaintransactions.ErrTransactionAmountMustBeNonNegative):
		return "amount"
	case errors.Is(err, domaintransactions.ErrTransactionAccountFromRequired),
		errors.Is(err, domaintransactions.ErrTransactionAccountFromMustBeEmpty):
		return "accountFromId"
	case errors.Is(err, domaintransactions.ErrTransactionAccountToRequired),
		errors.Is(err, domaintransactions.ErrTransactionAccountToMustBeEmpty):
		return "accountToId"
	case errors.Is(err, domaintransactions.ErrTransactionCategoryRequired):
		return "categoryId"
	case errors.Is(err, domaintransactions.ErrTransactionCategoryMustBeEmpty):
		return "categoryId"
	case errors.Is(err, domaintransactions.ErrTransactionSubcategoryMustBeEmpty):
		return "subcategoryId"
	case errors.Is(err, domaintransactions.ErrTransactionTransferAccountsMustDiffer):
		return "accountToId"
	case errors.Is(err, domaintransactions.ErrInvalidTransactionType):
		return "type"
	case errors.Is(err, domaintransactions.ErrInvalidTransactionStatus):
		return "status"
	default:
		return "item"
	}
}

func inferPatchField(input PatchTransactionInput, err error) string {
	switch {
	case errors.Is(err, ErrTransactionNotFound):
		return "id"
	case errors.Is(err, ErrConcurrentTransactionUpdate):
		return "id"
	case errors.Is(err, ErrCancelledTransactionPatchConflict):
		return "status"
	case errors.Is(err, ErrPostedTransactionPatchConflict):
		if input.AmountSet {
			return "amount"
		}
		if input.CurrencySet {
			return "currency"
		}
		if input.TypeSet {
			return "type"
		}
		if input.AccountFromIDSet {
			return "accountFromId"
		}
		if input.AccountToIDSet {
			return "accountToId"
		}
		if input.PlannedAtSet {
			return "plannedAt"
		}
		if input.StatusSet {
			return "status"
		}
		return "item"
	case errors.Is(err, domaintransactions.ErrTransactionAmountMustBeNonNegative):
		return "amount"
	case errors.Is(err, domaintransactions.ErrTransactionAccountFromRequired),
		errors.Is(err, domaintransactions.ErrTransactionAccountFromMustBeEmpty):
		return "accountFromId"
	case errors.Is(err, domaintransactions.ErrTransactionAccountToRequired),
		errors.Is(err, domaintransactions.ErrTransactionAccountToMustBeEmpty):
		return "accountToId"
	case errors.Is(err, domaintransactions.ErrTransactionCategoryRequired):
		return "categoryId"
	case errors.Is(err, domaintransactions.ErrTransactionCategoryMustBeEmpty):
		return "categoryId"
	case errors.Is(err, domaintransactions.ErrTransactionSubcategoryMustBeEmpty):
		return "subcategoryId"
	case errors.Is(err, domaintransactions.ErrTransactionTransferAccountsMustDiffer):
		return "accountToId"
	case errors.Is(err, ErrTransactionAlreadyPosted), errors.Is(err, ErrTransactionAlreadyCancelled):
		return "status"
	default:
		return "item"
	}
}

func (e *BulkItemError) String() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("index=%d field=%s err=%v", e.Index, e.Field, e.Err)
}
