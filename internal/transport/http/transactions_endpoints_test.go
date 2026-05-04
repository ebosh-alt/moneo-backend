package http_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	appaccounting "moneo/internal/app/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
	transporthttp "moneo/internal/transport/http"
)

func TestTransactionsCreateGetAndOwnershipIsolation(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	ownerToken := registerAndGetAccessToken(t, fixture.router, "txn-owner@example.com")
	foreignToken := registerAndGetAccessToken(t, fixture.router, "txn-foreign@example.com")

	createRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "posted",
		"amount":        "900.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-12",
		"accountFromId": "acc_main",
		"categoryId":    "cat_food",
		"subcategoryId": "sub_groceries",
		"comment":       "Groceries",
	}, map[string]string{
		"Authorization": "Bearer " + ownerToken,
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}

	var created map[string]any
	decodeJSONResponse(t, createRec, &created)
	transactionID, _ := created["id"].(string)
	if transactionID == "" {
		t.Fatal("expected non-empty transaction id")
	}
	if created["status"] != "posted" {
		t.Fatalf("expected posted status, got %v", created["status"])
	}
	if created["budgetMemberId"] != nil || created["incomeSourceId"] != nil || created["goalId"] != nil {
		t.Fatalf("expected reserved MVP2 refs as null, got %+v", created)
	}

	getRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions/"+transactionID, nil, map[string]string{
		"Authorization": "Bearer " + ownerToken,
	})
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", getRec.Code, getRec.Body.String())
	}

	foreignGetRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions/"+transactionID, nil, map[string]string{
		"Authorization": "Bearer " + foreignToken,
	})
	if foreignGetRec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for foreign access, got %d", foreignGetRec.Code)
	}
}

func TestTransactionsEndpointsRequireAuthentication(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestTransactionsCreateValidationAndBusinessRuleMapping(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-validation@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	unsupportedRefRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":           "expense",
		"status":         "posted",
		"amount":         "900.00",
		"currency":       "RUB",
		"accountFromId":  "acc_main",
		"categoryId":     "cat_food",
		"budgetMemberId": "bm_1",
	}, headers)
	if unsupportedRefRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", unsupportedRefRec.Code)
	}
	var unsupportedRefErr structuredErrorResponse
	decodeJSONResponse(t, unsupportedRefRec, &unsupportedRefErr)
	if unsupportedRefErr.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", unsupportedRefErr.Error.Code)
	}
	assertErrorDetailField(t, unsupportedRefErr.Error.Details, "budgetMemberId")

	cancelledCreateRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "cancelled",
		"amount":        "900.00",
		"currency":      "RUB",
		"accountFromId": "acc_main",
		"categoryId":    "cat_food",
	}, headers)
	if cancelledCreateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", cancelledCreateRec.Code)
	}

	businessRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "transfer",
		"status":        "planned",
		"amount":        "100.00",
		"currency":      "RUB",
		"accountFromId": "acc_main",
		"accountToId":   "acc_main",
	}, headers)
	if businessRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d, body=%s", businessRec.Code, businessRec.Body.String())
	}
	var businessErr structuredErrorResponse
	decodeJSONResponse(t, businessRec, &businessErr)
	if businessErr.Error.Code != "business_rule_violation" {
		t.Fatalf("expected business_rule_violation code, got %q", businessErr.Error.Code)
	}

	savingRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "saving",
		"status":        "planned",
		"amount":        "100.00",
		"currency":      "RUB",
		"accountFromId": "acc_main",
		"accountToId":   "acc_savings",
		"categoryId":    "cat_food",
	}, headers)
	if savingRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 for saving with accountToId, got %d, body=%s", savingRec.Code, savingRec.Body.String())
	}
}

func TestTransactionsListFiltersAndPagination(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-list@example.com")
	userID := userIDFromAuthFixture(t, fixture.auth, token)
	headers := map[string]string{"Authorization": "Bearer " + token}

	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_a",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPosted,
		AmountMinor:    900_00,
		OccurredAtDate: "2026-04-12",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
		Comment:        "Groceries",
	})
	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_b",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPosted,
		AmountMinor:    1500_00,
		OccurredAtDate: "2026-04-15",
		AccountFromID:  "acc_other",
		CategoryID:     "cat_food",
		Comment:        "Cafe",
	})
	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_c",
		Type:           domaintransactions.TransactionTypeTransfer,
		Status:         domaintransactions.TransactionStatusPlanned,
		AmountMinor:    500_00,
		OccurredAtDate: "2026-04-16",
		AccountFromID:  "acc_main",
		AccountToID:    "acc_savings",
		Comment:        "Move to savings",
	})

	listRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions?month=2026-04&accountId=acc_main&type=expense&status=posted&q=groc&limit=50&offset=0&sort=occurredAt:desc", nil, headers)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", listRec.Code, listRec.Body.String())
	}

	var payload paginatedEnvelope
	decodeJSONResponse(t, listRec, &payload)
	if payload.Pagination.Total != 1 {
		t.Fatalf("expected total=1, got %d", payload.Pagination.Total)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload.Items))
	}
	if payload.Items[0]["id"] != "txn_a" {
		t.Fatalf("expected txn_a in filtered response, got %v", payload.Items[0]["id"])
	}
}

