package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appaccounting "moneo/internal/app/accounting"
	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
	transporthttp "moneo/internal/transport/http"
	"moneo/internal/transport/http/generated"

	"github.com/gin-gonic/gin"
)

func TestTransactionsStrictWriteCreatePatchDelete(t *testing.T) {
	fixture := newTransactionsStrictRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-strict-write@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	createRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":          "expense",
		"status":        "planned",
		"amount":        "900.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-12",
		"accountFromId": "acc_main",
		"categoryId":    "cat_food",
		"comment":       "Groceries",
	}, headers)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	decodeJSONResponse(t, createRec, &created)
	transactionID, _ := created["id"].(string)
	if transactionID == "" {
		t.Fatal("expected non-empty transaction id")
	}

	patchRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/"+transactionID, map[string]any{
		"comment": "Updated groceries",
	}, headers)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", patchRec.Code, patchRec.Body.String())
	}

	var patched map[string]any
	decodeJSONResponse(t, patchRec, &patched)
	if patched["comment"] != "Updated groceries" {
		t.Fatalf("expected updated comment, got %v", patched["comment"])
	}

	deleteRec := performJSONRequest(t, fixture.router, http.MethodDelete, "/api/v1/transactions/"+transactionID, nil, headers)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d, body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	validationRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", map[string]any{
		"type":           "expense",
		"status":         "posted",
		"amount":         "900.00",
		"currency":       "RUB",
		"accountFromId":  "acc_main",
		"categoryId":     "cat_food",
		"budgetMemberId": "bm_1",
	}, headers)
	if validationRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", validationRec.Code)
	}

	var validationErr structuredErrorResponse
	decodeJSONResponse(t, validationRec, &validationErr)
	if validationErr.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", validationErr.Error.Code)
	}
	assertErrorDetailField(t, validationErr.Error.Details, "budgetMemberId")
}

func TestTransactionsStrictWritePostCancelDuplicate(t *testing.T) {
	fixture := newTransactionsStrictRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-strict-state@example.com")
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
		t.Fatalf("expected planned duplicate status, got %v", duplicateDefault["status"])
	}
}

func TestTransactionsStrictWriteBulkEndpoints(t *testing.T) {
	fixture := newTransactionsStrictRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-strict-bulk@example.com")
	userID := userIDFromAuthFixture(t, fixture.auth, token)
	headers := map[string]string{"Authorization": "Bearer " + token}

	invalidBulkRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions/bulk", map[string]any{
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
	if invalidBulkRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", invalidBulkRec.Code)
	}
	var invalidBulkPayload structuredErrorResponse
	decodeJSONResponse(t, invalidBulkRec, &invalidBulkPayload)
	assertErrorDetailField(t, invalidBulkPayload.Error.Details, "items[1].amount")

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

	okPatchRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/bulk", map[string]any{
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
	if okPatchRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", okPatchRec.Code, okPatchRec.Body.String())
	}

	conflictPatchRec := performJSONRequest(t, fixture.router, http.MethodPatch, "/api/v1/transactions/bulk", map[string]any{
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
	if conflictPatchRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", conflictPatchRec.Code, conflictPatchRec.Body.String())
	}

	var conflictPatchPayload structuredErrorResponse
	decodeJSONResponse(t, conflictPatchRec, &conflictPatchPayload)
	assertErrorDetailField(t, conflictPatchPayload.Error.Details, "items[1].amount")

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

func TestAPIHandlerDeleteTransactionNoContentDecode(t *testing.T) {
	service := newTransactionUseCases()
	catalogHandler := newTransactionsCatalogHandlerForTest(service)
	apiHandler := transporthttp.NewAPIHandler(catalogHandler)

	userID := shared.UserID("user-delete")
	service.mustSeed(t, transactionSeed{
		UserID:         userID,
		ID:             "txn_delete",
		Type:           domaintransactions.TransactionTypeExpense,
		Status:         domaintransactions.TransactionStatusPlanned,
		AmountMinor:    100_00,
		OccurredAtDate: "2026-04-12",
		AccountFromID:  "acc_main",
		CategoryID:     "cat_food",
	})

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/transactions/txn_delete", nil)
	ctx.Params = gin.Params{{Key: "transactionId", Value: "txn_delete"}}
	ctx.Set("auth.user", domainidentity.User{
		ID:        userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	resp, err := apiHandler.DeleteTransaction(ctx, generated.DeleteTransactionRequestObject{
		TransactionId: generated.TransactionId("txn_delete"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := resp.(generated.DeleteTransaction204Response); !ok {
		t.Fatalf("expected DeleteTransaction204Response, got %T", resp)
	}

	if _, err := service.GetByID(ctx, userID, shared.TransactionID("txn_delete")); err == nil {
		t.Fatal("expected transaction to be deleted")
	} else if err != appaccounting.ErrTransactionNotFound {
		t.Fatalf("expected ErrTransactionNotFound after delete, got %v", err)
	}
}
