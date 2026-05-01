package http_test

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	appaccounting "moneo/internal/app/accounting"
	appcatalog "moneo/internal/app/catalog"
	domainaccounting "moneo/internal/domain/accounting"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
	transporthttp "moneo/internal/transport/http"
)

func TestCatalogValidationErrorsIncludeFieldLevelDetails(t *testing.T) {
	store := newCatalogTestStore(t)
	router := newCatalogRouterWithAuthFixture(t, store).router
	accessToken := registerAndGetAccessToken(t, router, "catalog-validation@example.com")

	rec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":           " ",
		"type":           "invalid",
		"currency":       "BTC",
		"initialBalance": "100,50",
	}, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", payload.Error.Code)
	}
	if payload.Error.Message != "Validation failed" {
		t.Fatalf("expected validation message, got %q", payload.Error.Message)
	}

	assertErrorDetailField(t, payload.Error.Details, "name")
	assertErrorDetailField(t, payload.Error.Details, "type")
	assertErrorDetailField(t, payload.Error.Details, "currency")
	assertErrorDetailField(t, payload.Error.Details, "initialBalance")
	assertErrorDetailField(t, payload.Error.Details, "includeInNetWorth")
	assertErrorDetailField(t, payload.Error.Details, "includeInDailyBudget")
}

func TestCatalogMoneyParsingRejectsNonStringInitialBalance(t *testing.T) {
	store := newCatalogTestStore(t)
	router := newCatalogRouterWithAuthFixture(t, store).router
	accessToken := registerAndGetAccessToken(t, router, "catalog-money@example.com")

	rec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts", map[string]any{
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
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestCatalogOwnershipReturnsNotFoundForForeignResources(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	ownerToken := registerAndGetAccessToken(t, router, "owner@example.com")
	foreignToken := registerAndGetAccessToken(t, router, "foreign@example.com")

	ownerID := userIDFromToken(t, fixture, ownerToken)
	foreignID := userIDFromToken(t, fixture, foreignToken)

	accountID := store.mustCreateAccount(t, ownerID, "Owner account")
	categoryID := store.mustCreateCategory(t, ownerID, "Owner category")
	subcategoryID := store.mustCreateSubcategory(t, ownerID, categoryID, "Owner subcategory")

	if ownerID == foreignID {
		t.Fatal("expected distinct owner and foreign users")
	}

	testCases := []string{
		"/api/v1/accounts/" + string(accountID),
		"/api/v1/categories/" + string(categoryID),
		"/api/v1/subcategories/" + string(subcategoryID),
	}

	for _, path := range testCases {
		t.Run(path, func(t *testing.T) {
			rec := performJSONRequest(t, router, http.MethodGet, path, nil, map[string]string{
				"Authorization": "Bearer " + foreignToken,
			})
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d", rec.Code)
			}

			var payload structuredErrorResponse
			decodeJSONResponse(t, rec, &payload)
			if payload.Error.Code != "not_found" {
				t.Fatalf("expected not_found code, got %q", payload.Error.Code)
			}
		})
	}
}

func TestCatalogListResponsesUseItemsAndPaginationShape(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-list@example.com")
	userID := userIDFromToken(t, fixture, accessToken)

	categoryID := store.mustCreateCategory(t, userID, "Food")
	store.mustCreateSubcategory(t, userID, categoryID, "Groceries")
	store.mustCreateSubcategory(t, userID, categoryID, "Restaurants")
	store.mustCreateAccount(t, userID, "Main card")
	store.mustCreateAccount(t, userID, "Cash")
	store.mustCreateAccount(t, userID, "Savings")

	testCases := []string{
		"/api/v1/accounts?limit=2&offset=1",
		"/api/v1/categories?limit=2&offset=0",
		"/api/v1/subcategories?limit=1&offset=0",
	}

	for _, path := range testCases {
		t.Run(path, func(t *testing.T) {
			rec := performJSONRequest(t, router, http.MethodGet, path, nil, map[string]string{
				"Authorization": "Bearer " + accessToken,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			var payload paginatedEnvelope
			decodeJSONResponse(t, rec, &payload)
			if payload.Items == nil {
				t.Fatal("expected items field in list response")
			}
			if payload.Pagination.Limit <= 0 {
				t.Fatalf("expected positive pagination limit, got %d", payload.Pagination.Limit)
			}
			if payload.Pagination.Offset < 0 {
				t.Fatalf("expected non-negative pagination offset, got %d", payload.Pagination.Offset)
			}
			if payload.Pagination.Total < len(payload.Items) {
				t.Fatalf("expected total >= items length, got total=%d items=%d", payload.Pagination.Total, len(payload.Items))
			}
		})
	}
}

func TestAccountsListAppliesTypeCurrencyAndSortFilters(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-filter@example.com")
	userID := userIDFromToken(t, fixture, accessToken)

	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    userID,
		Name:      "Z card",
		Type:      domainaccounting.AccountTypeDebitCard,
		Currency:  shared.CurrencyRUB,
		Balance:   100_00,
		Initial:   100_00,
		CreatedAt: time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    userID,
		Name:      "A cash",
		Type:      domainaccounting.AccountTypeCash,
		Currency:  shared.CurrencyRUB,
		Balance:   200_00,
		Initial:   200_00,
		CreatedAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    userID,
		Name:      "USD savings",
		Type:      domainaccounting.AccountTypeSavings,
		Currency:  shared.CurrencyUSD,
		Balance:   300_00,
		Initial:   300_00,
		CreatedAt: time.Date(2026, 4, 28, 11, 0, 0, 0, time.UTC),
	})

	rec := performJSONRequest(t, router, http.MethodGet, "/api/v1/accounts?type=cash&currency=RUB&sort=name:asc", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload paginatedEnvelope
	decodeJSONResponse(t, rec, &payload)
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 filtered account, got %d", len(payload.Items))
	}
	if payload.Items[0]["name"] != "A cash" {
		t.Fatalf("expected filtered account A cash, got %v", payload.Items[0]["name"])
	}
}