func TestTransactionsPatchRules(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-patch@example.com")
	userID := userIDFromAuthFixture(t, fixture.auth, token)
	headers := map[string]string{"Authorization": "Bearer " + token}

	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_planned",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPlanned,
		AmountMinor:    900_00,
		OccurredAtDate: "2026-04-20",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
	})
	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_posted",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPosted,
		AmountMinor:    1200_00,
		OccurredAtDate: "2026-04-10",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
		Comment:        "Before patch",
	})

	plannedPatchRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/txn_planned", map[string]any{
		"type":          "transfer",
		"amount":        "5000.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-21",
		"plannedAt":     "2026-04-21",
		"accountFromId": "acc_main",
		"accountToId":   "acc_savings",
		"categoryId":    "",
		"subcategoryId": "",
		"comment":       "Move to savings",
	}, headers)
	if plannedPatchRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", plannedPatchRec.Code, plannedPatchRec.Body.String())
	}

	postedPatchAllowedRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/txn_posted", map[string]any{
		"occurredAt": "2026-04-13",
		"categoryId": "cat_food",
		"comment":    "Updated comment",
	}, headers)
	if postedPatchAllowedRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", postedPatchAllowedRec.Code, postedPatchAllowedRec.Body.String())
	}

	postedPatchForbiddenRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/txn_posted", map[string]any{
		"amount": "1300.00",
	}, headers)
	if postedPatchForbiddenRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", postedPatchForbiddenRec.Code, postedPatchForbiddenRec.Body.String())
	}

	postedPatchCurrencyRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/txn_posted", map[string]any{
		"currency": "RUB",
	}, headers)
	if postedPatchCurrencyRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409 for posted currency mutation, got %d, body=%s", postedPatchCurrencyRec.Code, postedPatchCurrencyRec.Body.String())
	}

	statusPatchRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/txn_planned", map[string]any{
		"status": "posted",
	}, headers)
	if statusPatchRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409 for status patch, got %d, body=%s", statusPatchRec.Code, statusPatchRec.Body.String())
	}
}

func TestTransactionsDeleteRules(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-delete@example.com")
	userID := userIDFromAuthFixture(t, fixture.auth, token)
	headers := map[string]string{"Authorization": "Bearer " + token}

	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_planned_delete",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPlanned,
		AmountMinor:    100_00,
		OccurredAtDate: "2026-04-11",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
	})
	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_posted_delete",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPosted,
		AmountMinor:    150_00,
		OccurredAtDate: "2026-04-11",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
	})

	deletePlannedRec := performJSONRequest(t, fixture.router, http.MethodDelete, "/api/v1/transactions/txn_planned_delete", nil, headers)
	if deletePlannedRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", deletePlannedRec.Code)
	}

	deletePostedRec := performJSONRequest(t, fixture.router, http.MethodDelete, "/api/v1/transactions/txn_posted_delete", nil, headers)
	if deletePostedRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", deletePostedRec.Code)
	}
}

func TestTransactionsPostCancelAndDuplicate(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-state@example.com")
	userID := userIDFromAuthFixture(t, fixture.auth, token)
	headers := map[string]string{"Authorization": "Bearer " + token}

	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_state",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPlanned,
		AmountMinor:    900_00,
		OccurredAtDate: "2026-04-12",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
		Comment:        "Groceries",
	})

	postRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/post", map[string]any{}, headers)
	if postRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", postRec.Code, postRec.Body.String())
	}

	postAgainRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/post", map[string]any{}, headers)
	if postAgainRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", postAgainRec.Code)
	}

	cancelRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/cancel", map[string]any{}, headers)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", cancelRec.Code, cancelRec.Body.String())
	}

	cancelAgainRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/cancel", map[string]any{}, headers)
	if cancelAgainRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", cancelAgainRec.Code)
	}

	postCancelledRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/post", map[string]any{}, headers)
	if postCancelledRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", postCancelledRec.Code)
	}

	duplicateDefaultRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/duplicate", map[string]any{}, headers)
	if duplicateDefaultRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", duplicateDefaultRec.Code, duplicateDefaultRec.Body.String())
	}
	var duplicateDefault map[string]any
	decodeJSONResponse(t, duplicateDefaultRec, &duplicateDefault)
	if duplicateDefault["status"] != "planned" {
		t.Fatalf("expected default duplicate status planned, got %v", duplicateDefault["status"])
	}

	duplicatePostedRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/txn_state/duplicate", map[string]any{
		"status": "posted",
	}, headers)
	if duplicatePostedRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", duplicatePostedRec.Code, duplicatePostedRec.Body.String())
	}
	var duplicatePosted map[string]any
	decodeJSONResponse(t, duplicatePostedRec, &duplicatePosted)
	if duplicatePosted["status"] != "posted" {
		t.Fatalf("expected posted duplicate status, got %v", duplicatePosted["status"])
	}
}

