package http_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	appaccounting "moneo/internal/app/accounting"
	domainaccounting "moneo/internal/domain/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
	transporthttp "moneo/internal/transport/http"
)

func TestTransactionsIntegrationBalancesForPostedPlannedAndCancelled(t *testing.T) {
	fixture := newIntegratedCatalogTransactionsFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-int-balance@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	accountRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":                 "Main",
		"type":                 "cash",
		"currency":             "RUB",
		"initialBalance":       "1000.00",
		"includeInNetWorth":    true,
		"includeInDailyBudget": true,
	}, headers)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("create account status=%d body=%s", accountRec.Code, accountRec.Body.String())
	}
	var accountPayload map[string]any
	decodeJSONResponse(t, accountRec, &accountPayload)
	accountID, _ := accountPayload["id"].(string)

	categoryRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/categories", map[string]any{
		"name": "Food",
		"type": "flexible",
	}, headers)
	if categoryRec.Code != http.StatusCreated {
		t.Fatalf("create category status=%d body=%s", categoryRec.Code, categoryRec.Body.String())
	}
	var categoryPayload map[string]any
	decodeJSONResponse(t, categoryRec, &categoryPayload)
	categoryID, _ := categoryPayload["id"].(string)

	createPostedRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "posted",
		"amount":        "100.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-12",
		"accountFromId": accountID,
		"categoryId":    categoryID,
		"comment":       "Posted expense",
	}, headers)
	if createPostedRec.Code != http.StatusCreated {
		t.Fatalf("create posted transaction status=%d body=%s", createPostedRec.Code, createPostedRec.Body.String())
	}
	assertAccountBalance(t, fixture.router, token, accountID, "900.00")

	createPlannedRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "planned",
		"amount":        "50.00",
		"currency":      "RUB",
		"plannedAt":     "2026-04-20",
		"accountFromId": accountID,
		"categoryId":    categoryID,
		"comment":       "Planned expense",
	}, headers)
	if createPlannedRec.Code != http.StatusCreated {
		t.Fatalf("create planned transaction status=%d body=%s", createPlannedRec.Code, createPlannedRec.Body.String())
	}
	assertAccountBalance(t, fixture.router, token, accountID, "900.00")

	var plannedPayload map[string]any
	decodeJSONResponse(t, createPlannedRec, &plannedPayload)
	plannedID, _ := plannedPayload["id"].(string)

	cancelPlannedRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/"+plannedID+"/cancel", map[string]any{}, headers)
	if cancelPlannedRec.Code != http.StatusOK {
		t.Fatalf("cancel planned status=%d body=%s", cancelPlannedRec.Code, cancelPlannedRec.Body.String())
	}
	assertAccountBalance(t, fixture.router, token, accountID, "900.00")

	createPostedSecondRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "posted",
		"amount":        "30.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-13",
		"accountFromId": accountID,
		"categoryId":    categoryID,
		"comment":       "Posted expense 2",
	}, headers)
	if createPostedSecondRec.Code != http.StatusCreated {
		t.Fatalf("create second posted transaction status=%d body=%s", createPostedSecondRec.Code, createPostedSecondRec.Body.String())
	}
	assertAccountBalance(t, fixture.router, token, accountID, "870.00")

	var postedSecondPayload map[string]any
	decodeJSONResponse(t, createPostedSecondRec, &postedSecondPayload)
	postedSecondID, _ := postedSecondPayload["id"].(string)

	cancelPostedRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/"+postedSecondID+"/cancel", map[string]any{}, headers)
	if cancelPostedRec.Code != http.StatusOK {
		t.Fatalf("cancel posted status=%d body=%s", cancelPostedRec.Code, cancelPostedRec.Body.String())
	}
	assertAccountBalance(t, fixture.router, token, accountID, "900.00")
}