func TestAccountsSummaryCalculatesBucketsAndAppliesCurrencyFilter(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	ownerToken := registerAndGetAccessToken(t, router, "catalog-summary-owner@example.com")
	foreignToken := registerAndGetAccessToken(t, router, "catalog-summary-foreign@example.com")

	ownerID := userIDFromToken(t, fixture, ownerToken)
	foreignID := userIDFromToken(t, fixture, foreignToken)

	archivedAt := time.Date(2026, 4, 28, 11, 30, 0, 0, time.UTC)
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    ownerID,
		Name:      "Main card",
		Type:      domainaccounting.AccountTypeDebitCard,
		Currency:  shared.CurrencyRUB,
		Balance:   120_000_00,
		Initial:   120_000_00,
		CreatedAt: time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:               ownerID,
		Name:                 "Cash wallet",
		Type:                 domainaccounting.AccountTypeCash,
		Currency:             shared.CurrencyRUB,
		Balance:              60_000_00,
		Initial:              60_000_00,
		IncludeInNetWorth:    boolPtr(false),
		IncludeInDailyBudget: boolPtr(true),
		CreatedAt:            time.Date(2026, 4, 28, 9, 10, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:               ownerID,
		Name:                 "Savings",
		Type:                 domainaccounting.AccountTypeSavings,
		Currency:             shared.CurrencyRUB,
		Balance:              50_000_00,
		Initial:              50_000_00,
		IncludeInNetWorth:    boolPtr(true),
		IncludeInDailyBudget: boolPtr(false),
		CreatedAt:            time.Date(2026, 4, 28, 9, 20, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    ownerID,
		Name:      "Deposit",
		Type:      domainaccounting.AccountTypeDeposit,
		Currency:  shared.CurrencyRUB,
		Balance:   30_000_00,
		Initial:   30_000_00,
		CreatedAt: time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:               ownerID,
		Name:                 "Credit",
		Type:                 domainaccounting.AccountTypeCreditCard,
		Currency:             shared.CurrencyRUB,
		Balance:              40_000_00,
		Initial:              40_000_00,
		IncludeInNetWorth:    boolPtr(true),
		IncludeInDailyBudget: boolPtr(false),
		CreatedAt:            time.Date(2026, 4, 28, 9, 40, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:               ownerID,
		Name:                 "Debt",
		Type:                 domainaccounting.AccountTypeDebt,
		Currency:             shared.CurrencyRUB,
		Balance:              10_000_00,
		Initial:              10_000_00,
		IncludeInNetWorth:    boolPtr(false),
		IncludeInDailyBudget: boolPtr(false),
		CreatedAt:            time.Date(2026, 4, 28, 9, 50, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:               ownerID,
		Name:                 "Archived RUB",
		Type:                 domainaccounting.AccountTypeCash,
		Currency:             shared.CurrencyRUB,
		Balance:              999_00,
		Initial:              999_00,
		IncludeInNetWorth:    boolPtr(true),
		IncludeInDailyBudget: boolPtr(true),
		ArchivedAt:           &archivedAt,
		CreatedAt:            time.Date(2026, 4, 28, 8, 0, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    ownerID,
		Name:      "USD card",
		Type:      domainaccounting.AccountTypeDebitCard,
		Currency:  shared.CurrencyUSD,
		Balance:   777_00,
		Initial:   777_00,
		CreatedAt: time.Date(2026, 4, 28, 9, 55, 0, 0, time.UTC),
	})
	store.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    foreignID,
		Name:      "Foreign account",
		Type:      domainaccounting.AccountTypeDebitCard,
		Currency:  shared.CurrencyRUB,
		Balance:   888_000_00,
		Initial:   888_000_00,
		CreatedAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
	})

	rec := performJSONRequest(t, router, http.MethodGet, "/api/v1/accounts/summary?currency=RUB", nil, map[string]string{
		"Authorization": "Bearer " + ownerToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Currency                string `json:"currency"`
		NetWorth                string `json:"netWorth"`
		CashBalance             string `json:"cashBalance"`
		AvailableForDailyBudget string `json:"availableForDailyBudget"`
		CreditLiabilities       string `json:"creditLiabilities"`
		Accounts                []struct {
			Name string `json:"name"`
		} `json:"accounts"`
	}
	decodeJSONResponse(t, rec, &payload)

	if payload.Currency != "RUB" {
		t.Fatalf("expected summary currency RUB, got %q", payload.Currency)
	}
	if payload.NetWorth != "190000.00" {
		t.Fatalf("expected netWorth 190000.00, got %q", payload.NetWorth)
	}
	if payload.CashBalance != "260000.00" {
		t.Fatalf("expected cashBalance 260000.00, got %q", payload.CashBalance)
	}
	if payload.AvailableForDailyBudget != "210000.00" {
		t.Fatalf("expected availableForDailyBudget 210000.00, got %q", payload.AvailableForDailyBudget)
	}
	if payload.CreditLiabilities != "50000.00" {
		t.Fatalf("expected creditLiabilities 50000.00, got %q", payload.CreditLiabilities)
	}
	if len(payload.Accounts) != 6 {
		t.Fatalf("expected 6 active RUB accounts, got %d", len(payload.Accounts))
	}

	for _, item := range payload.Accounts {
		if item.Name == "Archived RUB" {
			t.Fatal("archived account must be excluded from summary")
		}
		if item.Name == "USD card" {
			t.Fatal("different currency account must be excluded from summary")
		}
		if item.Name == "Foreign account" {
			t.Fatal("foreign account must be excluded from summary")
		}
	}
}

func TestAccountsSummaryValidatesCurrency(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-summary-validation@example.com")
	rec := performJSONRequest(t, router, http.MethodGet, "/api/v1/accounts/summary", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestCreateCategoryValidationAndDuplicateActiveNameConflict(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-category-create@example.com")
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	invalidRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/categories", map[string]any{
		"name":  " ",
		"type":  "invalid",
		"color": "#ZZZZZZ",
	}, headers)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", invalidRec.Code)
	}

	var invalidPayload structuredErrorResponse
	decodeJSONResponse(t, invalidRec, &invalidPayload)
	if invalidPayload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error, got %q", invalidPayload.Error.Code)
	}
	assertErrorDetailField(t, invalidPayload.Error.Details, "name")
	assertErrorDetailField(t, invalidPayload.Error.Details, "type")
	assertErrorDetailField(t, invalidPayload.Error.Details, "color")

	createRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/categories", map[string]any{
		"name":      "Food",
		"type":      "required",
		"color":     "#2F80ED",
		"sortOrder": 100,
	}, headers)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}

	duplicateRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/categories", map[string]any{
		"name": "food",
		"type": "required",
	}, headers)
	if duplicateRec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", duplicateRec.Code)
	}

	var duplicatePayload structuredErrorResponse
	decodeJSONResponse(t, duplicateRec, &duplicatePayload)
	if duplicatePayload.Error.Code != "conflict" {
		t.Fatalf("expected conflict, got %q", duplicatePayload.Error.Code)
	}
	if duplicatePayload.Error.Message != "Category with this name already exists" {
		t.Fatalf("expected conflict message, got %q", duplicatePayload.Error.Message)
	}
	assertErrorDetailField(t, duplicatePayload.Error.Details, "name")
}

func TestCategoriesIncludeSubcategoriesAndFilters(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-category-list@example.com")
	userID := userIDFromToken(t, fixture, accessToken)

	requiredCategoryID := store.mustCreateCategoryWithParams(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Food",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 200,
	})
	flexibleCategoryID := store.mustCreateCategoryWithParams(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Fun",
		Type:      domaincatalog.CategoryTypeFlexible,
		SortOrder: 100,
	})
	store.mustCreateSubcategory(t, userID, requiredCategoryID, "Groceries")
	store.mustCreateSubcategory(t, userID, requiredCategoryID, "Restaurants")
	store.mustCreateSubcategory(t, userID, flexibleCategoryID, "Cinema")

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	listWithSubcategoriesRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories?type=required&includeSubcategories=true&sort=name:asc",
		nil,
		headers,
	)
	if listWithSubcategoriesRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listWithSubcategoriesRec.Code)
	}
	var listWithSubcategories struct {
		Items []struct {
			ID            string `json:"id"`
			Subcategories []struct {
				ID         string  `json:"id"`
				CategoryID string  `json:"categoryId"`
				SortOrder  int     `json:"sortOrder"`
				IsArchived bool    `json:"isArchived"`
				ArchivedAt *string `json:"archivedAt"`
			} `json:"subcategories"`
		} `json:"items"`
	}
	decodeJSONResponse(t, listWithSubcategoriesRec, &listWithSubcategories)
	if len(listWithSubcategories.Items) != 1 {
		t.Fatalf("expected 1 required category, got %d", len(listWithSubcategories.Items))
	}
	if listWithSubcategories.Items[0].ID != string(requiredCategoryID) {
		t.Fatalf("expected required category id %q, got %q", requiredCategoryID, listWithSubcategories.Items[0].ID)
	}
	if len(listWithSubcategories.Items[0].Subcategories) != 2 {
		t.Fatalf("expected 2 required subcategories, got %d", len(listWithSubcategories.Items[0].Subcategories))
	}
	for i, subcategory := range listWithSubcategories.Items[0].Subcategories {
		if subcategory.CategoryID != string(requiredCategoryID) {
			t.Fatalf("subcategory[%d]: expected categoryId %q, got %q", i, requiredCategoryID, subcategory.CategoryID)
		}
		if subcategory.SortOrder <= 0 {
			t.Fatalf("subcategory[%d]: expected positive sortOrder, got %d", i, subcategory.SortOrder)
		}
		if subcategory.IsArchived {
			t.Fatalf("subcategory[%d]: expected active subcategory, got archived", i)
		}
		if subcategory.ArchivedAt != nil {
			t.Fatalf("subcategory[%d]: expected archivedAt=nil, got %v", i, subcategory.ArchivedAt)
		}
	}

	listWithoutSubcategoriesRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories?includeSubcategories=false",
		nil,
		headers,
	)
	if listWithoutSubcategoriesRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listWithoutSubcategoriesRec.Code)
	}
	var listWithoutSubcategories struct {
		Items []struct {
			Subcategories []any `json:"subcategories"`
		} `json:"items"`
	}
	decodeJSONResponse(t, listWithoutSubcategoriesRec, &listWithoutSubcategories)
	if len(listWithoutSubcategories.Items) == 0 {
		t.Fatal("expected non-empty categories list")
	}
	for _, item := range listWithoutSubcategories.Items {
		if len(item.Subcategories) != 0 {
			t.Fatal("expected subcategories to be omitted when includeSubcategories=false")
		}
	}

	getWithSubcategoriesRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories/"+string(requiredCategoryID)+"?includeSubcategories=true",
		nil,
		headers,
	)
	if getWithSubcategoriesRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getWithSubcategoriesRec.Code)
	}
	var getWithSubcategories struct {
		Subcategories []struct {
			CategoryID string  `json:"categoryId"`
			SortOrder  int     `json:"sortOrder"`
			IsArchived bool    `json:"isArchived"`
			ArchivedAt *string `json:"archivedAt"`
		} `json:"subcategories"`
	}
	decodeJSONResponse(t, getWithSubcategoriesRec, &getWithSubcategories)
	if len(getWithSubcategories.Subcategories) != 2 {
		t.Fatalf("expected 2 subcategories in get response, got %d", len(getWithSubcategories.Subcategories))
	}
	for i, subcategory := range getWithSubcategories.Subcategories {
		if subcategory.CategoryID != string(requiredCategoryID) {
			t.Fatalf("get subcategory[%d]: expected categoryId %q, got %q", i, requiredCategoryID, subcategory.CategoryID)
		}
		if subcategory.SortOrder <= 0 {
			t.Fatalf("get subcategory[%d]: expected positive sortOrder, got %d", i, subcategory.SortOrder)
		}
		if subcategory.IsArchived {
			t.Fatalf("get subcategory[%d]: expected active subcategory, got archived", i)
		}
		if subcategory.ArchivedAt != nil {
			t.Fatalf("get subcategory[%d]: expected archivedAt=nil, got %v", i, subcategory.ArchivedAt)
		}
	}
}

