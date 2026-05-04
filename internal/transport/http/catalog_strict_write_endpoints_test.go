package http_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestCatalogStrictWriteAccountFlow(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-strict-write-account@example.com")
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	createRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":                 "Main card",
		"type":                 "debit_card",
		"currency":             "RUB",
		"initialBalance":       "100.00",
		"includeInNetWorth":    true,
		"includeInDailyBudget": true,
	}, headers)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create account status 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, createRec, &created)
	if created.ID == "" {
		t.Fatal("expected account id in create response")
	}
	if created.Name != "Main card" {
		t.Fatalf("expected created account name Main card, got %q", created.Name)
	}
	if created.IsArchived {
		t.Fatal("expected created account to be active")
	}
	if created.ArchivedAt != nil {
		t.Fatalf("expected created archivedAt nil, got %v", created.ArchivedAt)
	}

	patchRec := performJSONRequest(t, router, http.MethodPatch, "/api/v1/accounts/"+created.ID, map[string]any{
		"name": "Wallet",
	}, headers)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch account status 200, got %d, body=%s", patchRec.Code, patchRec.Body.String())
	}
	var patched struct {
		Name string `json:"name"`
	}
	decodeJSONResponse(t, patchRec, &patched)
	if patched.Name != "Wallet" {
		t.Fatalf("expected patched account name Wallet, got %q", patched.Name)
	}

	archiveRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts/"+created.ID+"/archive", nil, headers)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("expected archive account status 200, got %d, body=%s", archiveRec.Code, archiveRec.Body.String())
	}
	var archived struct {
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, archiveRec, &archived)
	if !archived.IsArchived {
		t.Fatal("expected account archived after archive call")
	}
	if archived.ArchivedAt == nil || strings.TrimSpace(*archived.ArchivedAt) == "" {
		t.Fatal("expected archivedAt in archive response")
	}

	restoreRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/accounts/"+created.ID+"/restore", nil, headers)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore account status 200, got %d, body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	var restored struct {
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, restoreRec, &restored)
	if restored.IsArchived {
		t.Fatal("expected account active after restore")
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("expected restored archivedAt nil, got %v", restored.ArchivedAt)
	}
}

