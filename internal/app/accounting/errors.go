package accounting

import "errors"

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
