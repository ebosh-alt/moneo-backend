package accounting

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
)

func TestCreateTransactionPostedIncomeAdjustsBalanceInSingleTx(t *testing.T) {
	now := time.Date(2026, 4, 30, 18, 0, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	accountTo := shared.AccountID("acc-to")

	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			accountTo: mustAccount(t, userID, accountTo, 1_000_00, now),
		},
	}
	repo := newStubTransactionRepo()
	txm := &stubTxManager{}
	idgen := &stubTransactionIDGen{next: []shared.TransactionID{"tx-1"}}
	service := NewCreateTransactionService(repo, accounts, txm, idgen, fixedClock{now: now})

	created, err := service.Create(context.Background(), CreateTransactionInput{
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPosted,
		Amount:      shared.NewMoney(250_00, shared.CurrencyRUB),
		AccountToID: &accountTo,
	})
	if err != nil {
		t.Fatalf("create posted income transaction: %v", err)
	}
	if txm.calls != 1 {
		t.Fatalf("expected 1 tx boundary call, got %d", txm.calls)
	}
	if created.Status() != domaintransactions.TransactionStatusPosted {
		t.Fatalf("expected posted status, got %s", created.Status())
	}

	updated := accounts.accounts[accountTo]
	if updated.Balance().MinorUnits() != 1_250_00 {
		t.Fatalf("expected balance 125000, got %d", updated.Balance().MinorUnits())
	}
}

