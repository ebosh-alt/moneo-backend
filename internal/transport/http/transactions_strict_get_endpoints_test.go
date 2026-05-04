package http_test

import (
	"net/http"
	"testing"

	domaintransactions "moneo/internal/domain/transactions"
)

func TestTransactionsStrictGetEndpointsRequireAuthentication(t *testing.T) {
	fixture := newTransactionsStrictRouterWithAuthFixture(t)
	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestTransactionsStrictGetEndpointsListFiltersAndGetByID(t *testing.T) {
	fixture := newTransactionsStrictRouterWithAuthFixture(t)
	token := registerAndGetAccessToken(t, fixture.router, "txn-strict-list@example.com")
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

	listRec := performJSONRequest(
		t,
		fixture.router,
		http.MethodGet,
		"/api/v1/transactions?month=2026-04&accountId=acc_main&type=expense&status=posted&q=groc&limit=50&offset=0&sort=occurredAt:desc",
		nil,
		headers,
	)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", listRec.Code, listRec.Body.String())
	}

	var listPayload paginatedEnvelope
	decodeJSONResponse(t, listRec, &listPayload)
	if listPayload.Pagination.Total != 1 {
		t.Fatalf("expected total=1, got %d", listPayload.Pagination.Total)
	}
	if len(listPayload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(listPayload.Items))
	}
	if listPayload.Items[0]["id"] != "txn_a" {
		t.Fatalf("expected txn_a in filtered response, got %v", listPayload.Items[0]["id"])
	}
	if listPayload.Items[0]["amount"] != "900.00" {
		t.Fatalf("expected amount 900.00, got %v", listPayload.Items[0]["amount"])
	}
	if listPayload.Items[0]["occurredAt"] != "2026-04-12" {
		t.Fatalf("expected occurredAt 2026-04-12, got %v", listPayload.Items[0]["occurredAt"])
	}

	getRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/transactions/txn_a", nil, headers)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", getRec.Code, getRec.Body.String())
	}

	var getPayload map[string]any
	decodeJSONResponse(t, getRec, &getPayload)
	if getPayload["id"] != "txn_a" {
		t.Fatalf("expected id txn_a, got %v", getPayload["id"])
	}
	if getPayload["amount"] != "900.00" {
		t.Fatalf("expected amount 900.00, got %v", getPayload["amount"])
	}
	if getPayload["occurredAt"] != "2026-04-12" {
		t.Fatalf("expected occurredAt 2026-04-12, got %v", getPayload["occurredAt"])
	}
}
