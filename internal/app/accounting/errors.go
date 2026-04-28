package accounting

import "errors"

var (
	ErrAccountNotFound         = errors.New("account not found")
	ErrConcurrentAccountUpdate = errors.New("concurrent account update")
)
