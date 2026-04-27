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

func TestCategoryRepositoryCreateFindAndListScopedByUserID(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewCategoryRepository(pool)
	ctx := context.Background()

	userA := insertAccountTestUser(t, pool, "cat-user-a@example.com")
	userB := insertAccountTestUser(t, pool, "cat-user-b@example.com")

	first := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userA,
		Name:      "Food",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 10,
	})
	second := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userB,
		Name:      "Salary",
		Type:      domaincatalog.CategoryTypeIncome,
		SortOrder: 20,
	})

	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first category: %v", err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second category: %v", err)
	}

	found, err := repo.FindCategoryByID(ctx, userA, first.ID())
	if err != nil {
		t.Fatalf("find category by owner: %v", err)
	}
	if found.UserID() != userA {
		t.Fatalf("expected user %q, got %q", userA, found.UserID())
	}
	if found.Name() != "Food" {
		t.Fatalf("expected category name Food, got %q", found.Name())
	}
	if found.Type() != domaincatalog.CategoryTypeRequired {
		t.Fatalf("expected category type required, got %q", found.Type())
	}

	_, err = repo.FindCategoryByID(ctx, userA, second.ID())
	if !errors.Is(err, appcatalog.ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound for foreign category, got %v", err)
	}

	list, err := repo.ListCategoriesByUserID(ctx, userA)
	if err != nil {
		t.Fatalf("list categories by user id: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 category for userA, got %d", len(list))
	}
	if list[0].ID() != first.ID() {
		t.Fatalf("expected category %q, got %q", first.ID(), list[0].ID())
	}
}

func TestCategoryRepositoryCreateRejectsDuplicateActiveNameForSameUser(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewCategoryRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "cat-duplicate@example.com")

	first := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Food",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 10,
	})
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first category: %v", err)
	}

	second := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "food",
		Type:      domaincatalog.CategoryTypeFlexible,
		SortOrder: 20,
	})
	err := repo.Create(ctx, second)
	if !errors.Is(err, appcatalog.ErrDuplicateActiveCategoryName) {
		t.Fatalf("expected ErrDuplicateActiveCategoryName, got %v", err)
	}
}

func TestCategoryRepositoryListByUserIDSupportsTypeAndArchiveFiltering(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	repo := NewCategoryRepository(pool)
	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "cat-filter@example.com")

	now := time.Now().UTC()
	archivedAt := now.Add(-time.Hour)
	required := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Required",
		Type:      domaincatalog.CategoryTypeRequired,
		SortOrder: 10,
		CreatedAt: now.Add(-2 * time.Hour),
	})
	flexible := newCategoryFixture(t, categoryFixtureParams{
		UserID:    userID,
		Name:      "Flexible",
		Type:      domaincatalog.CategoryTypeFlexible,
		SortOrder: 20,
		CreatedAt: now.Add(-90 * time.Minute),
	})
	incomeArchived := newCategoryFixture(t, categoryFixtureParams{
		UserID:     userID,
		Name:       "Income archived",
		Type:       domaincatalog.CategoryTypeIncome,
		SortOrder:  30,
		ArchivedAt: &archivedAt,
		CreatedAt:  now.Add(-30 * time.Minute),
		UpdatedAt:  archivedAt,
	})

	if err := repo.Create(ctx, required); err != nil {
		t.Fatalf("create required category: %v", err)
	}
	if err := repo.Create(ctx, flexible); err != nil {
		t.Fatalf("create flexible category: %v", err)
	}
	if err := repo.Create(ctx, incomeArchived); err != nil {
		t.Fatalf("create archived income category: %v", err)
	}

	activeOnly, err := repo.ListByUserID(ctx, CategoryListInput{
		UserID:          userID,
		IncludeArchived: false,
	})
	if err != nil {
		t.Fatalf("list active categories: %v", err)
	}
	if len(activeOnly) != 2 {
		t.Fatalf("expected 2 active categories, got %d", len(activeOnly))
	}

	withArchived, err := repo.ListByUserID(ctx, CategoryListInput{
		UserID:          userID,
		IncludeArchived: true,
	})
	if err != nil {
		t.Fatalf("list categories with archived: %v", err)
	}
	if len(withArchived) != 3 {
		t.Fatalf("expected 3 categories with archived, got %d", len(withArchived))
	}

	requiredType := domaincatalog.CategoryTypeRequired
	requiredOnly, err := repo.ListByUserID(ctx, CategoryListInput{
		UserID:          userID,
		IncludeArchived: true,
		Type:            &requiredType,
	})
	if err != nil {
		t.Fatalf("list required categories: %v", err)
	}
	if len(requiredOnly) != 1 {
		t.Fatalf("expected 1 required category, got %d", len(requiredOnly))
	}
	if requiredOnly[0].Type() != domaincatalog.CategoryTypeRequired {
		t.Fatalf("expected required category type, got %q", requiredOnly[0].Type())
	}

	incomeType := domaincatalog.CategoryTypeIncome
	incomeActiveOnly, err := repo.ListByUserID(ctx, CategoryListInput{
		UserID:          userID,
		IncludeArchived: false,
		Type:            &incomeType,
	})
	if err != nil {
		t.Fatalf("list active income categories: %v", err)
	}
	if len(incomeActiveOnly) != 0 {
		t.Fatalf("expected 0 active income categories, got %d", len(incomeActiveOnly))
	}

	incomeWithArchived, err := repo.ListByUserID(ctx, CategoryListInput{
		UserID:          userID,
		IncludeArchived: true,
		Type:            &incomeType,
	})
	if err != nil {
		t.Fatalf("list income categories with archived: %v", err)
	}
	if len(incomeWithArchived) != 1 {
		t.Fatalf("expected 1 income category with archived, got %d", len(incomeWithArchived))
	}
	if incomeWithArchived[0].Name() != "Income archived" {
		t.Fatalf("expected archived income category, got %q", incomeWithArchived[0].Name())
	}
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

func newCategoryFixture(t *testing.T, params categoryFixtureParams) domaincatalog.Category {
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

	category, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:         shared.CategoryID(uuid.NewString()),
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

	return category
}
