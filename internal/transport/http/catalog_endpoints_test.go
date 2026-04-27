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
	UserID    shared.UserID
	Name      string
	Type      domainaccounting.AccountType
	Currency  shared.Currency
	Balance   int64
	Initial   int64
	CreatedAt time.Time
}

func (s *catalogTestStore) mustCreateAccountWithParams(t *testing.T, params accountFixtureParams) shared.AccountID {
	t.Helper()

	s.accountSeq++
	id := shared.AccountID("acc-" + strconv.Itoa(s.accountSeq))
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Date(2026, 4, 28, 12, 0, 0, s.accountSeq, time.UTC)
	}

	account, err := domainaccounting.NewAccount(domainaccounting.NewAccountParams{
		ID:                   id,
		UserID:               params.UserID,
		Name:                 params.Name,
		Type:                 params.Type,
		Balance:              shared.NewMoney(params.Balance, params.Currency),
		InitialBalance:       shared.NewMoney(params.Initial, params.Currency),
		IncludeInNetWorth:    true,
		IncludeInDailyBudget: true,
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