func TestTransactionsBulkCreateAndValidationDetails(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-bulk-create@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	invalidRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/bulk", map[string]any{
		"items": []map[string]any{
			{
				"type":          "expense",
				"status":        "posted",
				"amount":        "900.00",
				"currency":      "RUB",
				"accountFromId": "acc_main",
				"categoryId":    "cat_food",
			},
			{
				"type":          "expense",
				"status":        "posted",
				"amount":        "invalid",
				"currency":      "RUB",
				"accountFromId": "acc_main",
				"categoryId":    "cat_food",
			},
		},
	}, headers)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", invalidRec.Code)
	}
	var invalidPayload structuredErrorResponse
	decodeJSONResponse(t, invalidRec, &invalidPayload)
	assertErrorDetailField(t, invalidPayload.Error.Details, "items[1].amount")

	createRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/bulk", map[string]any{
		"items": []map[string]any{
			{
				"type":          "expense",
				"status":        "posted",
				"amount":        "900.00",
				"currency":      "RUB",
				"occurredAt":    "2026-04-12",
				"accountFromId": "acc_main",
				"categoryId":    "cat_food",
				"comment":       "Groceries",
			},
			{
				"type":          "transfer",
				"status":        "planned",
				"amount":        "5000.00",
				"currency":      "RUB",
				"occurredAt":    "2026-04-20",
				"plannedAt":     "2026-04-20",
				"accountFromId": "acc_main",
				"accountToId":   "acc_savings",
				"comment":       "Move to savings",
			},
		},
	}, headers)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	decodeJSONResponse(t, createRec, &payload)
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 created items, got %d", len(payload.Items))
	}
}

func TestTransactionsBulkCreateAllOrNothing(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-bulk-create-rollback@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/bulk", map[string]any{
		"items": []map[string]any{
			{
				"type":          "expense",
				"status":        "posted",
				"amount":        "900.00",
				"currency":      "RUB",
				"accountFromId": "acc_main",
				"categoryId":    "cat_food",
			},
			{
				"type":          "transfer",
				"status":        "planned",
				"amount":        "100.00",
				"currency":      "RUB",
				"accountFromId": "acc_main",
				"accountToId":   "acc_main",
			},
		},
	}, headers)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d, body=%s", rec.Code, rec.Body.String())
	}

	listRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions", nil, headers)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listRec.Code)
	}
	var listPayload paginatedEnvelope
	decodeJSONResponse(t, listRec, &listPayload)
	if listPayload.Pagination.Total != 0 {
		t.Fatalf("expected no created transactions after failed bulk, got total=%d", listPayload.Pagination.Total)
	}
}

func TestTransactionsBulkPatchAndConflictDetails(t *testing.T) {
	fixture := newTransactionsRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-bulk-patch@example.com")
	userID := userIDFromAuthFixture(t, fixture.auth, token)
	headers := map[string]string{"Authorization": "Bearer " + token}

	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_bp_1",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPosted,
		AmountMinor:    900_00,
		OccurredAtDate: "2026-04-12",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
		Comment:        "Old",
	})
	fixture.service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_bp_2",
		Type:           domaintransactions.TransactionTypeTransfer,
		Status:         domaintransactions.TransactionStatusPlanned,
		AmountMinor:    500_00,
		OccurredAtDate: "2026-04-20",
		AccountFromID:  "acc_main",
		AccountToID:    "acc_savings",
	})

	okRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/bulk", map[string]any{
		"items": []map[string]any{
			{
				"id":      "txn_bp_1",
				"comment": "Updated groceries",
			},
			{
				"id":     "txn_bp_2",
				"status": "posted",
			},
		},
	}, headers)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", okRec.Code, okRec.Body.String())
	}

	conflictRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/bulk", map[string]any{
		"items": []map[string]any{
			{
				"id":      "txn_bp_1",
				"comment": "Second update",
			},
			{
				"id":     "txn_bp_1",
				"amount": "1000.00",
			},
		},
	}, headers)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", conflictRec.Code, conflictRec.Body.String())
	}
	var conflictPayload structuredErrorResponse
	decodeJSONResponse(t, conflictRec, &conflictPayload)
	assertErrorDetailField(t, conflictPayload.Error.Details, "items[1].amount")

	getRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions/txn_bp_1", nil, headers)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getRec.Code)
	}
	var getPayload map[string]any
	decodeJSONResponse(t, getRec, &getPayload)
	if getPayload["comment"] != "Updated groceries" {
		t.Fatalf("expected rollback of failed bulk patch, comment=%v", getPayload["comment"])
	}
}