func TestPatchCategoryUpdatesMutableFields(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-category-patch@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	categoryID := store.mustCreateCategoryWithParams(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Food",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 100,
	})

	rec := performJSONRequest(t, router, http.MethodPatch, "/api/v1/categories/"+string(categoryID), map[string]any{
		"name":      "Products",
		"type":      "flexible",
		"color":     "#27AE60",
		"sortOrder": 110,
	}, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Name      string  `json:"name"`
		Type      string  `json:"type"`
		Color     *string `json:"color"`
		SortOrder int     `json:"sortOrder"`
	}
	decodeJSONResponse(t, rec, &payload)
	if payload.Name != "Products" {
		t.Fatalf("expected name Products, got %q", payload.Name)
	}
	if payload.Type != "flexible" {
		t.Fatalf("expected type flexible, got %q", payload.Type)
	}
	if payload.Color == nil || *payload.Color != "#27AE60" {
		t.Fatalf("expected color #27AE60, got %v", payload.Color)
	}
	if payload.SortOrder != 110 {
		t.Fatalf("expected sortOrder 110, got %d", payload.SortOrder)
	}
}

func TestCategorySoftArchiveCascadeAndRestore(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-category-archive@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	categoryID := store.mustCreateCategoryWithParams(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Food",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 100,
	})
	store.mustCreateSubcategory(t, userID, categoryID, "Groceries")
	store.mustCreateSubcategory(t, userID, categoryID, "Restaurants")

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	archiveRec := performJSONRequest(t, router, http.MethodDelete, "/api/v1/categories/"+string(categoryID), nil, headers)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("expected archive status 200, got %d, body=%s", archiveRec.Code, archiveRec.Body.String())
	}

	var archivedPayload struct {
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, archiveRec, &archivedPayload)
	if !archivedPayload.IsArchived {
		t.Fatal("expected category to be archived")
	}
	if archivedPayload.ArchivedAt == nil || strings.TrimSpace(*archivedPayload.ArchivedAt) == "" {
		t.Fatal("expected archivedAt in archive response")
	}

	getArchivedRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories/"+string(categoryID)+"?includeSubcategories=true",
		nil,
		headers,
	)
	if getArchivedRec.Code != http.StatusOK {
		t.Fatalf("expected get archived status 200, got %d", getArchivedRec.Code)
	}
	var getArchivedPayload struct {
		Subcategories []any `json:"subcategories"`
	}
	decodeJSONResponse(t, getArchivedRec, &getArchivedPayload)
	if len(getArchivedPayload.Subcategories) != 0 {
		t.Fatalf("expected archived category subcategories to be empty after cascade archive, got %d", len(getArchivedPayload.Subcategories))
	}

	activeListRec := performJSONRequest(t, router, http.MethodGet, "/api/v1/categories", nil, headers)
	if activeListRec.Code != http.StatusOK {
		t.Fatalf("expected active list status 200, got %d", activeListRec.Code)
	}
	var activeListPayload paginatedEnvelope
	decodeJSONResponse(t, activeListRec, &activeListPayload)
	if len(activeListPayload.Items) != 0 {
		t.Fatalf("expected archived category to be excluded from active list, got %d items", len(activeListPayload.Items))
	}

	allListRec := performJSONRequest(t, router, http.MethodGet, "/api/v1/categories?includeArchived=true", nil, headers)
	if allListRec.Code != http.StatusOK {
		t.Fatalf("expected includeArchived list status 200, got %d", allListRec.Code)
	}
	var allListPayload paginatedEnvelope
	decodeJSONResponse(t, allListRec, &allListPayload)
	if len(allListPayload.Items) != 1 {
		t.Fatalf("expected archived category in includeArchived list, got %d", len(allListPayload.Items))
	}

	restoreRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/categories/"+string(categoryID)+"/restore", nil, headers)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d, body=%s", restoreRec.Code, restoreRec.Body.String())
	}

	var restoredPayload struct {
		IsArchived bool `json:"isArchived"`
		ArchivedAt any  `json:"archivedAt"`
	}
	decodeJSONResponse(t, restoreRec, &restoredPayload)
	if restoredPayload.IsArchived {
		t.Fatal("expected category to be restored")
	}
	if restoredPayload.ArchivedAt != nil {
		t.Fatalf("expected restored archivedAt nil, got %v", restoredPayload.ArchivedAt)
	}

	getRestoredRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories/"+string(categoryID)+"?includeSubcategories=true",
		nil,
		headers,
	)
	if getRestoredRec.Code != http.StatusOK {
		t.Fatalf("expected get restored status 200, got %d", getRestoredRec.Code)
	}
	var getRestoredPayload struct {
		Subcategories []any `json:"subcategories"`
	}
	decodeJSONResponse(t, getRestoredRec, &getRestoredPayload)
	if len(getRestoredPayload.Subcategories) != 2 {
		t.Fatalf("expected restored category subcategories count 2, got %d", len(getRestoredPayload.Subcategories))
	}
}