func TestTransactionsIntegrationOwnershipIsolation(t *testing.T) {
	fixture := newIntegratedCatalogTransactionsFixture(t)
	ownerToken := registerAndGetAccessToken(t, fixture.router, "txn-int-owner@example.com")
	foreignToken := registerAndGetAccessToken(t, fixture.router, "txn-int-foreign@example.com")
	ownerHeaders := map[string]string{"Authorization": "Bearer " + ownerToken}
	foreignHeaders := map[string]string{"Authorization": "Bearer " + foreignToken}

	accountRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":                 "Owner main",
		"type":                 "cash",
		"currency":             "RUB",
		"initialBalance":       "1000.00",
		"includeInNetWorth":    true,
		"includeInDailyBudget": true,
	}, ownerHeaders)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("create owner account status=%d body=%s", accountRec.Code, accountRec.Body.String())
	}
	var accountPayload map[string]any
	decodeJSONResponse(t, accountRec, &accountPayload)
	accountID, _ := accountPayload["id"].(string)

	categoryRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/categories", map[string]any{
		"name": "Owner category",
		"type": "flexible",
	}, ownerHeaders)
	if categoryRec.Code != http.StatusCreated {
		t.Fatalf("create owner category status=%d body=%s", categoryRec.Code, categoryRec.Body.String())
	}
	var categoryPayload map[string]any
	decodeJSONResponse(t, categoryRec, &categoryPayload)
	categoryID, _ := categoryPayload["id"].(string)

	transactionRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "posted",
		"amount":        "100.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-12",
		"accountFromId": accountID,
		"categoryId":    categoryID,
	}, ownerHeaders)
	if transactionRec.Code != http.StatusCreated {
		t.Fatalf("create owner transaction status=%d body=%s", transactionRec.Code, transactionRec.Body.String())
	}
	var transactionPayload map[string]any
	decodeJSONResponse(t, transactionRec, &transactionPayload)
	transactionID, _ := transactionPayload["id"].(string)

	foreignGet := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions/"+transactionID, nil, foreignHeaders)
	if foreignGet.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign get, got %d body=%s", foreignGet.Code, foreignGet.Body.String())
	}

	foreignCancel := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/"+transactionID+"/cancel", map[string]any{}, foreignHeaders)
	if foreignCancel.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign cancel, got %d body=%s", foreignCancel.Code, foreignCancel.Body.String())
	}

	foreignCreate := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "income",
		"status":        "posted",
		"amount":        "500.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-13",
		"accountToId":   accountID,
		"accountFromId": nil,
	}, foreignHeaders)
	if foreignCreate.Code == http.StatusCreated {
		t.Fatalf("expected foreign user cannot create transaction against owner account, got status=%d", foreignCreate.Code)
	}

	assertAccountBalance(t, fixture.router, ownerToken, accountID, "900.00")
}

type integratedCatalogTransactionsFixture struct {
	router http.Handler
	store  *catalogTestStore
}

