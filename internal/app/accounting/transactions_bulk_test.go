package accounting

import (
	"context"
	"errors"
	"testing"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
)

func TestBulkCreateAllOrNothingRollbackOnInvalidItem(t *testing.T) {
	now := time.Date(2026, 4, 30, 22, 30, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	fromID := shared.AccountID("acc-from")
	toID := shared.AccountID("acc-to")
	categoryID := shared.CategoryID("cat-1")

	repo := newStubTransactionRepo()
	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			fromID: mustAccount(t, userID, fromID, 1_000_00, now),
			toID:   mustAccount(t, userID, toID, 500_00, now),
		},
	}
	txm := &rollbackStubTxManager{repo: repo, accounts: accounts}
	idgen := &stubTransactionIDGen{next: []shared.TransactionID{"tx-1", "tx-2"}}
	service := NewBulkCreateTransactionsService(repo, accounts, txm, idgen, fixedClock{now: now})

	_, err := service.CreateBulk(context.Background(), BulkCreateTransactionsInput{
		Items: []CreateTransactionInput{
			{
				UserID:        userID,
				Type:          domaintransactions.TransactionTypeExpense,
				Status:        domaintransactions.TransactionStatusPosted,
				Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
				AccountFromID: &fromID,
				CategoryID:    &categoryID,
			},
			{
				UserID:        userID,
				Type:          domaintransactions.TransactionTypeTransfer,
				Status:        domaintransactions.TransactionStatusPlanned,
				Amount:        shared.NewMoney(50_00, shared.CurrencyRUB),
				AccountFromID: &fromID,
				AccountToID:   &fromID,
			},
		},
	})
	if err == nil {
		t.Fatal("expected bulk create error")
	}

	var itemErr *BulkItemError
	if !errors.As(err, &itemErr) {
		t.Fatalf("expected BulkItemError, got %v", err)
	}
	if itemErr.Index != 1 {
		t.Fatalf("expected failed item index 1, got %d", itemErr.Index)
	}

	transactions, listErr := repo.ListByUserID(context.Background(), ListTransactionsQuery{UserID: userID})
	if listErr != nil {
		t.Fatalf("list transactions after rollback: %v", listErr)
	}
	if len(transactions) != 0 {
		t.Fatalf("expected no transactions after rollback, got %d", len(transactions))
	}

	if balance := accounts.accounts[fromID].Balance().MinorUnits(); balance != 1_000_00 {
		t.Fatalf("expected unchanged account balance, got %d", balance)
	}
}

func TestBulkPatchStatusTransitionsApplyBalanceDeltas(t *testing.T) {
	now := time.Date(2026, 4, 30, 23, 0, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	fromID := shared.AccountID("acc-from")
	toID := shared.AccountID("acc-to")
	categoryID := shared.CategoryID("cat-1")

	repo := newStubTransactionRepo()
	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			fromID: mustAccount(t, userID, fromID, 1_000_00, now),
			toID:   mustAccount(t, userID, toID, 500_00, now),
		},
	}
	txm := &rollbackStubTxManager{repo: repo, accounts: accounts}
	service := NewBulkPatchTransactionsService(repo, accounts, txm, fixedClock{now: now})

	txIncome := mustTransaction(t, newTransactionFixtureInput{
		ID:          "tx-income",
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPlanned,
		AmountMinor: 100_00,
		AccountToID: &toID,
		OccurredAt:  ptrTime(now.Add(-2 * time.Hour)),
		CreatedAt:   now.Add(-2 * time.Hour),
		UpdatedAt:   now.Add(-2 * time.Hour),
	})
	txExpense := mustTransaction(t, newTransactionFixtureInput{
		ID:            "tx-expense",
		UserID:        userID,
		Type:          domaintransactions.TransactionTypeExpense,
		Status:        domaintransactions.TransactionStatusPlanned,
		AmountMinor:   50_00,
		AccountFromID: &fromID,
		CategoryID:    &categoryID,
		OccurredAt:    ptrTime(now.Add(-2 * time.Hour)),
		CreatedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     now.Add(-2 * time.Hour),
	})
	txTransfer := mustTransaction(t, newTransactionFixtureInput{
		ID:            "tx-transfer",
		UserID:        userID,
		Type:          domaintransactions.TransactionTypeTransfer,
		Status:        domaintransactions.TransactionStatusPlanned,
		AmountMinor:   30_00,
		AccountFromID: &fromID,
		AccountToID:   &toID,
		OccurredAt:    ptrTime(now.Add(-2 * time.Hour)),
		CreatedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     now.Add(-2 * time.Hour),
	})
	repo.put(txIncome)
	repo.put(txExpense)
	repo.put(txTransfer)

	posted := domaintransactions.TransactionStatusPosted
	updated, err := service.PatchBulk(context.Background(), BulkPatchTransactionsInput{
		Items: []PatchTransactionInput{
			{UserID: userID, TransactionID: txIncome.ID(), StatusSet: true, Status: &posted},
			{UserID: userID, TransactionID: txExpense.ID(), StatusSet: true, Status: &posted},
			{UserID: userID, TransactionID: txTransfer.ID(), StatusSet: true, Status: &posted},
		},
	})
	if err != nil {
		t.Fatalf("bulk patch status transitions: %v", err)
	}
	if len(updated) != 3 {
		t.Fatalf("expected 3 updated transactions, got %d", len(updated))
	}

	for _, transaction := range updated {
		if transaction.Status() != domaintransactions.TransactionStatusPosted {
			t.Fatalf("expected posted status, got %s", transaction.Status())
		}
	}

	if balance := accounts.accounts[fromID].Balance().MinorUnits(); balance != 920_00 {
		t.Fatalf("expected from account balance 92000, got %d", balance)
	}
	if balance := accounts.accounts[toID].Balance().MinorUnits(); balance != 630_00 {
		t.Fatalf("expected to account balance 63000, got %d", balance)
	}
}

type rollbackStubTxManager struct {
	repo     *stubTransactionRepo
	accounts *stubTransactionAccounts
	depth    int
}

func (m *rollbackStubTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.depth++
	if m.depth > 1 {
		defer func() { m.depth-- }()
		return fn(ctx)
	}

	transactionsSnapshot := cloneTransactionsByUser(m.repo.byUser)
	accountsSnapshot := cloneAccounts(m.accounts.accounts)

	err := fn(ctx)
	m.depth--
	if err != nil {
		m.repo.byUser = transactionsSnapshot
		m.accounts.accounts = accountsSnapshot
	}
	return err
}

func cloneTransactionsByUser(
	source map[shared.UserID]map[shared.TransactionID]domaintransactions.Transaction,
) map[shared.UserID]map[shared.TransactionID]domaintransactions.Transaction {
	cloned := make(map[shared.UserID]map[shared.TransactionID]domaintransactions.Transaction, len(source))
	for userID, userTransactions := range source {
		cloned[userID] = make(map[shared.TransactionID]domaintransactions.Transaction, len(userTransactions))
		for transactionID, transaction := range userTransactions {
			cloned[userID][transactionID] = transaction
		}
	}
	return cloned
}

func cloneAccounts(
	source map[shared.AccountID]domainaccounting.Account,
) map[shared.AccountID]domainaccounting.Account {
	cloned := make(map[shared.AccountID]domainaccounting.Account, len(source))
	for accountID, account := range source {
		cloned[accountID] = account
	}
	return cloned
}
