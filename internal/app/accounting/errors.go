package accounting

import (
	"errors"
	"fmt"
)

var (
	ErrAccountNotFound                   = errors.New("account not found")
	ErrConcurrentAccountUpdate           = errors.New("concurrent account update")
	ErrTransactionNotFound               = errors.New("transaction not found")
	ErrConcurrentTransactionUpdate       = errors.New("concurrent transaction update")
	ErrTransactionAlreadyPosted          = errors.New("transaction already posted")
	ErrTransactionAlreadyCancelled       = errors.New("transaction already cancelled")
	ErrPostedTransactionPatchConflict    = errors.New("posted transaction cannot be patched in mvp1")
	ErrPostedTransactionDeleteConflict   = errors.New("posted transaction cannot be deleted in mvp1")
	ErrCancelledTransactionPatchConflict = errors.New("cancelled transaction cannot be patched")
)

type BulkItemError struct {
	Index int
	Field string
	Err   error
}

func (e *BulkItemError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field == "" {
		return fmt.Sprintf("bulk item %d failed: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("bulk item %d field %s failed: %v", e.Index, e.Field, e.Err)
}

func (e *BulkItemError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