func TestCreateTransactionPlannedDoesNotAdjustBalance(t *testing.T) {
	now := time.Date(2026, 4, 30, 18, 5, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	accountFrom := shared.AccountID("acc-from")
	categoryID := shared.CategoryID("cat-1")

	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			accountFrom: mustAccount(t, userID, accountFrom, 1_000_00, now),
		},
	}
	repo := newStubTransactionRepo()
	txm := &stubTxManager{}
	idgen := &stubTransactionIDGen{next: []shared.TransactionID{"tx-2"}}
	service := NewCreateTransactionService(repo, accounts, txm, idgen, fixedClock{now: now})

	_, err := service.Create(context.Background(), CreateTransactionInput{
		UserID:        userID,
		Type:          domaintransactions.TransactionTypeExpense,
		Status:        domaintransactions.TransactionStatusPlanned,
		Amount:        shared.NewMoney(100_00, shared.CurrencyRUB),
		AccountFromID: &accountFrom,
		CategoryID:    &categoryID,
		PlannedAt:     ptrTime(now.Add(24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("create planned expense transaction: %v", err)
	}
	if txm.calls != 1 {
		t.Fatalf("expected 1 tx boundary call, got %d", txm.calls)
	}

	unchanged := accounts.accounts[accountFrom]
	if unchanged.Balance().MinorUnits() != 1_000_00 {
		t.Fatalf("expected unchanged balance 100000, got %d", unchanged.Balance().MinorUnits())
	}
}

func TestPostTransactionTransferAdjustsBothAccounts(t *testing.T) {
	now := time.Date(2026, 4, 30, 19, 0, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	fromID := shared.AccountID("acc-from")
	toID := shared.AccountID("acc-to")

	transaction := mustTransaction(t, newTransactionFixtureInput{
		ID:            "tx-3",
		UserID:        userID,
		Type:          domaintransactions.TransactionTypeTransfer,
		Status:        domaintransactions.TransactionStatusPlanned,
		AmountMinor:   300_00,
		AccountFromID: &fromID,
		AccountToID:   &toID,
		PlannedAt:     ptrTime(now.Add(12 * time.Hour)),
		CreatedAt:     now.Add(-time.Hour),
		UpdatedAt:     now.Add(-time.Hour),
	})

	repo := newStubTransactionRepo()
	repo.put(transaction)
	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			fromID: mustAccount(t, userID, fromID, 1_000_00, now.Add(-time.Hour)),
			toID:   mustAccount(t, userID, toID, 500_00, now.Add(-time.Hour)),
		},
	}
	txm := &stubTxManager{}
	service := NewPostTransactionService(repo, accounts, txm, fixedClock{now: now})

	posted, err := service.PostByID(context.Background(), userID, transaction.ID())
	if err != nil {
		t.Fatalf("post transfer transaction: %v", err)
	}
	if txm.calls != 1 {
		t.Fatalf("expected 1 tx boundary call, got %d", txm.calls)
	}
	if posted.Status() != domaintransactions.TransactionStatusPosted {
		t.Fatalf("expected posted status, got %s", posted.Status())
	}

	from := accounts.accounts[fromID]
	to := accounts.accounts[toID]
	if from.Balance().MinorUnits() != 700_00 {
		t.Fatalf("expected from balance 70000, got %d", from.Balance().MinorUnits())
	}
	if to.Balance().MinorUnits() != 800_00 {
		t.Fatalf("expected to balance 80000, got %d", to.Balance().MinorUnits())
	}
}

func TestCancelPostedExpenseRevertsBalance(t *testing.T) {
	now := time.Date(2026, 4, 30, 20, 0, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	fromID := shared.AccountID("acc-from")
	categoryID := shared.CategoryID("cat-1")

	transaction := mustTransaction(t, newTransactionFixtureInput{
		ID:            "tx-4",
		UserID:        userID,
		Type:          domaintransactions.TransactionTypeExpense,
		Status:        domaintransactions.TransactionStatusPosted,
		AmountMinor:   200_00,
		AccountFromID: &fromID,
		CategoryID:    &categoryID,
		OccurredAt:    ptrTime(now.Add(-2 * time.Hour)),
		CreatedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     now.Add(-2 * time.Hour),
	})
	repo := newStubTransactionRepo()
	repo.put(transaction)
	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			fromID: mustAccount(t, userID, fromID, 800_00, now.Add(-2*time.Hour)),
		},
	}
	txm := &stubTxManager{}
	service := NewCancelTransactionService(repo, accounts, txm, fixedClock{now: now})

	cancelled, err := service.CancelByID(context.Background(), userID, transaction.ID())
	if err != nil {
		t.Fatalf("cancel posted expense transaction: %v", err)
	}
	if cancelled.Status() != domaintransactions.TransactionStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status())
	}

	updated := accounts.accounts[fromID]
	if updated.Balance().MinorUnits() != 1_000_00 {
		t.Fatalf("expected reverted balance 100000, got %d", updated.Balance().MinorUnits())
	}
}

func TestDeletePostedTransactionReturnsConflict(t *testing.T) {
	now := time.Date(2026, 4, 30, 20, 30, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	toID := shared.AccountID("acc-to")

	transaction := mustTransaction(t, newTransactionFixtureInput{
		ID:          "tx-5",
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 300_00,
		AccountToID: &toID,
		OccurredAt:  ptrTime(now.Add(-time.Hour)),
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
	})
	repo := newStubTransactionRepo()
	repo.put(transaction)
	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			toID: mustAccount(t, userID, toID, 1_300_00, now.Add(-time.Hour)),
		},
	}
	txm := &stubTxManager{}
	service := NewDeleteTransactionService(repo, accounts, txm, fixedClock{now: now})

	_, err := service.DeleteByID(context.Background(), userID, transaction.ID())
	if !errors.Is(err, ErrPostedTransactionDeleteConflict) {
		t.Fatalf("expected ErrPostedTransactionDeleteConflict, got %v", err)
	}

	updated := accounts.accounts[toID]
	if updated.Balance().MinorUnits() != 1_300_00 {
		t.Fatalf("expected unchanged balance 130000, got %d", updated.Balance().MinorUnits())
	}
}

