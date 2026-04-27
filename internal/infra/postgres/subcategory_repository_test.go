package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	appcatalog "moneo/internal/app/catalog"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"

	"github.com/google/uuid"
)

func TestSubcategoryRepositoryCreateFindAndListByCategoryScopedByUserID(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	repo := NewSubcategoryRepository(pool)
	ctx := context.Background()

	userA := insertAccountTestUser(t, pool, "subcat-user-a@example.com")
	userB := insertAccountTestUser(t, pool, "subcat-user-b@example.com")

	categoryA := newCategoryFixture(t, categoryFixtureParams{
		UserID: userA,
		Name:   "Food",
		Type:   domaincatalog.CategoryTypeRequired,
	})
	categoryB := newCategoryFixture(t, categoryFixtureParams{
		UserID: userB,
		Name:   "Salary",
		Type:   domaincatalog.CategoryTypeIncome,
	})
	if err := categoryRepo.Create(ctx, categoryA); err != nil {
		t.Fatalf("create category A: %v", err)
	}
	if err := categoryRepo.Create(ctx, categoryB); err != nil {
		t.Fatalf("create category B: %v", err)
	}

	first := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userA,
		CategoryID: categoryA.ID(),
		Name:       "Groceries",
		SortOrder:  10,
	})
	second := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userB,
		CategoryID: categoryB.ID(),
		Name:       "Bonus",
		SortOrder:  20,
	})
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first subcategory: %v", err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second subcategory: %v", err)
	}

	found, err := repo.FindSubcategoryByID(ctx, userA, first.ID())
	if err != nil {
		t.Fatalf("find subcategory by owner: %v", err)
	}
	if found.UserID() != userA {
		t.Fatalf("expected user %q, got %q", userA, found.UserID())
	}
	if found.CategoryID() != categoryA.ID() {
		t.Fatalf("expected category %q, got %q", categoryA.ID(), found.CategoryID())
	}

	_, err = repo.FindSubcategoryByID(ctx, userA, second.ID())
	if !errors.Is(err, appcatalog.ErrSubcategoryNotFound) {
		t.Fatalf("expected ErrSubcategoryNotFound for foreign subcategory, got %v", err)
	}

	list, err := repo.ListByCategoryID(ctx, userA, categoryA.ID(), false)
	if err != nil {
		t.Fatalf("list subcategories by category: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 subcategory, got %d", len(list))
	}
	if list[0].ID() != first.ID() {
		t.Fatalf("expected subcategory %q, got %q", first.ID(), list[0].ID())
	}
}

func TestSubcategoryRepositoryCreateRejectsDuplicateActiveNamePerCategory(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	repo := NewSubcategoryRepository(pool)
	ctx := context.Background()

	userID := insertAccountTestUser(t, pool, "subcat-duplicate@example.com")
	categoryA := newCategoryFixture(t, categoryFixtureParams{
		UserID: userID,
		Name:   "Food",
		Type:   domaincatalog.CategoryTypeRequired,
	})
	categoryB := newCategoryFixture(t, categoryFixtureParams{
		UserID: userID,
		Name:   "Transport",
		Type:   domaincatalog.CategoryTypeFlexible,
	})
	if err := categoryRepo.Create(ctx, categoryA); err != nil {
		t.Fatalf("create category A: %v", err)
	}
	if err := categoryRepo.Create(ctx, categoryB); err != nil {
		t.Fatalf("create category B: %v", err)
	}

	first := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userID,
		CategoryID: categoryA.ID(),
		Name:       "Coffee",
	})
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first subcategory: %v", err)
	}

	duplicate := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userID,
		CategoryID: categoryA.ID(),
		Name:       "coffee",
	})
	err := repo.Create(ctx, duplicate)
	if !errors.Is(err, appcatalog.ErrDuplicateActiveSubcategoryName) {
		t.Fatalf("expected ErrDuplicateActiveSubcategoryName, got %v", err)
	}

	allowedInAnotherCategory := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userID,
		CategoryID: categoryB.ID(),
		Name:       "coffee",
	})
	if err := repo.Create(ctx, allowedInAnotherCategory); err != nil {
		t.Fatalf("create subcategory in another category: %v", err)
	}
}

