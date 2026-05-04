package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"moneo/internal/transport/http/generated"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

func TestOpenAPIContractCatalogsKeyScenarios(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	validator := newOpenAPIContractResponseValidator(t)

	token := registerAndGetAccessToken(t, fixture.router, "openapi-contract-catalogs@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	createAccountRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":                 "Main card",
		"type":                 "debit_card",
		"currency":             "RUB",
		"initialBalance":       "1000.00",
		"includeInNetWorth":    true,
		"includeInDailyBudget": true,
	}, headers)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", createAccountRec.Code, createAccountRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":                 "Main card",
		"type":                 "debit_card",
		"currency":             "RUB",
		"initialBalance":       "1000.00",
		"includeInNetWorth":    true,
		"includeInDailyBudget": true,
	}, headers, createAccountRec)

	var createdAccount map[string]any
	decodeJSONResponse(t, createAccountRec, &createdAccount)
	accountID, _ := createdAccount["id"].(string)
	if accountID == "" {
		t.Fatal("expected non-empty account id")
	}

	getAccountPath := "/api/v1/accounts/" + accountID
	getAccountRec := performJSONRequest(t, fixture.router, http.MethodGet, getAccountPath, nil, headers)
	if getAccountRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", getAccountRec.Code, getAccountRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodGet, getAccountPath, nil, headers, getAccountRec)

	summaryPath := "/api/v1/accounts/summary?currency=RUB"
	summaryRec := performJSONRequest(t, fixture.router, http.MethodGet, summaryPath, nil, headers)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", summaryRec.Code, summaryRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodGet, summaryPath, nil, headers, summaryRec)

	missingCurrencySummaryRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/accounts/summary", nil, headers)
	if missingCurrencySummaryRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", missingCurrencySummaryRec.Code, missingCurrencySummaryRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodGet, "/api/v1/accounts/summary", nil, headers, missingCurrencySummaryRec)

	invalidCategoryBody := map[string]any{
		"name": "Food",
		"type": "unknown_type",
	}
	invalidCategoryRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/categories", invalidCategoryBody, headers)
	if invalidCategoryRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", invalidCategoryRec.Code, invalidCategoryRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodPost, "/api/v1/categories", invalidCategoryBody, headers, invalidCategoryRec)
}

func TestOpenAPIContractTransactionsKeyScenarios(t *testing.T) {
	fixture := newTransactionsStrictRouterWithAuthFixture(t)
	validator := newOpenAPIContractResponseValidator(t)

	token := registerAndGetAccessToken(t, fixture.router, "openapi-contract-transactions@example.com")
	headers := map[string]string{"Authorization": "Bearer " + token}

	createBody := map[string]any{
		"type":          "expense",
		"status":        "planned",
		"amount":        "900.00",
		"currency":      "RUB",
		"occurredAt":    "2026-04-12",
		"accountFromId": "acc_main",
		"categoryId":    "cat_food",
		"comment":       "Groceries",
	}
	createRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", createBody, headers)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodPost, "/api/v1/transactions", createBody, headers, createRec)

	var createdTransaction map[string]any
	decodeJSONResponse(t, createRec, &createdTransaction)
	transactionID, _ := createdTransaction["id"].(string)
	if transactionID == "" {
		t.Fatal("expected non-empty transaction id")
	}

	getPath := "/api/v1/transactions/" + transactionID
	getRec := performJSONRequest(t, fixture.router, http.MethodGet, getPath, nil, headers)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", getRec.Code, getRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodGet, getPath, nil, headers, getRec)

	listPath := "/api/v1/transactions?month=2026-04&limit=50&offset=0&sort=occurredAt:desc"
	listRec := performJSONRequest(t, fixture.router, http.MethodGet, listPath, nil, headers)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", listRec.Code, listRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodGet, listPath, nil, headers, listRec)

	patchBody := map[string]any{"comment": "Updated groceries"}
	patchRec := performJSONRequest(t, fixture.router, http.MethodPatch, getPath, patchBody, headers)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", patchRec.Code, patchRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodPatch, getPath, patchBody, headers, patchRec)

	deleteRec := performJSONRequest(t, fixture.router, http.MethodDelete, getPath, nil, headers)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d, body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodDelete, getPath, nil, headers, deleteRec)

	invalidMonthPath := "/api/v1/transactions?month=2026-4"
	invalidMonthRec := performJSONRequest(t, fixture.router, http.MethodGet, invalidMonthPath, nil, headers)
	if invalidMonthRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", invalidMonthRec.Code, invalidMonthRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodGet, invalidMonthPath, nil, headers, invalidMonthRec)

	invalidBody := map[string]any{
		"type":          "expense",
		"status":        "planned",
		"amount":        900.00,
		"currency":      "RUB",
		"occurredAt":    "2026-04-12",
		"accountFromId": "acc_main",
		"categoryId":    "cat_food",
	}
	invalidBodyRec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/transactions", invalidBody, headers)
	if invalidBodyRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", invalidBodyRec.Code, invalidBodyRec.Body.String())
	}
	validator.AssertResponse(t, http.MethodPost, "/api/v1/transactions", invalidBody, headers, invalidBodyRec)
}

type openAPIContractResponseValidator struct {
	router routers.Router
}

func newOpenAPIContractResponseValidator(t *testing.T) *openAPIContractResponseValidator {
	t.Helper()

	swagger, err := generated.GetSwagger()
	if err != nil {
		t.Fatalf("load embedded swagger: %v", err)
	}

	router, err := gorillamux.NewRouter(swagger)
	if err != nil {
		t.Fatalf("build swagger router: %v", err)
	}

	return &openAPIContractResponseValidator{router: router}
}

func (v *openAPIContractResponseValidator) AssertResponse(
	t *testing.T,
	method string,
	path string,
	requestBody any,
	headers map[string]string,
	response *httptest.ResponseRecorder,
) {
	t.Helper()

	var payload []byte
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatalf("marshal request body for contract validation: %v", err)
		}
		payload = data
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	route, pathParams, err := v.router.FindRoute(req)
	if err != nil {
		t.Fatalf("find openapi route for %s %s: %v", method, path, err)
	}

	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			AuthenticationFunc: func(_ context.Context, _ *openapi3filter.AuthenticationInput) error {
				return nil
			},
		},
	}

	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: requestValidationInput,
		Status:                 response.Code,
		Header:                 response.Result().Header,
	}
	responseValidationInput.SetBodyBytes(response.Body.Bytes())

	if err := openapi3filter.ValidateResponse(context.Background(), responseValidationInput); err != nil {
		t.Fatalf(
			"openapi response validation failed for %s %s with status=%d: %v; body=%s",
			method,
			path,
			response.Code,
			err,
			response.Body.String(),
		)
	}
}