func TestPatchPostedTransactionReturnsConflict(t *testing.T) {
	now := time.Date(2026, 4, 30, 21, 0, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	toID := shared.AccountID("acc-to")

	transaction := mustTransaction(t, newTransactionFixtureInput{
		ID:          "tx-6",
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPosted,
		AmountMinor: 100_00,
		AccountToID: &toID,
		OccurredAt:  ptrTime(now.Add(-time.Hour)),
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
	})
	repo := newStubTransactionRepo()
	repo.put(transaction)
	service := NewPatchTransactionService(repo, &stubTxManager{}, fixedClock{now: now})

	newAmount := shared.NewMoney(200_00, shared.CurrencyRUB)
	_, err := service.Patch(context.Background(), PatchTransactionInput{
		UserID:        userID,
		TransactionID: transaction.ID(),
		AmountSet:     true,
		Amount:        &newAmount,
	})
	if !errors.Is(err, ErrPostedTransactionPatchConflict) {
		t.Fatalf("expected ErrPostedTransactionPatchConflict, got %v", err)
	}

	_, err = service.Patch(context.Background(), PatchTransactionInput{
		UserID:        userID,
		TransactionID: transaction.ID(),
		CurrencySet:   true,
	})
	if !errors.Is(err, ErrPostedTransactionPatchConflict) {
		t.Fatalf("expected ErrPostedTransactionPatchConflict on currency patch, got %v", err)
	}
}

func TestPatchTransactionStatusReturnsConflict(t *testing.T) {
	now := time.Date(2026, 4, 30, 21, 30, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	toID := shared.AccountID("acc-to")

	transaction := mustTransaction(t, newTransactionFixtureInput{
		ID:          "tx-6-status",
		UserID:      userID,
		Type:        domaintransactions.TransactionTypeIncome,
		Status:      domaintransactions.TransactionStatusPlanned,
		AmountMinor: 100_00,
		AccountToID: &toID,
		PlannedAt:   ptrTime(now.Add(time.Hour)),
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
	})
	repo := newStubTransactionRepo()
	repo.put(transaction)
	service := NewPatchTransactionService(repo, &stubTxManager{}, fixedClock{now: now})
	posted := domaintransactions.TransactionStatusPosted

	_, err := service.Patch(context.Background(), PatchTransactionInput{
		UserID:        userID,
		TransactionID: transaction.ID(),
		StatusSet:     true,
		Status:        &posted,
	})
	if !errors.Is(err, ErrPostedTransactionPatchConflict) {
		t.Fatalf("expected ErrPostedTransactionPatchConflict on status patch, got %v", err)
	}
}

func TestDuplicateTransactionCreatesPlannedCopyWithoutBalanceAdjustment(t *testing.T) {
	now := time.Date(2026, 4, 30, 22, 0, 0, 0, time.UTC)
	userID := shared.UserID("user-1")
	fromID := shared.AccountID("acc-from")
	categoryID := shared.CategoryID("cat-1")

	source := mustTransaction(t, newTransactionFixtureInput{
		ID:            "tx-7",
		UserID:        userID,
		Type:          domaintransactions.TransactionTypeExpense,
		Status:        domaintransactions.TransactionStatusPosted,
		AmountMinor:   90_00,
		AccountFromID: &fromID,
		CategoryID:    &categoryID,
		OccurredAt:    ptrTime(now.Add(-48 * time.Hour)),
		CreatedAt:     now.Add(-48 * time.Hour),
		UpdatedAt:     now.Add(-48 * time.Hour),
	})
	repo := newStubTransactionRepo()
	repo.put(source)
	idgen := &stubTransactionIDGen{next: []shared.TransactionID{"tx-8"}}
	accounts := &stubTransactionAccounts{
		accounts: map[shared.AccountID]domainaccounting.Account{
			fromID: mustAccount(t, userID, fromID, 1_000_00, now.Add(-48*time.Hour)),
		},
	}
	service := NewDuplicateTransactionService(repo, accounts, &stubTxManager{}, idgen, fixedClock{now: now})

	copyTx, err := service.DuplicateByID(context.Background(), DuplicateTransactionInput{
		UserID:        userID,
		TransactionID: source.ID(),
	})
	if err != nil {
		t.Fatalf("duplicate transaction: %v", err)
	}
	if copyTx.ID() == source.ID() {
		t.Fatalf("expected duplicated transaction with new id")
	}
	if copyTx.Status() != domaintransactions.TransactionStatusPlanned {
		t.Fatalf("expected planned status, got %s", copyTx.Status())
	}
	if copyTx.OccurredAt() == nil {
		t.Fatalf("expected duplicated transaction to inherit occurred_at")
	}
	if copyTx.PlannedAt() == nil {
		t.Fatalf("expected duplicated transaction planned_at to be set")
	}
}

type stubTransactionRepo struct {
	byUser map[shared.UserID]map[shared.TransactionID]domaintransactions.Transaction
}

func newStubTransactionRepo() *stubTransactionRepo {
	return &stubTransactionRepo{
		byUser: make(map[shared.UserID]map[shared.TransactionID]domaintransactions.Transaction),
	}
}

func (r *stubTransactionRepo) Create(_ context.Context, transaction domaintransactions.Transaction) error {
	if _, ok := r.byUser[transaction.UserID()]; !ok {
		r.byUser[transaction.UserID()] = make(map[shared.TransactionID]domaintransactions.Transaction)
	}
	r.byUser[transaction.UserID()][transaction.ID()] = transaction
	return nil
}

func (r *stubTransactionRepo) FindByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	transaction, ok := r.get(userID, transactionID)
	if !ok {
		return domaintransactions.Transaction{}, ErrTransactionNotFound
	}
	return transaction, nil
}

func (r *stubTransactionRepo) ListByUserID(_ context.Context, input ListTransactionsQuery) ([]domaintransactions.Transaction, error) {
	transactions := make([]domaintransactions.Transaction, 0)
	for _, transaction := range r.byUser[input.UserID] {
		if input.Type != nil && transaction.Type() != *input.Type {
			continue
		}
		if input.Status != nil && transaction.Status() != *input.Status {
			continue
		}
		transactions = append(transactions, transaction)
	}
	return transactions, nil
}

func (r *stubTransactionRepo) CountByUserID(_ context.Context, input ListTransactionsQuery) (int, error) {
	transactions, err := r.ListByUserID(context.Background(), input)
	if err != nil {
		return 0, err
	}
	return len(transactions), nil
}

func (r *stubTransactionRepo) UpdateByID(
	_ context.Context,
	transaction domaintransactions.Transaction,
	expectedUpdatedAt time.Time,
) error {
	current, ok := r.get(transaction.UserID(), transaction.ID())
	if !ok {
		return ErrTransactionNotFound
	}
	if !current.UpdatedAt().Equal(expectedUpdatedAt) {
		return ErrConcurrentTransactionUpdate
	}
	r.byUser[transaction.UserID()][transaction.ID()] = transaction
	return nil
}

func (r *stubTransactionRepo) DeleteByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
	expectedUpdatedAt time.Time,
) error {
	userTransactions, ok := r.byUser[userID]
	if !ok {
		return ErrTransactionNotFound
	}
	current, ok := userTransactions[transactionID]
	if !ok {
		return ErrTransactionNotFound
	}
	if !current.UpdatedAt().Equal(expectedUpdatedAt) {
		return ErrConcurrentTransactionUpdate
	}
	if current.Status() == domaintransactions.TransactionStatusPosted {
		return ErrPostedTransactionDeleteConflict
	}
	delete(userTransactions, transactionID)
	return nil
}