func newIntegratedCatalogTransactionsFixture(t *testing.T) integratedCatalogTransactionsFixture {
	t.Helper()

	store := newCatalogTestStore(t)
	transactionRepo := newIntegratedTransactionRepo()
	accountRepo := &integratedAccountRepo{store: store}
	txm := &integratedTxManager{
		transactionRepo: transactionRepo,
		accountRepo:     accountRepo,
	}
	clock := &mutableClock{now: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)}
	idgen := &sequenceTransactionIDGenerator{}

	transactionCreate := appaccounting.NewCreateTransactionService(transactionRepo, accountRepo, txm, idgen, clock)
	transactionGet := appaccounting.NewGetTransactionService(transactionRepo)
	transactionList := appaccounting.NewListTransactionsService(transactionRepo)
	transactionPatch := appaccounting.NewPatchTransactionService(transactionRepo, txm, clock)
	transactionDelete := appaccounting.NewDeleteTransactionService(transactionRepo, accountRepo, txm, clock)
	transactionPost := appaccounting.NewPostTransactionService(transactionRepo, accountRepo, txm, clock)
	transactionCancel := appaccounting.NewCancelTransactionService(transactionRepo, accountRepo, txm, clock)
	transactionDuplicate := appaccounting.NewDuplicateTransactionService(transactionRepo, accountRepo, txm, idgen, clock)
	transactionBulkCreate := appaccounting.NewBulkCreateTransactionsService(transactionRepo, accountRepo, txm, idgen, clock)
	transactionBulkPatch := appaccounting.NewBulkPatchTransactionsService(transactionRepo, accountRepo, txm, clock)

	catalogHandler := transporthttp.NewCatalogHandler(transporthttp.CatalogHandlerDeps{
		AccountsCreate:              accountCreateUseCase{store: store},
		AccountsGet:                 accountGetUseCase{store: store},
		AccountsList:                accountListUseCase{store: store},
		AccountsSummary:             accountSummaryUseCase{store: store},
		AccountsArchive:             accountArchiveUseCase{store: store},
		AccountsRestore:             accountRestoreUseCase{store: store},
		AccountsUpdate:              accountUpdateUseCase{store: store},
		CategoriesCreate:            categoryCreateUseCase{store: store},
		CategoriesGet:               categoryGetUseCase{store: store},
		CategoriesList:              categoryListUseCase{store: store},
		CategoriesUpdate:            categoryUpdateUseCase{store: store},
		CategoriesArchive:           categoryArchiveUseCase{store: store},
		CategoriesRestore:           categoryRestoreUseCase{store: store},
		SubcategoriesCreate:         subcategoryCreateUseCase{store: store},
		SubcategoriesListByCategory: subcategoryListByCategoryUseCase{store: store},
		SubcategoriesUpdate:         subcategoryUpdateUseCase{store: store},
		SubcategoriesArchive:        subcategoryArchiveUseCase{store: store},
		SubcategoriesRestore:        subcategoryRestoreUseCase{store: store},
		SubcategoriesGet:            subcategoryGetUseCase{store: store},
		SubcategoriesList:           subcategoryListUseCase{store: store},
		TransactionsCreate:          transactionCreate,
		TransactionsGet:             transactionGet,
		TransactionsList:            transactionList,
		TransactionsPatch:           transactionPatch,
		TransactionsDelete:          transactionDelete,
		TransactionsPost:            transactionPost,
		TransactionsCancel:          transactionCancel,
		TransactionsDuplicate:       transactionDuplicate,
		TransactionsBulkCreate:      transactionBulkCreate,
		TransactionsBulkPatch:       transactionBulkPatch,
	})
	apiHandler := transporthttp.NewAPIHandler(catalogHandler)

	authFixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		StrictAPIHandler: apiHandler,
	})

	return integratedCatalogTransactionsFixture{
		router: authFixture.router,
		store:  store,
	}
}