func TestSubcategoryRepositoryListByCategorySupportsArchiveFiltering(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	repo := NewSubcategoryRepository(pool)
	ctx := context.Background()

	userID := insertAccountTestUser(t, pool, "subcat-archive@example.com")
	category := newCategoryFixture(t, categoryFixtureParams{
		UserID: userID,
		Name:   "Food",
		Type:   domaincatalog.CategoryTypeRequired,
	})
	if err := categoryRepo.Create(ctx, category); err != nil {
		t.Fatalf("create category: %v", err)
	}

	now := time.Now().UTC()
	archivedAt := now.Add(-time.Hour)

	active := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userID,
		CategoryID: category.ID(),
		Name:       "Groceries",
		SortOrder:  10,
		CreatedAt:  now.Add(-2 * time.Hour),
	})
	archived := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     userID,
		CategoryID: category.ID(),
		Name:       "Restaurant",
		SortOrder:  20,
		ArchivedAt: &archivedAt,
		CreatedAt:  now.Add(-90 * time.Minute),
		UpdatedAt:  archivedAt,
	})
	if err := repo.Create(ctx, active); err != nil {
		t.Fatalf("create active subcategory: %v", err)
	}
	if err := repo.Create(ctx, archived); err != nil {
		t.Fatalf("create archived subcategory: %v", err)
	}

	activeOnly, err := repo.ListByCategoryID(ctx, userID, category.ID(), false)
	if err != nil {
		t.Fatalf("list active subcategories by category: %v", err)
	}
	if len(activeOnly) != 1 {
		t.Fatalf("expected 1 active subcategory, got %d", len(activeOnly))
	}
	if activeOnly[0].Name() != "Groceries" {
		t.Fatalf("expected Groceries, got %q", activeOnly[0].Name())
	}

	withArchived, err := repo.ListByCategoryID(ctx, userID, category.ID(), true)
	if err != nil {
		t.Fatalf("list subcategories by category with archived: %v", err)
	}
	if len(withArchived) != 2 {
		t.Fatalf("expected 2 subcategories with archived, got %d", len(withArchived))
	}

	userList, err := repo.ListSubcategoriesByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("list subcategories by user id: %v", err)
	}
	if len(userList) != 1 {
		t.Fatalf("expected 1 active subcategory in user list, got %d", len(userList))
	}
}

func TestSubcategoryRepositoryChecksParentCategoryOwnershipForCreateAndList(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	repo := NewSubcategoryRepository(pool)
	ctx := context.Background()

	ownerUserID := insertAccountTestUser(t, pool, "subcat-owner@example.com")
	foreignUserID := insertAccountTestUser(t, pool, "subcat-foreign@example.com")

	ownerCategory := newCategoryFixture(t, categoryFixtureParams{
		UserID: ownerUserID,
		Name:   "Owner category",
		Type:   domaincatalog.CategoryTypeRequired,
	})
	if err := categoryRepo.Create(ctx, ownerCategory); err != nil {
		t.Fatalf("create owner category: %v", err)
	}

	foreignCreate := newSubcategoryFixture(t, subcategoryFixtureParams{
		UserID:     foreignUserID,
		CategoryID: ownerCategory.ID(),
		Name:       "Should fail",
	})
	err := repo.Create(ctx, foreignCreate)
	if !errors.Is(err, appcatalog.ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound for foreign create, got %v", err)
	}

	_, err = repo.ListByCategoryID(ctx, foreignUserID, ownerCategory.ID(), false)
	if !errors.Is(err, appcatalog.ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound for foreign list, got %v", err)
	}

	_, err = repo.ListByCategoryID(ctx, ownerUserID, shared.CategoryID(uuid.NewString()), false)
	if !errors.Is(err, appcatalog.ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound for missing category list, got %v", err)
	}
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

func newSubcategoryFixture(t *testing.T, params subcategoryFixtureParams) domaincatalog.Subcategory {
	t.Helper()

	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
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
		ID:         shared.SubcategoryID(uuid.NewString()),
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

	return subcategory
}