type transactionsRouterFixture struct {
	router  http.Handler
	auth    authEndpointsFixture
	service *transactionUseCases
}

func newTransactionsRouterWithAuthFixture(t *testing.T) transactionsRouterFixture {
	t.Helper()

	return newTransactionsStrictRouterWithAuthFixture(t)
}

func newTransactionsStrictRouterWithAuthFixture(t *testing.T) transactionsRouterFixture {
	t.Helper()

	service := newTransactionUseCases()
	catalogHandler := newTransactionsCatalogHandlerForTest(service)
	apiHandler := transporthttp.NewAPIHandler(nil, catalogHandler)
	authFixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		StrictAPIHandler: apiHandler,
	})

	return transactionsRouterFixture{
		router:  authFixture.router,
		auth:    authFixture,
		service: service,
	}
}

func newTransactionsCatalogHandlerForTest(service *transactionUseCases) *transporthttp.CatalogHandler {
	return transporthttp.NewCatalogHandler(transporthttp.CatalogHandlerDeps{
		TransactionsCreate:     service,
		TransactionsGet:        service,
		TransactionsList:       service,
		TransactionsPatch:      service,
		TransactionsDelete:     service,
		TransactionsPost:       service,
		TransactionsCancel:     service,
		TransactionsDuplicate:  service,
		TransactionsBulkCreate: service,
		TransactionsBulkPatch:  service,
	})
}

func userIDFromAuthFixture(t *testing.T, fixture authEndpointsFixture, token string) shared.UserID {
	t.Helper()

	userID, _, err := fixture.tokenService.VerifyAccessTokenIdentity(token)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	return userID
}

type transactionUseCases struct {
	now          time.Time
	seq          int
	transactions map[shared.TransactionID]domaintransactions.Transaction
}

func newTransactionUseCases() *transactionUseCases {
	return &transactionUseCases{
		now:          time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC),
		transactions: make(map[shared.TransactionID]domaintransactions.Transaction),
	}
}

func (u *transactionUseCases) Create(_ context.Context, input appaccounting.CreateTransactionInput) (domaintransactions.Transaction, error) {
	status := input.Status
	if status == "" {
		status = domaintransactions.TransactionStatusPlanned
	}

	occurredAt := cloneTime(input.OccurredAt)
	postedAt := (*time.Time)(nil)
	if status == domaintransactions.TransactionStatusPosted {
		if occurredAt == nil {
			now := u.nextNow()
			occurredAt = &now
		}
		postedAt = cloneTime(occurredAt)
	}

	now := u.nextNow()
	transaction, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             u.nextID(),
		UserID:         input.UserID,
		Type:           input.Type,
		Status:         status,
		Amount:         input.Amount,
		AccountFromID:  cloneAccountID(input.AccountFromID),
		AccountToID:    cloneAccountID(input.AccountToID),
		CategoryID:     cloneCategoryID(input.CategoryID),
		SubcategoryID:  cloneSubcategoryID(input.SubcategoryID),
		IncomeSourceID: nil,
		Comment:        cloneString(input.Comment),
		OccurredAt:     occurredAt,
		PlannedAt:      cloneTime(input.PlannedAt),
		PostedAt:       postedAt,
		CancelledAt:    nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}

	u.transactions[transaction.ID()] = transaction
	return transaction, nil
}

func (u *transactionUseCases) CreateBulk(
	ctx context.Context,
	input appaccounting.BulkCreateTransactionsInput,
) ([]domaintransactions.Transaction, error) {
	if len(input.Items) == 0 {
		return nil, &appaccounting.BulkItemError{Index: 0, Field: "items", Err: fmt.Errorf("items must contain at least one item")}
	}
	if len(input.Items) > 100 {
		return nil, &appaccounting.BulkItemError{Index: 0, Field: "items", Err: fmt.Errorf("items must not exceed 100")}
	}

	snapshot := copyTransactionMap(u.transactions)
	created := make([]domaintransactions.Transaction, 0, len(input.Items))
	for idx, item := range input.Items {
		transaction, err := u.Create(ctx, item)
		if err != nil {
			u.transactions = snapshot
			return nil, &appaccounting.BulkItemError{Index: idx, Field: "item", Err: err}
		}
		created = append(created, transaction)
	}
	return created, nil
}

func (u *transactionUseCases) GetByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	transaction, ok := u.transactions[transactionID]
	if !ok || transaction.UserID() != userID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}
	return transaction, nil
}