func assertAccountBalance(
	t *testing.T,
	handler http.Handler,
	accessToken string,
	accountID string,
	expected string,
) {
	t.Helper()

	getRec := performJSONRequest(t, handler, http.MethodGet, "/api/v1/accounts/"+accountID, nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if getRec.Code != http.StatusOK {
		t.Fatalf("get account status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var payload map[string]any
	decodeJSONResponse(t, getRec, &payload)
	balance, _ := payload["balance"].(string)
	if balance != expected {
		t.Fatalf("expected balance %s, got %s", expected, balance)
	}
}

type sequenceTransactionIDGenerator struct {
	next int
}

func (g *sequenceTransactionIDGenerator) NewTransactionID() shared.TransactionID {
	g.next++
	return shared.TransactionID(fmt.Sprintf("txn-int-%d", g.next))
}

type integratedTransactionRepo struct {
	transactions map[shared.TransactionID]domaintransactions.Transaction
}

func newIntegratedTransactionRepo() *integratedTransactionRepo {
	return &integratedTransactionRepo{
		transactions: make(map[shared.TransactionID]domaintransactions.Transaction),
	}
}

func (r *integratedTransactionRepo) Create(_ context.Context, transaction domaintransactions.Transaction) error {
	r.transactions[transaction.ID()] = transaction
	return nil
}

func (r *integratedTransactionRepo) FindByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	transaction, ok := r.transactions[transactionID]
	if !ok || transaction.UserID() != userID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}
	return transaction, nil
}

func (r *integratedTransactionRepo) ListByUserID(
	_ context.Context,
	input appaccounting.ListTransactionsQuery,
) ([]domaintransactions.Transaction, error) {
	transactions := make([]domaintransactions.Transaction, 0, len(r.transactions))
	for _, transaction := range r.transactions {
		if transaction.UserID() != input.UserID {
			continue
		}
		if input.Type != nil && transaction.Type() != *input.Type {
			continue
		}
		if input.Status != nil && transaction.Status() != *input.Status {
			continue
		}
		if input.AccountID != nil {
			fromMatch := transaction.AccountFromID() != nil && *transaction.AccountFromID() == *input.AccountID
			toMatch := transaction.AccountToID() != nil && *transaction.AccountToID() == *input.AccountID
			if !fromMatch && !toMatch {
				continue
			}
		}
		if input.EffectiveFrom != nil || input.EffectiveTo != nil {
			effective := transaction.OccurredAt()
			if effective == nil {
				effective = transaction.PlannedAt()
			}
			if effective == nil {
				continue
			}
			if input.EffectiveFrom != nil && effective.Before(*input.EffectiveFrom) {
				continue
			}
			if input.EffectiveTo != nil && effective.After(*input.EffectiveTo) {
				continue
			}
		}
		transactions = append(transactions, transaction)
	}
	if input.Offset > 0 {
		if input.Offset >= len(transactions) {
			return []domaintransactions.Transaction{}, nil
		}
		transactions = transactions[input.Offset:]
	}
	if input.Limit > 0 && input.Limit < len(transactions) {
		transactions = transactions[:input.Limit]
	}
	return transactions, nil
}

func (r *integratedTransactionRepo) CountByUserID(
	_ context.Context,
	input appaccounting.ListTransactionsQuery,
) (int, error) {
	noPagination := input
	noPagination.Limit = 0
	noPagination.Offset = 0
	transactions, err := r.ListByUserID(context.Background(), noPagination)
	if err != nil {
		return 0, err
	}
	return len(transactions), nil
}

func (r *integratedTransactionRepo) UpdateByID(
	_ context.Context,
	transaction domaintransactions.Transaction,
	expectedUpdatedAt time.Time,
) error {
	current, ok := r.transactions[transaction.ID()]
	if !ok || current.UserID() != transaction.UserID() {
		return appaccounting.ErrTransactionNotFound
	}
	if !current.UpdatedAt().Equal(expectedUpdatedAt) {
		return appaccounting.ErrConcurrentTransactionUpdate
	}
	r.transactions[transaction.ID()] = transaction
	return nil
}

func (r *integratedTransactionRepo) DeleteByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
	expectedUpdatedAt time.Time,
) error {
	current, ok := r.transactions[transactionID]
	if !ok || current.UserID() != userID {
		return appaccounting.ErrTransactionNotFound
	}
	if !current.UpdatedAt().Equal(expectedUpdatedAt) {
		return appaccounting.ErrConcurrentTransactionUpdate
	}
	if current.Status() == domaintransactions.TransactionStatusPosted {
		return appaccounting.ErrPostedTransactionDeleteConflict
	}
	delete(r.transactions, transactionID)
	return nil
}

type integratedAccountRepo struct {
	store *catalogTestStore
}

func (r *integratedAccountRepo) FindByID(
	_ context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, ok := r.store.accounts[accountID]
	if !ok || account.UserID() != userID {
		return domainaccounting.Account{}, appaccounting.ErrAccountNotFound
	}
	return account, nil
}

func (r *integratedAccountRepo) UpdateByID(
	_ context.Context,
	account domainaccounting.Account,
	expectedUpdatedAt time.Time,
) error {
	current, ok := r.store.accounts[account.ID()]
	if !ok || current.UserID() != account.UserID() {
		return appaccounting.ErrAccountNotFound
	}
	if !current.UpdatedAt().Equal(expectedUpdatedAt) {
		return appaccounting.ErrConcurrentAccountUpdate
	}
	r.store.accounts[account.ID()] = account
	return nil
}

type integratedTxManager struct {
	transactionRepo *integratedTransactionRepo
	accountRepo     *integratedAccountRepo
	depth           int
}

func (m *integratedTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.depth++
	if m.depth > 1 {
		defer func() { m.depth-- }()
		return fn(ctx)
	}

	transactionsSnapshot := make(map[shared.TransactionID]domaintransactions.Transaction, len(m.transactionRepo.transactions))
	for id, transaction := range m.transactionRepo.transactions {
		transactionsSnapshot[id] = transaction
	}
	accountsSnapshot := make(map[shared.AccountID]domainaccounting.Account, len(m.accountRepo.store.accounts))
	for id, account := range m.accountRepo.store.accounts {
		accountsSnapshot[id] = account
	}

	err := fn(ctx)
	m.depth--
	if err != nil {
		m.transactionRepo.transactions = transactionsSnapshot
		m.accountRepo.store.accounts = accountsSnapshot
	}
	return err
}
