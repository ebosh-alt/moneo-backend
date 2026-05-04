package http_test

import (
	"net/http"
	"testing"
)

func TestOpenAPIValidatorRejectsInvalidBodyWithStructuredError(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	accessToken := registerAndGetAccessToken(t, fixture.router, "openapi-invalid-body@example.com")

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":                 "Main card",
		"type":                 "debit_card",
		"currency":             "RUB",
		"initialBalance":       100.50,
		"includeInNetWorth":    true,
		"includeInDailyBudget": true,
	}, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", payload.Error.Code)
	}
	assertHasOneOfErrorFields(t, payload.Error.Details, "body", "initialBalance")
}

func TestOpenAPIBindingErrorForMissingQueryUsesStructuredEnvelope(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	accessToken := registerAndGetAccessToken(t, fixture.router, "openapi-missing-query@example.com")

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/accounts/summary", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", payload.Error.Code)
	}
	assertHasOneOfErrorFields(t, payload.Error.Details, "currency", "request")
}

func TestOpenAPIBindingErrorForInvalidQueryTypeUsesStructuredEnvelope(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	accessToken := registerAndGetAccessToken(t, fixture.router, "openapi-invalid-query@example.com")

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/accounts?limit=abc", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", payload.Error.Code)
	}
	assertHasOneOfErrorFields(t, payload.Error.Details, "limit", "request")
}

func assertHasOneOfErrorFields(t *testing.T, details []structuredFieldError, fields ...string) {
	t.Helper()

	for _, detail := range details {
		for _, field := range fields {
			if detail.Field == field {
				return
			}
		}
	}

	t.Fatalf("expected one of fields %v, got %#v", fields, details)
}