func (u *transactionUseCases) ListByUser(
	_ context.Context,
	input appaccounting.ListTransactionsQuery,
) ([]domaintransactions.Transaction, error) {
	items := make([]domaintransactions.Transaction, 0, len(u.transactions))
	for _, transaction := range u.transactions {
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
			matchesFrom := transaction.AccountFromID() != nil && *transaction.AccountFromID() == *input.AccountID
			matchesTo := transaction.AccountToID() != nil && *transaction.AccountToID() == *input.AccountID
			if !matchesFrom && !matchesTo {
				continue
			}
		}
		if input.CategoryID != nil {
			if transaction.CategoryID() == nil || *transaction.CategoryID() != *input.CategoryID {
				continue
			}
		}
		if input.SubcategoryID != nil {
			if transaction.SubcategoryID() == nil || *transaction.SubcategoryID() != *input.SubcategoryID {
				continue
			}
		}
		if input.EffectiveFrom != nil || input.EffectiveTo != nil {
			effective := effectiveAt(transaction)
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
		if input.OccurredFrom != nil || input.OccurredTo != nil {
			occurredAt := transaction.OccurredAt()
			if occurredAt == nil {
				continue
			}
			if input.OccurredFrom != nil && occurredAt.Before(*input.OccurredFrom) {
				continue
			}
			if input.OccurredTo != nil && occurredAt.After(*input.OccurredTo) {
				continue
			}
		}
		if input.Search != nil {
			comment := ""
			if transaction.Comment() != nil {
				comment = *transaction.Comment()
			}
			if !strings.Contains(strings.ToLower(comment), strings.ToLower(strings.TrimSpace(*input.Search))) {
				continue
			}
		}
		items = append(items, transaction)
	}

	sortMode := input.Sort
	if sortMode == "" {
		sortMode = appaccounting.TransactionsSortEffectiveAtDesc
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		switch sortMode {
		case appaccounting.TransactionsSortEffectiveAtAsc:
			return compareOptionalTimeAsc(effectiveAt(left), effectiveAt(right), left.CreatedAt(), right.CreatedAt(), left.ID(), right.ID())
		case appaccounting.TransactionsSortCreatedAtDesc:
			if !left.CreatedAt().Equal(right.CreatedAt()) {
				return left.CreatedAt().After(right.CreatedAt())
			}
		case appaccounting.TransactionsSortAmountDesc:
			if left.Amount().MinorUnits() != right.Amount().MinorUnits() {
				return left.Amount().MinorUnits() > right.Amount().MinorUnits()
			}
		default:
			return compareOptionalTimeDesc(effectiveAt(left), effectiveAt(right), left.CreatedAt(), right.CreatedAt(), left.ID(), right.ID())
		}
		return string(left.ID()) < string(right.ID())
	})

	if input.Offset > 0 {
		if input.Offset >= len(items) {
			return []domaintransactions.Transaction{}, nil
		}
		items = items[input.Offset:]
	}
	if input.Limit > 0 && input.Limit < len(items) {
		items = items[:input.Limit]
	}

	return items, nil
}