func TestCategoryMutationOwnershipIsolation(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	ownerToken := registerAndGetAccessToken(t, router, "catalog-category-owner@example.com")
	foreignToken := registerAndGetAccessToken(t, router, "catalog-category-foreign@example.com")
	ownerID := userIDFromToken(t, fixture, ownerToken)
	categoryID := store.mustCreateCategory(t, ownerID, "Owner category")

	testCases := []struct {
		method string
		path   string
		body   map[string]any
	}{
		{method: http.MethodPatch, path: "/api/v1/categories/" + string(categoryID), body: map[string]any{"name": "Updated"}},
		{method: http.MethodDelete, path: "/api/v1/categories/" + string(categoryID)},
		{method: http.MethodPost, path: "/api/v1/categories/" + string(categoryID) + "/restore"},
	}

	for _, tc := range testCases {
		rec := performJSONRequest(t, router, tc.method, tc.path, tc.body, map[string]string{
			"Authorization": "Bearer " + foreignToken,
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404 for %s %s, got %d", tc.method, tc.path, rec.Code)
		}
		var payload structuredErrorResponse
		decodeJSONResponse(t, rec, &payload)
		if payload.Error.Code != "not_found" {
			t.Fatalf("expected not_found code for %s %s, got %q", tc.method, tc.path, payload.Error.Code)
		}
	}
}

func TestSubcategoryCRUDArchiveRestoreFlow(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-subcategory-crud@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	categoryID := store.mustCreateCategoryWithParams(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Food",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 100,
	})

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	invalidCreateRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/categories/"+string(categoryID)+"/subcategories",
		map[string]any{"name": " "},
		headers,
	)
	if invalidCreateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected create validation status 400, got %d", invalidCreateRec.Code)
	}
	var invalidCreatePayload structuredErrorResponse
	decodeJSONResponse(t, invalidCreateRec, &invalidCreatePayload)
	assertErrorDetailField(t, invalidCreatePayload.Error.Details, "name")

	createRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/categories/"+string(categoryID)+"/subcategories",
		map[string]any{
			"name":      "Groceries",
			"sortOrder": 120,
		},
		headers,
	)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}
	var createdPayload struct {
		ID         string  `json:"id"`
		CategoryID string  `json:"categoryId"`
		Name       string  `json:"name"`
		SortOrder  int     `json:"sortOrder"`
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, createRec, &createdPayload)
	if createdPayload.Name != "Groceries" {
		t.Fatalf("expected created subcategory name Groceries, got %q", createdPayload.Name)
	}
	if createdPayload.CategoryID != string(categoryID) {
		t.Fatalf("expected categoryId %q, got %q", categoryID, createdPayload.CategoryID)
	}
	if createdPayload.SortOrder != 120 {
		t.Fatalf("expected created sortOrder 120, got %d", createdPayload.SortOrder)
	}
	if createdPayload.IsArchived {
		t.Fatal("expected created subcategory to be active")
	}
	if createdPayload.ArchivedAt != nil {
		t.Fatalf("expected created archivedAt nil, got %v", createdPayload.ArchivedAt)
	}

	duplicateRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/categories/"+string(categoryID)+"/subcategories",
		map[string]any{"name": "groceries"},
		headers,
	)
	if duplicateRec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status 409, got %d", duplicateRec.Code)
	}
	var duplicatePayload structuredErrorResponse
	decodeJSONResponse(t, duplicateRec, &duplicatePayload)
	if duplicatePayload.Error.Code != "conflict" {
		t.Fatalf("expected conflict code, got %q", duplicatePayload.Error.Code)
	}
	assertErrorDetailField(t, duplicatePayload.Error.Details, "name")

	listRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories/"+string(categoryID)+"/subcategories?includeArchived=false",
		nil,
		headers,
	)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listRec.Code)
	}
	var listPayload paginatedEnvelope
	decodeJSONResponse(t, listRec, &listPayload)
	if len(listPayload.Items) != 1 {
		t.Fatalf("expected 1 active subcategory, got %d", len(listPayload.Items))
	}
	if listPayload.Items[0]["name"] != "Groceries" {
		t.Fatalf("expected listed name Groceries, got %v", listPayload.Items[0]["name"])
	}
	if listPayload.Items[0]["sortOrder"] != float64(120) {
		t.Fatalf("expected listed sortOrder 120, got %v", listPayload.Items[0]["sortOrder"])
	}

	subcategoryID := createdPayload.ID
	patchRec := performJSONRequest(
		t,
		router,
		http.MethodPatch,
		"/api/v1/subcategories/"+subcategoryID,
		map[string]any{
			"name":      "Supermarkets",
			"sortOrder": 130,
		},
		headers,
	)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch status 200, got %d, body=%s", patchRec.Code, patchRec.Body.String())
	}
	var patchedPayload struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sortOrder"`
	}
	decodeJSONResponse(t, patchRec, &patchedPayload)
	if patchedPayload.Name != "Supermarkets" {
		t.Fatalf("expected patched name Supermarkets, got %q", patchedPayload.Name)
	}
	if patchedPayload.SortOrder != 130 {
		t.Fatalf("expected patched sortOrder 130, got %d", patchedPayload.SortOrder)
	}

	archiveRec := performJSONRequest(t, router, http.MethodDelete, "/api/v1/subcategories/"+subcategoryID, nil, headers)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("expected delete/archive status 200, got %d, body=%s", archiveRec.Code, archiveRec.Body.String())
	}
	var archivedPayload struct {
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, archiveRec, &archivedPayload)
	if !archivedPayload.IsArchived {
		t.Fatal("expected subcategory archived after delete")
	}
	if archivedPayload.ArchivedAt == nil || strings.TrimSpace(*archivedPayload.ArchivedAt) == "" {
		t.Fatal("expected archivedAt in archived subcategory response")
	}

	activeAfterArchiveRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories/"+string(categoryID)+"/subcategories",
		nil,
		headers,
	)
	if activeAfterArchiveRec.Code != http.StatusOK {
		t.Fatalf("expected active list status 200, got %d", activeAfterArchiveRec.Code)
	}
	var activeAfterArchivePayload paginatedEnvelope
	decodeJSONResponse(t, activeAfterArchiveRec, &activeAfterArchivePayload)
	if len(activeAfterArchivePayload.Items) != 0 {
		t.Fatalf("expected no active subcategories after archive, got %d", len(activeAfterArchivePayload.Items))
	}

	withArchivedRec := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/api/v1/categories/"+string(categoryID)+"/subcategories?includeArchived=true",
		nil,
		headers,
	)
	if withArchivedRec.Code != http.StatusOK {
		t.Fatalf("expected includeArchived list status 200, got %d", withArchivedRec.Code)
	}
	var withArchivedPayload paginatedEnvelope
	decodeJSONResponse(t, withArchivedRec, &withArchivedPayload)
	if len(withArchivedPayload.Items) != 1 {
		t.Fatalf("expected 1 archived subcategory in includeArchived list, got %d", len(withArchivedPayload.Items))
	}
	if withArchivedPayload.Items[0]["isArchived"] != true {
		t.Fatalf("expected archived item isArchived=true, got %v", withArchivedPayload.Items[0]["isArchived"])
	}

	restoreRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/subcategories/"+subcategoryID+"/restore",
		nil,
		headers,
	)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d, body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	var restoredPayload struct {
		IsArchived bool `json:"isArchived"`
		ArchivedAt any  `json:"archivedAt"`
	}
	decodeJSONResponse(t, restoreRec, &restoredPayload)
	if restoredPayload.IsArchived {
		t.Fatal("expected restored subcategory to be active")
	}
	if restoredPayload.ArchivedAt != nil {
		t.Fatalf("expected restored archivedAt nil, got %v", restoredPayload.ArchivedAt)
	}
}

func TestSubcategoryOwnershipIsolation(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	ownerToken := registerAndGetAccessToken(t, router, "catalog-subcategory-owner@example.com")
	foreignToken := registerAndGetAccessToken(t, router, "catalog-subcategory-foreign@example.com")
	ownerID := userIDFromToken(t, fixture, ownerToken)

	categoryID := store.mustCreateCategory(t, ownerID, "Owner category")
	subcategoryID := store.mustCreateSubcategory(t, ownerID, categoryID, "Owner subcategory")

	headers := map[string]string{
		"Authorization": "Bearer " + foreignToken,
	}

	testCases := []struct {
		method string
		path   string
		body   map[string]any
	}{
		{method: http.MethodGet, path: "/api/v1/categories/" + string(categoryID) + "/subcategories"},
		{method: http.MethodPost, path: "/api/v1/categories/" + string(categoryID) + "/subcategories", body: map[string]any{"name": "Foreign create"}},
		{method: http.MethodPatch, path: "/api/v1/subcategories/" + string(subcategoryID), body: map[string]any{"name": "Foreign patch"}},
		{method: http.MethodDelete, path: "/api/v1/subcategories/" + string(subcategoryID)},
		{method: http.MethodPost, path: "/api/v1/subcategories/" + string(subcategoryID) + "/restore"},
	}

	for _, tc := range testCases {
		rec := performJSONRequest(t, router, tc.method, tc.path, tc.body, headers)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404 for %s %s, got %d", tc.method, tc.path, rec.Code)
		}
		var payload structuredErrorResponse
		decodeJSONResponse(t, rec, &payload)
		if payload.Error.Code != "not_found" {
			t.Fatalf("expected not_found code for %s %s, got %q", tc.method, tc.path, payload.Error.Code)
		}
	}
}

func TestSubcategoryCreateRejectsArchivedParentCategory(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-subcategory-create-archived-parent@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	categoryID := store.mustCreateCategory(t, userID, "Food")

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	archiveCategoryRec := performJSONRequest(t, router, http.MethodDelete, "/api/v1/categories/"+string(categoryID), nil, headers)
	if archiveCategoryRec.Code != http.StatusOK {
		t.Fatalf("expected archive category status 200, got %d", archiveCategoryRec.Code)
	}

	createRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/categories/"+string(categoryID)+"/subcategories",
		map[string]any{"name": "Groceries"},
		headers,
	)
	if createRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected create under archived parent status 422, got %d", createRec.Code)
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, createRec, &payload)
	if payload.Error.Code != "business_rule_violation" {
		t.Fatalf("expected business_rule_violation, got %q", payload.Error.Code)
	}
	assertErrorDetailField(t, payload.Error.Details, "categoryId")
}

func TestSubcategoryRestoreRejectsWhenParentCategoryArchived(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-subcategory-restore-archived-parent@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	categoryID := store.mustCreateCategory(t, userID, "Food")

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	createRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/categories/"+string(categoryID)+"/subcategories",
		map[string]any{"name": "Groceries"},
		headers,
	)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create subcategory status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}
	var createPayload struct {
		ID string `json:"id"`
	}
	decodeJSONResponse(t, createRec, &createPayload)

	archiveSubcategoryRec := performJSONRequest(
		t,
		router,
		http.MethodDelete,
		"/api/v1/subcategories/"+createPayload.ID,
		nil,
		headers,
	)
	if archiveSubcategoryRec.Code != http.StatusOK {
		t.Fatalf("expected archive subcategory status 200, got %d", archiveSubcategoryRec.Code)
	}

	archiveCategoryRec := performJSONRequest(
		t,
		router,
		http.MethodDelete,
		"/api/v1/categories/"+string(categoryID),
		nil,
		headers,
	)
	if archiveCategoryRec.Code != http.StatusOK {
		t.Fatalf("expected archive category status 200, got %d", archiveCategoryRec.Code)
	}

	restoreRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/subcategories/"+createPayload.ID+"/restore",
		nil,
		headers,
	)
	if restoreRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected restore with archived parent status 422, got %d", restoreRec.Code)
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, restoreRec, &payload)
	if payload.Error.Code != "business_rule_violation" {
		t.Fatalf("expected business_rule_violation code, got %q", payload.Error.Code)
	}
	assertErrorDetailField(t, payload.Error.Details, "categoryId")
}

