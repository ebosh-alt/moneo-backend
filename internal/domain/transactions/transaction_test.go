package transactions

import (
	"errors"
	"testing"
	"time"

	"moneo/internal/domain/shared"
)

func TestNewTransactionValidatesTypeSpecificInvariants(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	from := shared.AccountID("acc-from")
	to := shared.AccountID("acc-to")
	category := shared.CategoryID("cat-1")

	tests := []struct {
		name   string
		params NewTransactionParams
		want   error
	}{
		{
			name: "income requires account_to",
			params: NewTransactionParams{
				ID:        shared.TransactionID("tx-1"),
				UserID:    shared.UserID("user-1"),
				Type:      TransactionTypeIncome,
				Status:    TransactionStatusPlanned,
				Amount:    shared.NewMoney(100_00, shared.CurrencyRUB),
				CreatedAt: now,
				UpdatedAt: now,
			},
			want: ErrTransactionAccountToRequired,
		},
		{
			name: "income does not allow account_from",
			params: NewTransactionParams{
				ID:            shared.TransactionID("tx-1"),
				UserID:        shared.UserID("user-1"),
				Type:          TransactionTypeIncome,
				Status:        TransactionStatusPlanned,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &from,
				AccountToID:   &to,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: ErrTransactionAccountFromMustBeEmpty,
		},
		{
			name: "expense requires account_from",
			params: NewTransactionParams{
				ID:         shared.TransactionID("tx-1"),
				UserID:     shared.UserID("user-1"),
				Type:       TransactionTypeExpense,
				Status:     TransactionStatusPlanned,
				Amount:     shared.NewMoney(100_00, shared.CurrencyRUB),
				CategoryID: &category,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			want: ErrTransactionAccountFromRequired,
		},
		{
			name: "expense requires category",
			params: NewTransactionParams{
				ID:            shared.TransactionID("tx-1"),
				UserID:        shared.UserID("user-1"),
				Type:          TransactionTypeExpense,
				Status:        TransactionStatusPlanned,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &from,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: ErrTransactionCategoryRequired,
		},
		{
			name: "expense does not allow account_to",
			params: NewTransactionParams{
				ID:            shared.TransactionID("tx-1"),
				UserID:        shared.UserID("user-1"),
				Type:          TransactionTypeExpense,
				Status:        TransactionStatusPlanned,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &from,
				AccountToID:   &to,
				CategoryID:    &category,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: ErrTransactionAccountToMustBeEmpty,
		},
		{
			name: "transfer requires account_from",
			params: NewTransactionParams{
				ID:          shared.TransactionID("tx-1"),
				UserID:      shared.UserID("user-1"),
				Type:        TransactionTypeTransfer,
				Status:      TransactionStatusPlanned,
				Amount:      shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountToID: &to,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: ErrTransactionAccountFromRequired,
		},
		{
			name: "transfer requires account_to",
			params: NewTransactionParams{
				ID:            shared.TransactionID("tx-1"),
				UserID:        shared.UserID("user-1"),
				Type:          TransactionTypeTransfer,
				Status:        TransactionStatusPlanned,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &from,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: ErrTransactionAccountToRequired,
		},
		{
			name: "transfer requires different accounts",
			params: NewTransactionParams{
				ID:            shared.TransactionID("tx-1"),
				UserID:        shared.UserID("user-1"),
				Type:          TransactionTypeTransfer,
				Status:        TransactionStatusPlanned,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &from,
				AccountToID:   &from,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: ErrTransactionTransferAccountsMustDiffer,
		},
		{
			name: "investment requires account_from",
			params: NewTransactionParams{
				ID:         shared.TransactionID("tx-1"),
				UserID:     shared.UserID("user-1"),
				Type:       TransactionTypeInvestment,
				Status:     TransactionStatusPlanned,
				Amount:     shared.NewMoney(100_00, shared.CurrencyRUB),
				CategoryID: &category,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			want: ErrTransactionAccountFromRequired,
		},
		{
			name: "saving requires category",
			params: NewTransactionParams{
				ID:            shared.TransactionID("tx-1"),
				UserID:        shared.UserID("user-1"),
				Type:          TransactionTypeSaving,
				Status:        TransactionStatusPlanned,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &from,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: ErrTransactionCategoryRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewTransaction(tc.params)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestNewTransactionRejectsNegativeAmount(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	to := shared.AccountID("acc-to")

	_, err := NewTransaction(NewTransactionParams{
		ID:          shared.TransactionID("tx-1"),
		UserID:      shared.UserID("user-1"),
		Type:        TransactionTypeIncome,
		Status:      TransactionStatusPlanned,
		Amount:      shared.NewMoney(-1, shared.CurrencyRUB),
		AccountToID: &to,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if !errors.Is(err, ErrTransactionAmountMustBeNonNegative) {
		t.Fatalf("expected ErrTransactionAmountMustBeNonNegative, got %v", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	postTime := now.Add(time.Hour)
	cancelTime := now.Add(2 * time.Hour)

	t.Run("planned to posted is allowed", func(t *testing.T) {
		tx := mustBuildIncomeTransaction(t, now)

		if err := tx.Post(postTime); err != nil {
			t.Fatalf("post planned transaction: %v", err)
		}

		if tx.Status() != TransactionStatusPosted {
			t.Fatalf("expected posted status, got %s", tx.Status())
		}
		if tx.PostedAt() == nil || !tx.PostedAt().Equal(postTime) {
			t.Fatalf("expected posted_at=%s, got %v", postTime, tx.PostedAt())
		}
	})

	t.Run("posted to cancelled is allowed", func(t *testing.T) {
		tx := mustBuildIncomeTransaction(t, now)
		if err := tx.Post(postTime); err != nil {
			t.Fatalf("post planned transaction: %v", err)
		}

		if err := tx.Cancel(cancelTime); err != nil {
			t.Fatalf("cancel posted transaction: %v", err)
		}

		if tx.Status() != TransactionStatusCancelled {
			t.Fatalf("expected cancelled status, got %s", tx.Status())
		}
		if tx.CancelledAt() == nil || !tx.CancelledAt().Equal(cancelTime) {
			t.Fatalf("expected cancelled_at=%s, got %v", cancelTime, tx.CancelledAt())
		}
	})

	t.Run("planned to cancelled is allowed", func(t *testing.T) {
		tx := mustBuildIncomeTransaction(t, now)

		if err := tx.Cancel(cancelTime); err != nil {
			t.Fatalf("cancel planned transaction: %v", err)
		}

		if tx.Status() != TransactionStatusCancelled {
			t.Fatalf("expected cancelled status, got %s", tx.Status())
		}
	})

	t.Run("post is idempotency conflict when already posted", func(t *testing.T) {
		tx := mustBuildIncomeTransaction(t, now)
		if err := tx.Post(postTime); err != nil {
			t.Fatalf("post planned transaction: %v", err)
		}

		err := tx.Post(postTime.Add(time.Minute))
		if !errors.Is(err, ErrTransactionAlreadyPosted) {
			t.Fatalf("expected ErrTransactionAlreadyPosted, got %v", err)
		}
	})

	t.Run("cancel is idempotency conflict when already cancelled", func(t *testing.T) {
		tx := mustBuildIncomeTransaction(t, now)
		if err := tx.Cancel(cancelTime); err != nil {
			t.Fatalf("cancel planned transaction: %v", err)
		}

		err := tx.Cancel(cancelTime.Add(time.Minute))
		if !errors.Is(err, ErrTransactionAlreadyCancelled) {
			t.Fatalf("expected ErrTransactionAlreadyCancelled, got %v", err)
		}
	})

	t.Run("post is conflict when cancelled", func(t *testing.T) {
		tx := mustBuildIncomeTransaction(t, now)
		if err := tx.Cancel(cancelTime); err != nil {
			t.Fatalf("cancel planned transaction: %v", err)
		}

		err := tx.Post(postTime)
		if !errors.Is(err, ErrTransactionAlreadyCancelled) {
			t.Fatalf("expected ErrTransactionAlreadyCancelled, got %v", err)
		}
	})
}

func mustBuildIncomeTransaction(t *testing.T, now time.Time) *Transaction {
	t.Helper()

	to := shared.AccountID("acc-to")
	tx, err := NewTransaction(NewTransactionParams{
		ID:          shared.TransactionID("tx-1"),
		UserID:      shared.UserID("user-1"),
		Type:        TransactionTypeIncome,
		Status:      TransactionStatusPlanned,
		Amount:      shared.NewMoney(100_00, shared.CurrencyRUB),
		AccountToID: &to,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("build test transaction: %v", err)
	}

	return &tx
}