func (u *transactionUseCases) CountByUser(
	ctx context.Context,
	input appaccounting.ListTransactionsQuery,
) (int, error) {
	noPagination := input
	noPagination.Limit = 0
	noPagination.Offset = 0
	items, err := u.ListByUser(ctx, noPagination)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func (u *transactionUseCases) Patch(
	_ context.Context,
	input appaccounting.PatchTransactionInput,
) (domaintransactions.Transaction, error) {
	if input.StatusSet {
		return domaintransactions.Transaction{}, appaccounting.ErrPostedTransactionPatchConflict
	}

	current, ok := u.transactions[input.TransactionID]
	if !ok || current.UserID() != input.UserID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}
	if current.Status() == domaintransactions.TransactionStatusCancelled {
		return domaintransactions.Transaction{}, appaccounting.ErrCancelledTransactionPatchConflict
	}
	if current.Status() == domaintransactions.TransactionStatusPosted &&
		(input.TypeSet || input.StatusSet || input.AmountSet || input.CurrencySet || input.AccountFromIDSet || input.AccountToIDSet || input.IncomeSourceSet || input.PlannedAtSet) {
		return domaintransactions.Transaction{}, appaccounting.ErrPostedTransactionPatchConflict
	}

	nextType := current.Type()
	if input.TypeSet && input.Type != nil {
		nextType = *input.Type
	}
	nextStatus := current.Status()
	nextAmount := current.Amount()
	if input.AmountSet && input.Amount != nil {
		nextAmount = *input.Amount
	}

	nextAccountFromID := current.AccountFromID()
	if input.AccountFromIDSet {
		nextAccountFromID = cloneAccountID(input.AccountFromID)
	}
	nextAccountToID := current.AccountToID()
	if input.AccountToIDSet {
		nextAccountToID = cloneAccountID(input.AccountToID)
	}
	nextCategoryID := current.CategoryID()
	if input.CategoryIDSet {
		nextCategoryID = cloneCategoryID(input.CategoryID)
	}
	nextSubcategoryID := current.SubcategoryID()
	if input.SubcategoryIDSet {
		nextSubcategoryID = cloneSubcategoryID(input.SubcategoryID)
	}
	nextComment := current.Comment()
	if input.CommentSet {
		nextComment = cloneString(input.Comment)
	}
	nextOccurredAt := current.OccurredAt()
	if input.OccurredAtSet {
		nextOccurredAt = cloneTime(input.OccurredAt)
	}
	nextPlannedAt := current.PlannedAt()
	if input.PlannedAtSet {
		nextPlannedAt = cloneTime(input.PlannedAt)
	}

	now := u.nextNow()
	updated, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             current.ID(),
		UserID:         current.UserID(),
		Type:           nextType,
		Status:         nextStatus,
		Amount:         nextAmount,
		AccountFromID:  nextAccountFromID,
		AccountToID:    nextAccountToID,
		CategoryID:     nextCategoryID,
		SubcategoryID:  nextSubcategoryID,
		IncomeSourceID: current.IncomeSourceID(),
		Comment:        nextComment,
		OccurredAt:     nextOccurredAt,
		PlannedAt:      nextPlannedAt,
		PostedAt:       current.PostedAt(),
		CancelledAt:    current.CancelledAt(),
		CreatedAt:      current.CreatedAt(),
		UpdatedAt:      now,
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}
	u.transactions[updated.ID()] = updated
	return updated, nil
}

func (u *transactionUseCases) PatchBulk(
	ctx context.Context,
	input appaccounting.BulkPatchTransactionsInput,
) ([]domaintransactions.Transaction, error) {
	if len(input.Items) == 0 {
		return nil, &appaccounting.BulkItemError{Index: 0, Field: "items", Err: fmt.Errorf("items must contain at least one item")}
	}
	if len(input.Items) > 100 {
		return nil, &appaccounting.BulkItemError{Index: 0, Field: "items", Err: fmt.Errorf("items must not exceed 100")}
	}

	snapshot := copyTransactionMap(u.transactions)
	updated := make([]domaintransactions.Transaction, 0, len(input.Items))
	for idx, item := range input.Items {
		current, ok := u.transactions[item.TransactionID]
		if !ok || current.UserID() != item.UserID {
			u.transactions = snapshot
			return nil, &appaccounting.BulkItemError{Index: idx, Field: "id", Err: appaccounting.ErrTransactionNotFound}
		}

		patchedInput := item
		patchedInput.StatusSet = false
		patchedInput.Status = nil
		patched, patchErr := u.Patch(ctx, patchedInput)
		if patchErr != nil {
			u.transactions = snapshot
			field := "item"
			if item.AmountSet {
				field = "amount"
			} else if item.CurrencySet {
				field = "currency"
			} else if item.TypeSet {
				field = "type"
			} else if item.AccountFromIDSet {
				field = "accountFromId"
			} else if item.AccountToIDSet {
				field = "accountToId"
			} else if item.PlannedAtSet {
				field = "plannedAt"
			}
			return nil, &appaccounting.BulkItemError{Index: idx, Field: field, Err: patchErr}
		}

		next := patched
		if item.StatusSet {
			if item.Status == nil {
				u.transactions = snapshot
				return nil, &appaccounting.BulkItemError{Index: idx, Field: "status", Err: appaccounting.ErrPostedTransactionPatchConflict}
			}
			switch *item.Status {
			case domaintransactions.TransactionStatusPosted:
				posted, err := u.PostByID(ctx, item.UserID, item.TransactionID)
				if err != nil {
					u.transactions = snapshot
					return nil, &appaccounting.BulkItemError{Index: idx, Field: "status", Err: err}
				}
				next = posted
			case domaintransactions.TransactionStatusCancelled:
				cancelled, err := u.CancelByID(ctx, item.UserID, item.TransactionID)
				if err != nil {
					u.transactions = snapshot
					return nil, &appaccounting.BulkItemError{Index: idx, Field: "status", Err: err}
				}
				next = cancelled
			case domaintransactions.TransactionStatusPlanned:
				if current.Status() != domaintransactions.TransactionStatusPlanned {
					u.transactions = snapshot
					return nil, &appaccounting.BulkItemError{Index: idx, Field: "status", Err: appaccounting.ErrPostedTransactionPatchConflict}
				}
			}
		}
		updated = append(updated, next)
	}

	return updated, nil
}

