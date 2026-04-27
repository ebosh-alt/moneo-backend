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

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", payload.Error.Code)
	}
	assertErrorDetailField(t, payload.Error.Details, "initialBalance")
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

	var payload structuredErrorResponse
	decodeJSONResponse(t, rec, &payload)
	if payload.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error code, got %q", payload.Error.Code)
	}
	assertErrorDetailField(t, payload.Error.Details, "currency")
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

	catalogHandler := transporthttp.NewCatalogHandler(
		accountCreateUseCase{store: store},
		accountGetUseCase{store: store},
		accountListUseCase{store: store},
		accountSummaryUseCase{store: store},
		accountArchiveUseCase{store: store},
		accountRestoreUseCase{store: store},
		accountUpdateUseCase{store: store},
		categoryGetUseCase{store: store},
		categoryListUseCase{store: store},
		subcategoryGetUseCase{store: store},
		subcategoryListUseCase{store: store},
	)

	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		CatalogHandler: catalogHandler,
	})

	return catalogRouterFixture{
		router:  fixture.router,
		auth:    fixture,
		service: store,
	}
}

func registerAndGetAccessToken(t *testing.T, handler http.Handler, email string) string {
	t.Helper()

	rec := performJSONRequest(t, handler, http.MethodPost, "/auth/register", map[string]any{
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

	s.categorySeq++
	id := shared.CategoryID("cat-" + strconv.Itoa(s.categorySeq))
	now := time.Date(2026, 4, 28, 12, 0, 0, s.categorySeq, time.UTC)
	category, err := domaincatalog.NewCategory(id, userID, name, now, now)
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

	s.subcategorySeq++
	id := shared.SubcategoryID("sub-" + strconv.Itoa(s.subcategorySeq))
	now := time.Date(2026, 4, 28, 12, 0, 0, s.subcategorySeq, time.UTC)
	subcategory, err := domaincatalog.NewSubcategory(id, userID, categoryID, name, now, now)
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

func (u categoryListUseCase) ListByUserID(_ context.Context, userID shared.UserID) ([]domaincatalog.Category, error) {
	categories := make([]domaincatalog.Category, 0, len(u.store.categories))
	for _, category := range u.store.categories {
		if category.UserID() != userID {
			continue
		}
		categories = append(categories, category)
	}

	sort.Slice(categories, func(i, j int) bool {
		return string(categories[i].ID()) < string(categories[j].ID())
	})
	return categories, nil
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
	if !ok || subcategory.UserID() != userID {
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
		subcategories = append(subcategories, subcategory)
	}

	sort.Slice(subcategories, func(i, j int) bool {
		return string(subcategories[i].ID()) < string(subcategories[j].ID())
	})
	return subcategories, nil
}

func boolPtr(value bool) *bool {
	return &value
}