func TestArchiveAndRestoreAccountFlowIsIdempotent(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-archive@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	accountID := store.mustCreateAccount(t, userID, "Main card")

	archiveRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts/"+string(accountID)+"/archive", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("expected archive status 200, got %d, body=%s", archiveRec.Code, archiveRec.Body.String())
	}

	var archivedPayload struct {
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, archiveRec, &archivedPayload)
	if !archivedPayload.IsArchived {
		t.Fatal("expected archived account")
	}
	if archivedPayload.ArchivedAt == nil || strings.TrimSpace(*archivedPayload.ArchivedAt) == "" {
		t.Fatal("expected archivedAt timestamp in archive response")
	}

	archiveAgainRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts/"+string(accountID)+"/archive", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if archiveAgainRec.Code != http.StatusOK {
		t.Fatalf("expected second archive status 200, got %d", archiveAgainRec.Code)
	}

	var archivedAgainPayload struct {
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, archiveAgainRec, &archivedAgainPayload)
	if !archivedAgainPayload.IsArchived {
		t.Fatal("expected archived account on second archive")
	}
	if archivedAgainPayload.ArchivedAt == nil || archivedPayload.ArchivedAt == nil {
		t.Fatal("expected archivedAt in both archive responses")
	}
	if *archivedAgainPayload.ArchivedAt != *archivedPayload.ArchivedAt {
		t.Fatalf("expected idempotent archive timestamp %q, got %q", *archivedPayload.ArchivedAt, *archivedAgainPayload.ArchivedAt)
	}

	restoreRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts/"+string(accountID)+"/restore", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d, body=%s", restoreRec.Code, restoreRec.Body.String())
	}

	var restoredPayload struct {
		IsArchived bool `json:"isArchived"`
		ArchivedAt any  `json:"archivedAt"`
	}
	decodeJSONResponse(t, restoreRec, &restoredPayload)
	if restoredPayload.IsArchived {
		t.Fatal("expected restored account to be active")
	}
	if restoredPayload.ArchivedAt != nil {
		t.Fatalf("expected archivedAt to be null after restore, got %v", restoredPayload.ArchivedAt)
	}
}

func TestArchiveRestoreAccountOwnershipIsolation(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	ownerToken := registerAndGetAccessToken(t, router, "catalog-archive-owner@example.com")
	foreignToken := registerAndGetAccessToken(t, router, "catalog-archive-foreign@example.com")
	ownerID := userIDFromToken(t, fixture, ownerToken)
	accountID := store.mustCreateAccount(t, ownerID, "Owner account")

	paths := []string{
		"/api/v1/accounts/" + string(accountID) + "/archive",
		"/api/v1/accounts/" + string(accountID) + "/restore",
	}

	for _, path := range paths {
		rec := performJSONRequest(t, router, http.MethodPost, path, nil, map[string]string{
			"Authorization": "Bearer " + foreignToken,
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404 for %s, got %d", path, rec.Code)
		}
		var payload structuredErrorResponse
		decodeJSONResponse(t, rec, &payload)
		if payload.Error.Code != "not_found" {
			t.Fatalf("expected not_found code for %s, got %q", path, payload.Error.Code)
		}
	}
}

func TestPatchAccountRejectsImmutableFields(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-patch@example.com")
	userID := userIDFromToken(t, fixture, accessToken)
	accountID := store.mustCreateAccount(t, userID, "Main card")

	rec := performJSONRequest(t, router, http.MethodPatch, "/api/v1/accounts/"+string(accountID), map[string]any{
		"currency":       "USD",
		"initialBalance": "100.00",
		"balance":        "100.00",
	}, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error, got %q", payload.Error.Code)
	}
	assertErrorDetailField(t, payload.Error.Details, "currency")
	assertErrorDetailField(t, payload.Error.Details, "initialBalance")
	assertErrorDetailField(t, payload.Error.Details, "balance")
}

type catalogRouterFixture struct {
	router  http.Handler
	auth    authEndpointsFixture
	service *catalogTestStore
}

func newCatalogRouterWithAuthFixture(t *testing.T, store *catalogTestStore) catalogRouterFixture {
	t.Helper()

	return newCatalogStrictRouterWithAuthFixture(t, store)
}

func newCatalogStrictRouterWithAuthFixture(t *testing.T, store *catalogTestStore) catalogRouterFixture {
	t.Helper()

	catalogHandler := newCatalogHandlerForTest(store)
	apiHandler := transporthttp.NewAPIHandler(nil, catalogHandler)

	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		StrictAPIHandler: apiHandler,
	})

	return catalogRouterFixture{
		router:  fixture.router,
		auth:    fixture,
		service: store,
	}
}

func newCatalogHandlerForTest(store *catalogTestStore) *transporthttp.CatalogHandler {
	return transporthttp.NewCatalogHandler(transporthttp.CatalogHandlerDeps{
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
	})
}

func registerAndGetAccessToken(t *testing.T, handler http.Handler, email string) string {
	t.Helper()

	rec := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            email,
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload authResponse
	decodeJSONResponse(t, rec, &payload)
	if strings.TrimSpace(payload.AccessToken) == "" {
		t.Fatal("access token must not be empty")
	}

	return payload.AccessToken
}

func userIDFromToken(t *testing.T, fixture catalogRouterFixture, token string) shared.UserID {
	t.Helper()

	userID, _, err := fixture.auth.tokenService.VerifyAccessTokenIdentity(token)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}

	return userID
}

func assertErrorDetailField(t *testing.T, details []structuredFieldError, field string) {
	t.Helper()

	for _, detail := range details {
		if detail.Field == field {
			return
		}
	}

	t.Fatalf("expected validation detail for field %q, got %+v", field, details)
}

type structuredErrorResponse struct {
	Error structuredErrorBody `json:"error"`
}

type structuredErrorBody struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details []structuredFieldError `json:"details"`
}

type structuredFieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type paginatedEnvelope struct {
	Items      []map[string]any `json:"items"`
	Pagination struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"pagination"`
}

type catalogTestStore struct {
	accountSeq     int
	categorySeq    int
	subcategorySeq int
	accounts       map[shared.AccountID]domainaccounting.Account
	categories     map[shared.CategoryID]domaincatalog.Category
	subcategories  map[shared.SubcategoryID]domaincatalog.Subcategory
}

func newCatalogTestStore(t *testing.T) *catalogTestStore {
	t.Helper()
	return &catalogTestStore{
		accounts:      make(map[shared.AccountID]domainaccounting.Account),
		categories:    make(map[shared.CategoryID]domaincatalog.Category),
		subcategories: make(map[shared.SubcategoryID]domaincatalog.Subcategory),
	}
}

func (s *catalogTestStore) mustCreateAccount(t *testing.T, userID shared.UserID, name string) shared.AccountID {
	t.Helper()

	return s.mustCreateAccountWithParams(t, accountFixtureParams{
		UserID:    userID,
		Name:      name,
		Type:      domainaccounting.AccountTypeDebitCard,
		Currency:  shared.CurrencyRUB,
		Balance:   100_00,
		Initial:   100_00,
		CreatedAt: time.Date(2026, 4, 28, 12, 0, 0, s.accountSeq+1, time.UTC),
	})
}

type accountFixtureParams struct {
	UserID               shared.UserID
	Name                 string
	Type                 domainaccounting.AccountType
	Currency             shared.Currency
	Balance              int64
	Initial              int64
	ArchivedAt           *time.Time
	IncludeInNetWorth    *bool
	IncludeInDailyBudget *bool
	CreatedAt            time.Time
}

func (s *catalogTestStore) mustCreateAccountWithParams(t *testing.T, params accountFixtureParams) shared.AccountID {
	t.Helper()

	s.accountSeq++
	id := shared.AccountID("acc-" + strconv.Itoa(s.accountSeq))
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Date(2026, 4, 28, 12, 0, 0, s.accountSeq, time.UTC)
	}
	includeInNetWorth := true
	if params.IncludeInNetWorth != nil {
		includeInNetWorth = *params.IncludeInNetWorth
	}
	includeInDailyBudget := true
	if params.IncludeInDailyBudget != nil {
		includeInDailyBudget = *params.IncludeInDailyBudget
	}

	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   id,
		UserID:               params.UserID,
		Name:                 params.Name,
		Type:                 params.Type,
		Balance:              shared.NewMoney(params.Balance, params.Currency),
		InitialBalance:       shared.NewMoney(params.Initial, params.Currency),
		IncludeInNetWorth:    includeInNetWorth,
		IncludeInDailyBudget: includeInDailyBudget,
		ArchivedAt:           params.ArchivedAt,
		CreatedAt:            createdAt,
		UpdatedAt:            createdAt,
	})
	if err != nil {
		t.Fatalf("build account fixture: %v", err)
	}

	s.accounts[id] = account
	return id
}

func (s *catalogTestStore) mustCreateCategory(t *testing.T, userID shared.UserID, name string) shared.CategoryID {
	t.Helper()

	return s.mustCreateCategoryWithParams(t, categoryFixtureParams{
		UserID: userID,
		Name:   name,
		Type:   domaincatalog.CategoryTypeFlexible,
	})
}

type categoryFixtureParams struct {
	UserID     shared.UserID
	Name       string
	Type       domaincatalog.CategoryType
	Color      *string
	SortOrder  int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (s *catalogTestStore) mustCreateCategoryWithParams(
	t *testing.T,
	params categoryFixtureParams,
) shared.CategoryID {
	t.Helper()

	s.categorySeq++
	id := shared.CategoryID("cat-" + strconv.Itoa(s.categorySeq))
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Date(2026, 4, 28, 12, 0, 0, s.categorySeq, time.UTC)
	}
	updatedAt := params.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	sortOrder := params.SortOrder
	if sortOrder == 0 {
		sortOrder = 100
	}

	category, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:         id,
		UserID:     params.UserID,
		Name:       params.Name,
		Type:       params.Type,
		Color:      params.Color,
		SortOrder:  sortOrder,
		ArchivedAt: params.ArchivedAt,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		t.Fatalf("build category fixture: %v", err)
	}

	s.categories[id] = category
	return id
}

func (s *catalogTestStore) mustCreateSubcategory(
	t *testing.T,
	userID shared.UserID,
	categoryID shared.CategoryID,
	name string,
) shared.SubcategoryID {
	t.Helper()

	return s.mustCreateSubcategoryWithParams(t, subcategoryFixtureParams{
		UserID:     userID,
		CategoryID: categoryID,
		Name:       name,
	})
}

type subcategoryFixtureParams struct {
	UserID     shared.UserID
	CategoryID shared.CategoryID
	Name       string
	SortOrder  int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (s *catalogTestStore) mustCreateSubcategoryWithParams(
	t *testing.T,
	params subcategoryFixtureParams,
) shared.SubcategoryID {
	t.Helper()

	s.subcategorySeq++
	id := shared.SubcategoryID("sub-" + strconv.Itoa(s.subcategorySeq))
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Date(2026, 4, 28, 12, 0, 0, s.subcategorySeq, time.UTC)
	}
	updatedAt := params.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	sortOrder := params.SortOrder
	if sortOrder == 0 {
		sortOrder = 100
	}

	subcategory, err := domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         id,
		UserID:     params.UserID,
		CategoryID: params.CategoryID,
		Name:       params.Name,
		SortOrder:  sortOrder,
		ArchivedAt: params.ArchivedAt,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		t.Fatalf("build subcategory fixture: %v", err)
	}

	s.subcategories[id] = subcategory
	return id
}

type accountCreateUseCase struct {
	store *catalogTestStore
}

func (u accountCreateUseCase) Create(_ context.Context, input appaccounting.CreateAccountInput) (domainaccounting.Account, error) {
	for _, existing := range u.store.accounts {
		if existing.UserID() != input.UserID {
			continue
		}
		if strings.EqualFold(existing.Name(), input.Name) && existing.ArchivedAt() == nil {
			return domainaccounting.Account{}, appaccounting.ErrAccountNameAlreadyExists
		}
	}

	u.store.accountSeq++
	id := shared.AccountID("acc-" + strconv.Itoa(u.store.accountSeq))
	now := time.Date(2026, 4, 28, 12, 0, 0, u.store.accountSeq, time.UTC)
	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   id,
		UserID:               input.UserID,
		Name:                 input.Name,
		Type:                 input.Type,
		Balance:              input.InitialBalance,
		InitialBalance:       input.InitialBalance,
		IncludeInNetWorth:    input.IncludeInNetWorth,
		IncludeInDailyBudget: input.IncludeInDailyBudget,
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	u.store.accounts[id] = account
	return account, nil
}

type accountGetUseCase struct {
	store *catalogTestStore
}

func (u accountGetUseCase) GetByID(_ context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error) {
	account, ok := u.store.accounts[accountID]
	if !ok || account.UserID() != userID {
		return domainaccounting.Account{}, appaccounting.ErrAccountNotFound
	}

	return account, nil
}

type accountListUseCase struct {
	store *catalogTestStore
}

func (u accountListUseCase) ListByUser(_ context.Context, input appaccounting.ListAccountsInput) ([]domainaccounting.Account, error) {
	accounts := make([]domainaccounting.Account, 0, len(u.store.accounts))
	for _, account := range u.store.accounts {
		if account.UserID() != input.UserID {
			continue
		}
		if !input.IncludeArchived && account.ArchivedAt() != nil {
			continue
		}
		if input.Type != nil && account.Type() != *input.Type {
			continue
		}
		if input.Currency != nil && account.Balance().Currency() != *input.Currency {
			continue
		}
		accounts = append(accounts, account)
	}

	sortMode := input.Sort
	if sortMode == "" {
		sortMode = appaccounting.AccountsSortCreatedAtDesc
	}

	sort.Slice(accounts, func(i, j int) bool {
		left := accounts[i]
		right := accounts[j]

		switch sortMode {
		case appaccounting.AccountsSortNameAsc:
			leftName := strings.ToLower(left.Name())
			rightName := strings.ToLower(right.Name())
			if leftName != rightName {
				return leftName < rightName
			}
		case appaccounting.AccountsSortBalanceDesc:
			if left.Balance().MinorUnits() != right.Balance().MinorUnits() {
				return left.Balance().MinorUnits() > right.Balance().MinorUnits()
			}
		default:
			if !left.CreatedAt().Equal(right.CreatedAt()) {
				return left.CreatedAt().After(right.CreatedAt())
			}
		}

		return string(left.ID()) < string(right.ID())
	})
	return accounts, nil
}

type accountUpdateUseCase struct {
	store *catalogTestStore
}

func (u accountUpdateUseCase) Update(_ context.Context, input appaccounting.UpdateAccountInput) (domainaccounting.Account, error) {
	account, ok := u.store.accounts[input.AccountID]
	if !ok || account.UserID() != input.UserID {
		return domainaccounting.Account{}, appaccounting.ErrAccountNotFound
	}

	name := account.Name()
	if input.Name != nil {
		name = *input.Name
	}
	accountType := account.Type()
	if input.Type != nil {
		accountType = *input.Type
	}
	includeInNetWorth := account.IncludeInNetWorth()
	if input.IncludeInNetWorth != nil {
		includeInNetWorth = *input.IncludeInNetWorth
	}
	includeInDailyBudget := account.IncludeInDailyBudget()
	if input.IncludeInDailyBudget != nil {
		includeInDailyBudget = *input.IncludeInDailyBudget
	}

	for id, existing := range u.store.accounts {
		if id == input.AccountID {
			continue
		}
		if existing.UserID() != input.UserID {
			continue
		}
		if strings.EqualFold(existing.Name(), name) && existing.ArchivedAt() == nil {
			return domainaccounting.Account{}, appaccounting.ErrAccountNameAlreadyExists
		}
	}

	updatedAt := time.Date(2026, 4, 28, 13, 0, 0, 0, time.UTC)
	updated, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   account.ID(),
		UserID:               account.UserID(),
		Name:                 name,
		Type:                 accountType,
		Balance:              account.Balance(),
		InitialBalance:       account.InitialBalance(),
		IncludeInNetWorth:    includeInNetWorth,
		IncludeInDailyBudget: includeInDailyBudget,
		ArchivedAt:           account.ArchivedAt(),
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            updatedAt,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	u.store.accounts[input.AccountID] = updated
	return updated, nil
}

type accountSummaryUseCase struct {
	store *catalogTestStore
}

func (u accountSummaryUseCase) GetByUserAndCurrency(
	_ context.Context,
	input appaccounting.GetAccountsSummaryInput,
) (appaccounting.AccountSummary, error) {
	var netWorthBase int64
	var cashBalance int64
	var availableForDailyBudget int64
	var creditLiabilities int64
	accounts := make([]appaccounting.AccountSummaryAccount, 0, len(u.store.accounts))

	for _, account := range u.store.accounts {
		if account.UserID() != input.UserID {
			continue
		}
		if account.ArchivedAt() != nil {
			continue
		}
		if account.Balance().Currency() != input.Currency {
			continue
		}

		balanceMinor := account.Balance().MinorUnits()
		if account.IncludeInNetWorth() {
			netWorthBase += balanceMinor
		}
		if account.IncludeInDailyBudget() {
			availableForDailyBudget += balanceMinor
		}
		if account.Type() == domainaccounting.AccountTypeCash ||
			account.Type() == domainaccounting.AccountTypeDebitCard ||
			account.Type() == domainaccounting.AccountTypeSavings ||
			account.Type() == domainaccounting.AccountTypeDeposit {
			cashBalance += balanceMinor
		}
		if account.Type() == domainaccounting.AccountTypeCreditCard || account.Type() == domainaccounting.AccountTypeDebt {
			creditLiabilities += balanceMinor
		}

		accounts = append(accounts, appaccounting.AccountSummaryAccount{
			ID:                   account.ID(),
			Name:                 account.Name(),
			Type:                 account.Type(),
			Balance:              account.Balance(),
			IncludeInNetWorth:    account.IncludeInNetWorth(),
			IncludeInDailyBudget: account.IncludeInDailyBudget(),
		})
	}

	return appaccounting.AccountSummary{
		Currency:                input.Currency,
		NetWorth:                shared.NewMoney(netWorthBase-creditLiabilities, input.Currency),
		CashBalance:             shared.NewMoney(cashBalance, input.Currency),
		AvailableForDailyBudget: shared.NewMoney(availableForDailyBudget, input.Currency),
		CreditLiabilities:       shared.NewMoney(creditLiabilities, input.Currency),
		Accounts:                accounts,
	}, nil
}

type accountArchiveUseCase struct {
	store *catalogTestStore
}

func (u accountArchiveUseCase) Archive(
	_ context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, ok := u.store.accounts[accountID]
	if !ok || account.UserID() != userID {
		return domainaccounting.Account{}, appaccounting.ErrAccountNotFound
	}
	if account.ArchivedAt() != nil {
		return account, nil
	}

	archivedAt := time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)
	updated, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   account.ID(),
		UserID:               account.UserID(),
		Name:                 account.Name(),
		Type:                 account.Type(),
		Balance:              account.Balance(),
		InitialBalance:       account.InitialBalance(),
		IncludeInNetWorth:    account.IncludeInNetWorth(),
		IncludeInDailyBudget: account.IncludeInDailyBudget(),
		ArchivedAt:           &archivedAt,
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            archivedAt,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	u.store.accounts[accountID] = updated
	return updated, nil
}

type accountRestoreUseCase struct {
	store *catalogTestStore
}

func (u accountRestoreUseCase) Restore(
	_ context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (domainaccounting.Account, error) {
	account, ok := u.store.accounts[accountID]
	if !ok || account.UserID() != userID {
		return domainaccounting.Account{}, appaccounting.ErrAccountNotFound
	}
	if account.ArchivedAt() == nil {
		return account, nil
	}

	restoredAt := time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC)
	updated, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   account.ID(),
		UserID:               account.UserID(),
		Name:                 account.Name(),
		Type:                 account.Type(),
		Balance:              account.Balance(),
		InitialBalance:       account.InitialBalance(),
		IncludeInNetWorth:    account.IncludeInNetWorth(),
		IncludeInDailyBudget: account.IncludeInDailyBudget(),
		ArchivedAt:           nil,
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            restoredAt,
	})
	if err != nil {
		return domainaccounting.Account{}, err
	}

	u.store.accounts[accountID] = updated
	return updated, nil
}

type categoryCreateUseCase struct {
	store *catalogTestStore
}

func (u categoryCreateUseCase) Create(
	_ context.Context,
	input appcatalog.CreateCategoryInput,
) (domaincatalog.Category, error) {
	for _, category := range u.store.categories {
		if category.UserID() != input.UserID {
			continue
		}
		if strings.EqualFold(category.Name(), input.Name) && category.ArchivedAt() == nil {
			return domaincatalog.Category{}, appcatalog.ErrCategoryNameAlreadyExists
		}
	}

	u.store.categorySeq++
	id := shared.CategoryID("cat-" + strconv.Itoa(u.store.categorySeq))
	now := time.Date(2026, 4, 28, 12, 0, 0, u.store.categorySeq, time.UTC)
	sortOrder := 100
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	category, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:        id,
		UserID:    input.UserID,
		Name:      input.Name,
		Type:      input.Type,
		Color:     input.Color,
		SortOrder: sortOrder,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return domaincatalog.Category{}, err
	}

	u.store.categories[id] = category
	return category, nil
}

type categoryGetUseCase struct {
	store *catalogTestStore
}

func (u categoryGetUseCase) GetByID(_ context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error) {
	category, ok := u.store.categories[categoryID]
	if !ok || category.UserID() != userID {
		return domaincatalog.Category{}, appcatalog.ErrCategoryNotFound
	}

	return category, nil
}

type categoryListUseCase struct {
	store *catalogTestStore
}

func (u categoryListUseCase) ListByUser(
	_ context.Context,
	input appcatalog.ListCategoriesInput,
) ([]domaincatalog.Category, error) {
	categories := make([]domaincatalog.Category, 0, len(u.store.categories))
	for _, category := range u.store.categories {
		if category.UserID() != input.UserID {
			continue
		}
		if !input.IncludeArchived && category.ArchivedAt() != nil {
			continue
		}
		if input.Type != nil && category.Type() != *input.Type {
			continue
		}
		categories = append(categories, category)
	}

	sort.Slice(categories, func(i, j int) bool {
		left := categories[i]
		right := categories[j]

		sortMode := input.Sort
		if sortMode == "" {
			sortMode = appcatalog.CategorySortSortOrderAsc
		}

		switch sortMode {
		case appcatalog.CategorySortNameAsc:
			leftName := strings.ToLower(left.Name())
			rightName := strings.ToLower(right.Name())
			if leftName != rightName {
				return leftName < rightName
			}
		case appcatalog.CategorySortCreatedAtDesc:
			if !left.CreatedAt().Equal(right.CreatedAt()) {
				return left.CreatedAt().After(right.CreatedAt())
			}
		default:
			if left.SortOrder() != right.SortOrder() {
				return left.SortOrder() < right.SortOrder()
			}
		}

		return string(left.ID()) < string(right.ID())
	})
	return categories, nil
}

type categoryUpdateUseCase struct {
	store *catalogTestStore
}

func (u categoryUpdateUseCase) Update(
	_ context.Context,
	input appcatalog.UpdateCategoryInput,
) (domaincatalog.Category, error) {
	category, ok := u.store.categories[input.CategoryID]
	if !ok || category.UserID() != input.UserID {
		return domaincatalog.Category{}, appcatalog.ErrCategoryNotFound
	}

	name := category.Name()
	if input.Name != nil {
		name = *input.Name
	}
	categoryType := category.Type()
	if input.Type != nil {
		categoryType = *input.Type
	}
	color := category.Color()
	if input.Color != nil {
		colorValue := *input.Color
		color = &colorValue
	}
	sortOrder := category.SortOrder()
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	for id, existing := range u.store.categories {
		if id == input.CategoryID {
			continue
		}
		if existing.UserID() != input.UserID {
			continue
		}
		if strings.EqualFold(existing.Name(), name) && existing.ArchivedAt() == nil {
			return domaincatalog.Category{}, appcatalog.ErrCategoryNameAlreadyExists
		}
	}

	updatedAt := time.Date(2026, 4, 28, 13, 30, 0, 0, time.UTC)
	updated, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:         category.ID(),
		UserID:     category.UserID(),
		Name:       name,
		Type:       categoryType,
		Color:      color,
		SortOrder:  sortOrder,
		ArchivedAt: category.ArchivedAt(),
		CreatedAt:  category.CreatedAt(),
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		return domaincatalog.Category{}, err
	}

	u.store.categories[input.CategoryID] = updated
	return updated, nil
}

type categoryArchiveUseCase struct {
	store *catalogTestStore
}

func (u categoryArchiveUseCase) Archive(
	_ context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) (domaincatalog.Category, error) {
	category, ok := u.store.categories[categoryID]
	if !ok || category.UserID() != userID {
		return domaincatalog.Category{}, appcatalog.ErrCategoryNotFound
	}
	if category.ArchivedAt() != nil {
		return category, nil
	}

	archivedAt := time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC)
	updated, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:         category.ID(),
		UserID:     category.UserID(),
		Name:       category.Name(),
		Type:       category.Type(),
		Color:      category.Color(),
		SortOrder:  category.SortOrder(),
		ArchivedAt: &archivedAt,
		CreatedAt:  category.CreatedAt(),
		UpdatedAt:  archivedAt,
	})
	if err != nil {
		return domaincatalog.Category{}, err
	}
	u.store.categories[categoryID] = updated

	for subcategoryID, subcategory := range u.store.subcategories {
		if subcategory.UserID() != userID {
			continue
		}
		if subcategory.CategoryID() == categoryID {
			archivedSubcategory, buildErr := buildSubcategoryWithArchiveState(subcategory, timePtr(archivedAt), archivedAt)
			if buildErr != nil {
				return domaincatalog.Category{}, buildErr
			}
			u.store.subcategories[subcategoryID] = archivedSubcategory
		}
	}

	return updated, nil
}

type categoryRestoreUseCase struct {
	store *catalogTestStore
}

func (u categoryRestoreUseCase) Restore(
	_ context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) (domaincatalog.Category, error) {
	category, ok := u.store.categories[categoryID]
	if !ok || category.UserID() != userID {
		return domaincatalog.Category{}, appcatalog.ErrCategoryNotFound
	}
	if category.ArchivedAt() == nil {
		return category, nil
	}

	restoredAt := time.Date(2026, 4, 28, 15, 30, 0, 0, time.UTC)
	updated, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:        category.ID(),
		UserID:    category.UserID(),
		Name:      category.Name(),
		Type:      category.Type(),
		Color:     category.Color(),
		SortOrder: category.SortOrder(),
		CreatedAt: category.CreatedAt(),
		UpdatedAt: restoredAt,
	})
	if err != nil {
		return domaincatalog.Category{}, err
	}
	u.store.categories[categoryID] = updated

	for subcategoryID, subcategory := range u.store.subcategories {
		if subcategory.UserID() != userID {
			continue
		}
		if subcategory.CategoryID() == categoryID {
			restoredSubcategory, buildErr := buildSubcategoryWithArchiveState(subcategory, nil, restoredAt)
			if buildErr != nil {
				return domaincatalog.Category{}, buildErr
			}
			u.store.subcategories[subcategoryID] = restoredSubcategory
		}
	}

	return updated, nil
}

type subcategoryCreateUseCase struct {
	store *catalogTestStore
}

func (u subcategoryCreateUseCase) Create(
	_ context.Context,
	input appcatalog.CreateSubcategoryInput,
) (domaincatalog.Subcategory, error) {
	category, ok := u.store.categories[input.CategoryID]
	if !ok || category.UserID() != input.UserID {
		return domaincatalog.Subcategory{}, appcatalog.ErrCategoryNotFound
	}
	if category.ArchivedAt() != nil {
		return domaincatalog.Subcategory{}, appcatalog.ErrParentCategoryArchived
	}

	for _, existing := range u.store.subcategories {
		if existing.UserID() != input.UserID {
			continue
		}
		if existing.CategoryID() != input.CategoryID {
			continue
		}
		if existing.ArchivedAt() != nil {
			continue
		}
		if strings.EqualFold(existing.Name(), input.Name) {
			return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNameAlreadyExists
		}
	}

	u.store.subcategorySeq++
	id := shared.SubcategoryID("sub-" + strconv.Itoa(u.store.subcategorySeq))
	now := time.Date(2026, 4, 28, 12, 30, 0, u.store.subcategorySeq, time.UTC)
	sortOrder := 100
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	subcategory, err := domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         id,
		UserID:     input.UserID,
		CategoryID: input.CategoryID,
		Name:       input.Name,
		SortOrder:  sortOrder,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		return domaincatalog.Subcategory{}, err
	}

	u.store.subcategories[id] = subcategory
	return subcategory, nil
}

type subcategoryListByCategoryUseCase struct {
	store *catalogTestStore
}

func (u subcategoryListByCategoryUseCase) List(
	_ context.Context,
	input appcatalog.ListSubcategoriesByCategoryInput,
) ([]domaincatalog.Subcategory, error) {
	parentCategory, ok := u.store.categories[input.CategoryID]
	if !ok || parentCategory.UserID() != input.UserID {
		return nil, appcatalog.ErrCategoryNotFound
	}

	subcategories := make([]domaincatalog.Subcategory, 0, len(u.store.subcategories))
	for _, subcategory := range u.store.subcategories {
		if subcategory.UserID() != input.UserID {
			continue
		}
		if subcategory.CategoryID() != input.CategoryID {
			continue
		}
		if !input.IncludeArchived && subcategory.ArchivedAt() != nil {
			continue
		}
		subcategories = append(subcategories, subcategory)
	}

	sort.Slice(subcategories, func(i, j int) bool {
		left := subcategories[i]
		right := subcategories[j]
		if left.SortOrder() != right.SortOrder() {
			return left.SortOrder() < right.SortOrder()
		}
		if !left.CreatedAt().Equal(right.CreatedAt()) {
			return left.CreatedAt().After(right.CreatedAt())
		}
		return string(left.ID()) < string(right.ID())
	})

	return subcategories, nil
}

type subcategoryUpdateUseCase struct {
	store *catalogTestStore
}

func (u subcategoryUpdateUseCase) Update(
	_ context.Context,
	input appcatalog.UpdateSubcategoryInput,
) (domaincatalog.Subcategory, error) {
	subcategory, ok := u.store.subcategories[input.SubcategoryID]
	if !ok || subcategory.UserID() != input.UserID {
		return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
	}

	name := subcategory.Name()
	if input.Name != nil {
		name = *input.Name
	}
	sortOrder := subcategory.SortOrder()
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	for id, existing := range u.store.subcategories {
		if id == input.SubcategoryID {
			continue
		}
		if existing.UserID() != input.UserID {
			continue
		}
		if existing.CategoryID() != subcategory.CategoryID() {
			continue
		}
		if existing.ArchivedAt() != nil {
			continue
		}
		if strings.EqualFold(existing.Name(), name) {
			return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNameAlreadyExists
		}
	}

	updatedAt := time.Date(2026, 4, 28, 16, 0, 0, 0, time.UTC)
	updated, err := domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         subcategory.ID(),
		UserID:     subcategory.UserID(),
		CategoryID: subcategory.CategoryID(),
		Name:       name,
		SortOrder:  sortOrder,
		ArchivedAt: subcategory.ArchivedAt(),
		CreatedAt:  subcategory.CreatedAt(),
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		return domaincatalog.Subcategory{}, err
	}

	u.store.subcategories[input.SubcategoryID] = updated
	return updated, nil
}

type subcategoryArchiveUseCase struct {
	store *catalogTestStore
}

func (u subcategoryArchiveUseCase) Archive(
	_ context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	subcategory, ok := u.store.subcategories[subcategoryID]
	if !ok || subcategory.UserID() != userID {
		return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
	}
	if subcategory.ArchivedAt() != nil {
		return subcategory, nil
	}

	archivedAt := time.Date(2026, 4, 28, 16, 15, 0, 0, time.UTC)
	archived, err := buildSubcategoryWithArchiveState(subcategory, timePtr(archivedAt), archivedAt)
	if err != nil {
		return domaincatalog.Subcategory{}, err
	}

	u.store.subcategories[subcategoryID] = archived
	return archived, nil
}

type subcategoryRestoreUseCase struct {
	store *catalogTestStore
}

func (u subcategoryRestoreUseCase) Restore(
	_ context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	subcategory, ok := u.store.subcategories[subcategoryID]
	if !ok || subcategory.UserID() != userID {
		return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
	}
	if subcategory.ArchivedAt() == nil {
		return subcategory, nil
	}

	parentCategory, ok := u.store.categories[subcategory.CategoryID()]
	if !ok || parentCategory.UserID() != userID {
		return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
	}
	if parentCategory.ArchivedAt() != nil {
		return domaincatalog.Subcategory{}, appcatalog.ErrParentCategoryArchived
	}

	restoredAt := time.Date(2026, 4, 28, 16, 30, 0, 0, time.UTC)
	restored, err := buildSubcategoryWithArchiveState(subcategory, nil, restoredAt)
	if err != nil {
		return domaincatalog.Subcategory{}, err
	}

	u.store.subcategories[subcategoryID] = restored
	return restored, nil
}

type subcategoryGetUseCase struct {
	store *catalogTestStore
}

func (u subcategoryGetUseCase) GetByID(
	_ context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	subcategory, ok := u.store.subcategories[subcategoryID]
	if !ok || subcategory.UserID() != userID || subcategory.ArchivedAt() != nil {
		return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
	}

	return subcategory, nil
}

type subcategoryListUseCase struct {
	store *catalogTestStore
}

func (u subcategoryListUseCase) ListByUserID(_ context.Context, userID shared.UserID) ([]domaincatalog.Subcategory, error) {
	subcategories := make([]domaincatalog.Subcategory, 0, len(u.store.subcategories))
	for _, subcategory := range u.store.subcategories {
		if subcategory.UserID() != userID {
			continue
		}
		if subcategory.ArchivedAt() != nil {
			continue
		}
		subcategories = append(subcategories, subcategory)
	}

	sort.Slice(subcategories, func(i, j int) bool {
		return string(subcategories[i].ID()) < string(subcategories[j].ID())
	})
	return subcategories, nil
}

func buildSubcategoryWithArchiveState(
	source domaincatalog.Subcategory,
	archivedAt *time.Time,
	updatedAt time.Time,
) (domaincatalog.Subcategory, error) {
	return domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         source.ID(),
		UserID:     source.UserID(),
		CategoryID: source.CategoryID(),
		Name:       source.Name(),
		SortOrder:  source.SortOrder(),
		ArchivedAt: archivedAt,
		CreatedAt:  source.CreatedAt(),
		UpdatedAt:  updatedAt,
	})
}

func timePtr(value time.Time) *time.Time {
	valueCopy := value
	return &valueCopy
}

func boolPtr(value bool) *bool {
	return &value
}