func TestCatalogStrictWriteCategoryAndSubcategoryFlow(t *testing.T) {
	store := newCatalogTestStore(t)
	fixture := newCatalogStrictRouterWithAuthFixture(t, store)
	router := fixture.router

	accessToken := registerAndGetAccessToken(t, router, "catalog-strict-write-category@example.com")
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	createCategoryRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/categories", map[string]any{
		"name":      "Food",
		"type":      "required",
		"sortOrder": 100,
	}, headers)
	if createCategoryRec.Code != http.StatusCreated {
		t.Fatalf("expected create category status 201, got %d, body=%s", createCategoryRec.Code, createCategoryRec.Body.String())
	}
	var createdCategory struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, createCategoryRec, &createdCategory)
	if createdCategory.ID == "" {
		t.Fatal("expected category id in create response")
	}
	if createdCategory.Name != "Food" {
		t.Fatalf("expected created category name Food, got %q", createdCategory.Name)
	}

	patchCategoryRec := performJSONRequest(t, router, http.MethodPatch, "/api/v1/categories/"+createdCategory.ID, map[string]any{
		"name": "Home food",
	}, headers)
	if patchCategoryRec.Code != http.StatusOK {
		t.Fatalf("expected patch category status 200, got %d, body=%s", patchCategoryRec.Code, patchCategoryRec.Body.String())
	}
	var patchedCategory struct {
		Name string `json:"name"`
	}
	decodeJSONResponse(t, patchCategoryRec, &patchedCategory)
	if patchedCategory.Name != "Home food" {
		t.Fatalf("expected patched category name Home food, got %q", patchedCategory.Name)
	}

	createSubcategoryRec := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/categories/"+createdCategory.ID+"/subcategories",
		map[string]any{
			"name":      "Groceries",
			"sortOrder": 120,
		},
		headers,
	)
	if createSubcategoryRec.Code != http.StatusCreated {
		t.Fatalf("expected create subcategory status 201, got %d, body=%s", createSubcategoryRec.Code, createSubcategoryRec.Body.String())
	}
	var createdSubcategory struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		IsArchived bool    `json:"isArchived"`
		ArchivedAt *string `json:"archivedAt"`
	}
	decodeJSONResponse(t, createSubcategoryRec, &createdSubcategory)
	if createdSubcategory.ID == "" {
		t.Fatal("expected subcategory id in create response")
	}

	patchSubcategoryRec := performJSONRequest(t, router, http.MethodPatch, "/api/v1/subcategories/"+createdSubcategory.ID, map[string]any{
		"name":      "Supermarkets",
		"sortOrder": 130,
	}, headers)
	if patchSubcategoryRec.Code != http.StatusOK {
		t.Fatalf("expected patch subcategory status 200, got %d, body=%s", patchSubcategoryRec.Code, patchSubcategoryRec.Body.String())
	}
	var patchedSubcategory struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sortOrder"`
	}
	decodeJSONResponse(t, patchSubcategoryRec, &patchedSubcategory)
	if patchedSubcategory.Name != "Supermarkets" {
		t.Fatalf("expected patched subcategory name Supermarkets, got %q", patchedSubcategory.Name)
	}
	if patchedSubcategory.SortOrder != 130 {
		t.Fatalf("expected patched subcategory sortOrder 130, got %d", patchedSubcategory.SortOrder)
	}

	archiveCategoryRec := performJSONRequest(t, router, http.MethodDelete, "/api/v1/categories/"+createdCategory.ID, nil, headers)
	if archiveCategoryRec.Code != http.StatusOK {
		t.Fatalf("expected archive category status 200, got %d, body=%s", archiveCategoryRec.Code, archiveCategoryRec.Body.String())
	}
	var archivedCategory struct {
		IsArchived bool `json:"isArchived"`
	}
	decodeJSONResponse(t, archiveCategoryRec, &archivedCategory)
	if !archivedCategory.IsArchived {
		t.Fatal("expected category archived after delete call")
	}

	restoreCategoryRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/categories/"+createdCategory.ID+"/restore", nil, headers)
	if restoreCategoryRec.Code != http.StatusOK {
		t.Fatalf("expected restore category status 200, got %d, body=%s", restoreCategoryRec.Code, restoreCategoryRec.Body.String())
	}
	var restoredCategory struct {
		IsArchived bool `json:"isArchived"`
	}
	decodeJSONResponse(t, restoreCategoryRec, &restoredCategory)
	if restoredCategory.IsArchived {
		t.Fatal("expected category active after restore")
	}

	archiveSubcategoryRec := performJSONRequest(t, router, http.MethodDelete, "/api/v1/subcategories/"+createdSubcategory.ID, nil, headers)
	if archiveSubcategoryRec.Code != http.StatusOK {
		t.Fatalf("expected archive subcategory status 200, got %d, body=%s", archiveSubcategoryRec.Code, archiveSubcategoryRec.Body.String())
	}
	var archivedSubcategory struct {
		IsArchived bool `json:"isArchived"`
	}
	decodeJSONResponse(t, archiveSubcategoryRec, &archivedSubcategory)
	if !archivedSubcategory.IsArchived {
		t.Fatal("expected subcategory archived after delete call")
	}

	restoreSubcategoryRec := performJSONRequest(t, router, http.MethodPost, "/api/v1/subcategories/"+createdSubcategory.ID+"/restore", nil, headers)
	if restoreSubcategoryRec.Code != http.StatusOK {
		t.Fatalf("expected restore subcategory status 200, got %d, body=%s", restoreSubcategoryRec.Code, restoreSubcategoryRec.Body.String())
	}
	var restoredSubcategory struct {
		IsArchived bool `json:"isArchived"`
	}
	decodeJSONResponse(t, restoreSubcategoryRec, &restoredSubcategory)
	if restoredSubcategory.IsArchived {
		t.Fatal("expected subcategory active after restore")
	}
}