func (r *stubTransactionRepo) put(transaction domaintransactions.Transaction) {
	if _, ok := r.byUser[transaction.UserID()]; !ok {
		r.byUser[transaction.UserID()] = make(map[shared.TransactionID]domaintransactions.Transaction)
	}
	r.byUser[transaction.UserID()][transaction.ID()] = transaction
}

func (r *stubTransactionRepo) get(
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, bool) {
	userTransactions, ok := r.byUser[userID]
	if !ok {
		return domaintransactions.Transaction{}, false
	}
	transaction, ok := userTransactions[transactionID]
	return transaction, ok
}

type stubTransactionAccounts struct {
	accounts map[shared.AccountID]domainaccounting.Account
}

func (s *stubTransactionAccounts) FindByID(
	_ context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, ok := s.accounts[accountID]
	if !ok || account.UserID() != userID {
		return domainaccounting.Account{}, ErrAccountNotFound
	}
	return account, nil
}

func (s *stubTransactionAccounts) UpdateByID(
	_ context.Context,
	account domainaccounting.Account,
	expectedUpdatedAt time.Time,
) error {
	current, ok := s.accounts[account.ID()]
	if !ok {
		return ErrAccountNotFound
	}
	if !current.UpdatedAt().Equal(expectedUpdatedAt) {
		return ErrConcurrentAccountUpdate
	}
	s.accounts[account.ID()] = account
	return nil
}