func (u *transactionUseCases) DeleteByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	transaction, ok := u.transactions[transactionID]
	if !ok || transaction.UserID() != userID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}
	if transaction.Status() == domaintransactions.TransactionStatusPosted {
		return domaintransactions.Transaction{}, appaccounting.ErrPostedTransactionDeleteConflict
	}
	delete(u.transactions, transactionID)
	return transaction, nil
}

func (u *transactionUseCases) PostByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	current, ok := u.transactions[transactionID]
	if !ok || current.UserID() != userID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}

	now := u.nextNow()
	candidate, err := transactionWithOccurredAtIfMissing(current, now)
	if err != nil {
		return domaintransactions.Transaction{}, err
	}
	if err := (&candidate).Post(now); err != nil {
		if err == domaintransactions.ErrTransactionAlreadyPosted {
			return domaintransactions.Transaction{}, appaccounting.ErrTransactionAlreadyPosted
		}
		if err == domaintransactions.ErrTransactionAlreadyCancelled {
			return domaintransactions.Transaction{}, appaccounting.ErrTransactionAlreadyCancelled
		}
		return domaintransactions.Transaction{}, err
	}

	u.transactions[candidate.ID()] = candidate
	return candidate, nil
}

func (u *transactionUseCases) CancelByID(
	_ context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (domaintransactions.Transaction, error) {
	current, ok := u.transactions[transactionID]
	if !ok || current.UserID() != userID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}

	now := u.nextNow()
	candidate := current
	if err := (&candidate).Cancel(now); err != nil {
		if err == domaintransactions.ErrTransactionAlreadyCancelled {
			return domaintransactions.Transaction{}, appaccounting.ErrTransactionAlreadyCancelled
		}
		return domaintransactions.Transaction{}, err
	}
	u.transactions[candidate.ID()] = candidate
	return candidate, nil
}

func (u *transactionUseCases) DuplicateByID(
	_ context.Context,
	input appaccounting.DuplicateTransactionInput,
) (domaintransactions.Transaction, error) {
	source, ok := u.transactions[input.TransactionID]
	if !ok || source.UserID() != input.UserID {
		return domaintransactions.Transaction{}, appaccounting.ErrTransactionNotFound
	}

	status := domaintransactions.TransactionStatusPlanned
	if input.Status != nil {
		status = *input.Status
	}
	occurredAt := source.OccurredAt()
	if input.OccurredAt != nil {
		occurredAt = cloneTime(input.OccurredAt)
	}
	plannedAt := source.PlannedAt()
	if input.PlannedAt != nil {
		plannedAt = cloneTime(input.PlannedAt)
	}
	if plannedAt == nil && occurredAt != nil {
		plannedAt = cloneTime(occurredAt)
	}
	comment := source.Comment()
	if input.Comment != nil {
		comment = cloneString(input.Comment)
	}

	postedAt := (*time.Time)(nil)
	if status == domaintransactions.TransactionStatusPosted {
		if occurredAt == nil {
			now := u.nextNow()
			occurredAt = &now
		}
		postedAt = cloneTime(occurredAt)
	}

	now := u.nextNow()
	duplicated, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             u.nextID(),
		UserID:         input.UserID,
		Type:           source.Type(),
		Status:         status,
		Amount:         source.Amount(),
		AccountFromID:  source.AccountFromID(),
		AccountToID:    source.AccountToID(),
		CategoryID:     source.CategoryID(),
		SubcategoryID:  source.SubcategoryID(),
		IncomeSourceID: source.IncomeSourceID(),
		Comment:        comment,
		OccurredAt:     occurredAt,
		PlannedAt:      plannedAt,
		PostedAt:       postedAt,
		CancelledAt:    nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return domaintransactions.Transaction{}, err
	}
	u.transactions[duplicated.ID()] = duplicated
	return duplicated, nil
}

type transactionSeed struct {
	UserID         shared.UserID
	ID             shared.TransactionID
	Type           domaintransactions.TransactionType
	Status         domaintransactions.TransactionStatus
	AmountMinor    int64
	OccurredAtDate string
	PlannedAtDate  string
	AccountFromID  shared.AccountID
	AccountToID    shared.AccountID
	CategoryID     shared.CategoryID
	SubcategoryID  shared.SubcategoryID
	Comment        string
}

