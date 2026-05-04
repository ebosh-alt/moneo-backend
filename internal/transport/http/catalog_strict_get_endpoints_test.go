package http_test

import (
	"net/http"
	"testing"
)

func TestCatalogStrictGetEndpointsSmoke(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-strict-get@example.com")
	userID := userIDFromToken(t, fixture, accessToken)

	accountID := store.mustCreateAccount(t, userID, "Main card")
	categoryID := store.mustCreateCategory(t, userID, "Food")
	subcategoryID := store.mustCreateSubcategory(t, userID, categoryID, "Groceries")

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	var accountsList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	rec := performJSONRequest(t, router, http.MethodGet, "/api/v1/accounts", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list accounts status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &accountsList)
	if len(accountsList.Items) != 1 || accountsList.Items[0].ID != string(accountID) {
		t.Fatalf("unexpected accounts list payload: %+v", accountsList.Items)
	}

	var accountGet struct {
		ID string `json:"id"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/accounts/"+string(accountID), nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected get account status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &accountGet)
	if accountGet.ID != string(accountID) {
		t.Fatalf("expected account id %q, got %q", accountID, accountGet.ID)
	}

	var summary struct {
		Currency string `json:"currency"`
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/accounts/summary?currency=RUB", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected summary status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &summary)
	if summary.Currency != "RUB" {
		t.Fatalf("expected summary currency RUB, got %q", summary.Currency)
	}
	if len(summary.Accounts) != 1 || summary.Accounts[0].ID != string(accountID) {
		t.Fatalf("unexpected summary accounts payload: %+v", summary.Accounts)
	}

	var categoriesList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/categories?includeSubcategories=true", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list categories status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &categoriesList)
	if len(categoriesList.Items) != 1 || categoriesList.Items[0].ID != string(categoryID) {
		t.Fatalf("unexpected categories list payload: %+v", categoriesList.Items)
	}

	var categoryGet struct {
		ID string `json:"id"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/categories/"+string(categoryID)+"?includeSubcategories=true", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected get category status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &categoryGet)
	if categoryGet.ID != string(categoryID) {
		t.Fatalf("expected category id %q, got %q", categoryID, categoryGet.ID)
	}

	var categorySubcategories struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/categories/"+string(categoryID)+"/subcategories", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list category subcategories status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &categorySubcategories)
	if len(categorySubcategories.Items) != 1 || categorySubcategories.Items[0].ID != string(subcategoryID) {
		t.Fatalf("unexpected category subcategories payload: %+v", categorySubcategories.Items)
	}

	var subcategoriesList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/subcategories", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list subcategories status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &subcategoriesList)
	if len(subcategoriesList.Items) != 1 || subcategoriesList.Items[0].ID != string(subcategoryID) {
		t.Fatalf("unexpected subcategories list payload: %+v", subcategoriesList.Items)
	}

	var subcategoryGet struct {
		ID string `json:"id"`
	}
	rec = performJSONRequest(t, router, http.MethodGet, "/api/v1/subcategories/"+string(subcategoryID), nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected get subcategory status 200, got %d", rec.Code)
	}
	decodeJSONResponse(t, rec, &subcategoryGet)
	if subcategoryGet.ID != string(subcategoryID) {
		t.Fatalf("expected subcategory id %q, got %q", subcategoryID, subcategoryGet.ID)
	}
}

func TestCatalogStrictGetEndpointsRequireAuth(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/accounts", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without auth, got %d", rec.Code)
	}
}