type stubTxManager struct {
	calls int
	err   error
}

func (m *stubTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.calls++
	if m.err != nil {
		return m.err
	}
	return fn(ctx)
}

type stubTransactionIDGen struct {
	next []shared.TransactionID
}

func (g *stubTransactionIDGen) NewTransactionID() shared.TransactionID {
	if len(g.next) == 0 {
		return shared.TransactionID("generated-id")
	}
	id := g.next[0]
	g.next = g.next[1:]
	return id
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type newTransactionFixtureInput struct {
	ID             string
	UserID         shared.UserID
	Type           domaintransactions.TransactionType
	Status         domaintransactions.TransactionStatus
	AmountMinor    int64
	AccountFromID  *shared.AccountID
	AccountToID    *shared.AccountID
	CategoryID     *shared.CategoryID
	SubcategoryID  *shared.SubcategoryID
	IncomeSourceID *shared.IncomeSourceID
	OccurredAt     *time.Time
	PlannedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func mustTransaction(t *testing.T, input newTransactionFixtureInput) domaintransactions.Transaction {
	t.Helper()

	transaction, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             shared.TransactionID(input.ID),
		UserID:         input.UserID,
		Type:           input.Type,
		Status:         input.Status,
		Amount:         shared.NewMoney(input.AmountMinor, shared.CurrencyRUB),
		AccountFromID:  input.AccountFromID,
		AccountToID:    input.AccountToID,
		CategoryID:     input.CategoryID,
		SubcategoryID:  input.SubcategoryID,
		IncomeSourceID: input.IncomeSourceID,
		OccurredAt:     input.OccurredAt,
		PlannedAt:      input.PlannedAt,
		PostedAt:       input.OccurredAt,
		CreatedAt:      input.CreatedAt,
		UpdatedAt:      input.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("build transaction fixture: %v", err)
	}

	return transaction
}

func mustAccount(
	t *testing.T,
	userID shared.UserID,
	accountID shared.AccountID,
	balanceMinor int64,
	updatedAt time.Time,
) domainaccounting.Account {
	t.Helper()

	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   accountID,
		UserID:               userID,
		Name:                 fmt.Sprintf("Account %s", accountID),
		Type:                 domainaccounting.AccountTypeCash,
		Balance:              shared.NewMoney(balanceMinor, shared.CurrencyRUB),
		InitialBalance:       shared.NewMoney(balanceMinor, shared.CurrencyRUB),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
		CreatedAt:            updatedAt.Add(-time.Minute),
		UpdatedAt:            updatedAt,
	})
	if err != nil {
		t.Fatalf("build account fixture: %v", err)
	}

	return account
}

func ptrTime(value time.Time) *time.Time {
	cloned := value
	return &cloned
}