func (u *transactionUseCases) mustSeed(t *testing.T, seed transactionSeed) domaintransactions.Transaction {
	t.Helper()

	createdAt := u.nextNow()
	occurredAt := parseSeedDate(t, seed.OccurredAtDate)
	plannedAt := parseSeedDate(t, seed.PlannedAtDate)
	accountFromID := seedAccountIDPtr(seed.AccountFromID)
	accountToID := seedAccountIDPtr(seed.AccountToID)
	categoryID := seedCategoryIDPtr(seed.CategoryID)
	subcategoryID := seedSubcategoryIDPtr(seed.SubcategoryID)
	comment := seedStringPtr(seed.Comment)
	postedAt := (*time.Time)(nil)
	if seed.Status == domaintransactions.TransactionStatusPosted {
		postedAt = cloneTime(occurredAt)
		if postedAt == nil {
			postedAt = &createdAt
		}
	}

	transaction, err := domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
		ID:             seed.ID,
		UserID:         seed.UserID,
		Type:           seed.Type,
		Status:         seed.Status,
		Amount:         shared.NewMoney(seed.AmountMinor, shared.CurrencyRUB),
		AccountFromID:  accountFromID,
		AccountToID:    accountToID,
		CategoryID:     categoryID,
		SubcategoryID:  subcategoryID,
		IncomeSourceID: nil,
		Comment:        comment,
		OccurredAt:     occurredAt,
		PlannedAt:      plannedAt,
		PostedAt:       postedAt,
		CancelledAt:    nil,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	})
	if err != nil {
		t.Fatalf("seed transaction: %v", err)
	}
	u.transactions[transaction.ID()] = transaction
	return transaction
}

func (u *transactionUseCases) nextID() shared.TransactionID {
	u.seq++
	return shared.TransactionID(fmt.Sprintf("txn_%d", u.seq))
}

func (u *transactionUseCases) nextNow() time.Time {
	u.seq++
	return u.now.Add(time.Duration(u.seq) * time.Minute)
}

func transactionWithOccurredAtIfMissing(
	transaction domaintransactions.Transaction,
	postTime time.Time,
) (domaintransactions.Transaction, error) {
	occurredAt := transaction.OccurredAt()
	if occurredAt == nil {
		occurredAt = &postTime
	}
	return domaintransactions.NewTransaction(domaintransactions.NewTransactionParams{
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
		Comment:        transaction.Comment(),
		OccurredAt:     occurredAt,
		PlannedAt:      transaction.PlannedAt(),
		PostedAt:       transaction.PostedAt(),
		CancelledAt:    transaction.CancelledAt(),
		CreatedAt:      transaction.CreatedAt(),
		UpdatedAt:      postTime,
	})
}

func effectiveAt(transaction domaintransactions.Transaction) *time.Time {
	if transaction.OccurredAt() != nil {
		return transaction.OccurredAt()
	}
	return transaction.PlannedAt()
}

func compareOptionalTimeDesc(
	left *time.Time,
	right *time.Time,
	leftCreated time.Time,
	rightCreated time.Time,
	leftID shared.TransactionID,
	rightID shared.TransactionID,
) bool {
	if left == nil && right == nil {
		if !leftCreated.Equal(rightCreated) {
			return leftCreated.After(rightCreated)
		}
		return string(leftID) < string(rightID)
	}
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if !left.Equal(*right) {
		return left.After(*right)
	}
	if !leftCreated.Equal(rightCreated) {
		return leftCreated.After(rightCreated)
	}
	return string(leftID) < string(rightID)
}

func compareOptionalTimeAsc(
	left *time.Time,
	right *time.Time,
	leftCreated time.Time,
	rightCreated time.Time,
	leftID shared.TransactionID,
	rightID shared.TransactionID,
) bool {
	if left == nil && right == nil {
		if !leftCreated.Equal(rightCreated) {
			return leftCreated.Before(rightCreated)
		}
		return string(leftID) < string(rightID)
	}
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if !left.Equal(*right) {
		return left.Before(*right)
	}
	if !leftCreated.Equal(rightCreated) {
		return leftCreated.Before(rightCreated)
	}
	return string(leftID) < string(rightID)
}

func parseSeedDate(t *testing.T, raw string) *time.Time {
	t.Helper()
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		t.Fatalf("parse seed date %q: %v", raw, err)
	}
	utc := parsed.UTC()
	return &utc
}

func seedAccountIDPtr(value shared.AccountID) *shared.AccountID {
	if strings.TrimSpace(string(value)) == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func seedCategoryIDPtr(value shared.CategoryID) *shared.CategoryID {
	if strings.TrimSpace(string(value)) == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func seedSubcategoryIDPtr(value shared.SubcategoryID) *shared.SubcategoryID {
	if strings.TrimSpace(string(value)) == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func seedStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := strings.TrimSpace(*value)
	if cloned == "" {
		return nil
	}
	return &cloned
}

func copyTransactionMap(
	source map[shared.TransactionID]domaintransactions.Transaction,
) map[shared.TransactionID]domaintransactions.Transaction {
	cloned := make(map[shared.TransactionID]domaintransactions.Transaction, len(source))
	for id, transaction := range source {
		cloned[id] = transaction
	}
	return cloned
}
