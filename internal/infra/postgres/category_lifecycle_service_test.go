package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	appcatalog "moneo/internal/app/catalog"
	"moneo/internal/domain/shared"
	"moneo/internal/infra/idgen"
)

type fixedCategoryClock struct {
	now time.Time
}

func (c fixedCategoryClock) Now() time.Time {
	return c.now
}

type failingCategorySubcategoryRepository struct {
	archiveErr error
	restoreErr error
}

func (r failingCategorySubcategoryRepository) ArchiveByCategoryID(
	_ context.Context,
	_ shared.UserID,
	_ shared.CategoryID,
	_ time.Time,
) error {
	return r.archiveErr
}

func (r failingCategorySubcategoryRepository) RestoreByCategoryID(
	_ context.Context,
	_ shared.UserID,
	_ shared.CategoryID,
	_ time.Time,
	_ time.Time,
) error {
	return r.restoreErr
}

func TestArchiveCategoryServiceRollbackWhenSubcategoryCascadeFails(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	txManager := NewTxManager(pool)

	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "cat-archive-rollback@example.com")
	category := newCategoryFixture(t, categoryFixtureParams{
		UserID: userID,
		Name:   "Food",
		Type:   "required",
	})
	if err := categoryRepo.Create(ctx, category); err != nil {
		t.Fatalf("create category: %v", err)
	}

	service := appcatalog.NewArchiveCategoryService(
		categoryRepo,
		failingCategorySubcategoryRepository{archiveErr: errors.New("cascade failure")},
		txManager,
		fixedCategoryClock{now: time.Date(2026, 4, 28, 18, 0, 0, 0, time.UTC)},
	)

	_, err := service.Archive(ctx, userID, category.ID())
	if err == nil {
		t.Fatal("expected archive error")
	}

	found, findErr := categoryRepo.FindCategoryByID(ctx, userID, category.ID())
	if findErr != nil {
		t.Fatalf("find category after failed archive: %v", findErr)
	}
	if found.ArchivedAt() != nil {
		t.Fatalf("expected category to remain active after rollback, got archivedAt=%v", found.ArchivedAt())
	}
}

func TestRestoreCategoryServiceRollbackWhenSubcategoryCascadeFails(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	txManager := NewTxManager(pool)

	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "cat-restore-rollback@example.com")
	archivedAt := time.Date(2026, 4, 28, 17, 0, 0, 0, time.UTC)
	category := newCategoryFixture(t, categoryFixtureParams{
		UserID:     userID,
		Name:       "Food",
		Type:       "required",
		ArchivedAt: &archivedAt,
		CreatedAt:  archivedAt.Add(-2 * time.Hour),
		UpdatedAt:  archivedAt,
	})
	if err := categoryRepo.Create(ctx, category); err != nil {
		t.Fatalf("create archived category: %v", err)
	}

	service := appcatalog.NewRestoreCategoryService(
		categoryRepo,
		failingCategorySubcategoryRepository{restoreErr: errors.New("cascade failure")},
		txManager,
		fixedCategoryClock{now: time.Date(2026, 4, 28, 18, 30, 0, 0, time.UTC)},
	)

	_, err := service.Restore(ctx, userID, category.ID())
	if err == nil {
		t.Fatal("expected restore error")
	}

	found, findErr := categoryRepo.FindCategoryByID(ctx, userID, category.ID())
	if findErr != nil {
		t.Fatalf("find category after failed restore: %v", findErr)
	}
	if found.ArchivedAt() == nil {
		t.Fatal("expected category to remain archived after rollback")
	}
}

func TestArchiveCategoryAndCreateSubcategoryConcurrencyKeepsInvariant(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	subcategoryRepo := NewSubcategoryRepository(pool)
	txManager := NewTxManager(pool)
	ids := idgen.NewUUIDGenerator()

	archiveService := appcatalog.NewArchiveCategoryService(
		categoryRepo,
		subcategoryRepo,
		txManager,
		fixedCategoryClock{now: time.Date(2026, 4, 28, 19, 0, 0, 0, time.UTC)},
	)
	createService := appcatalog.NewCreateSubcategoryService(
		subcategoryRepo,
		categoryRepo,
		ids,
		fixedCategoryClock{now: time.Date(2026, 4, 28, 19, 0, 1, 0, time.UTC)},
	)

	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "cat-create-race@example.com")

	for i := 0; i < 20; i++ {
		category := newCategoryFixture(t, categoryFixtureParams{
			UserID: userID,
			Name:   fmt.Sprintf("Food-%d", i),
			Type:   "required",
		})
		if err := categoryRepo.Create(ctx, category); err != nil {
			t.Fatalf("create category %d: %v", i, err)
		}

		start := make(chan struct{})
		errCh := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)

		go func(categoryID shared.CategoryID) {
			defer wg.Done()
			<-start
			_, err := archiveService.Archive(ctx, userID, categoryID)
			errCh <- err
		}(category.ID())

		go func(categoryID shared.CategoryID, idx int) {
			defer wg.Done()
			<-start
			_, err := createService.Create(ctx, appcatalog.CreateSubcategoryInput{
				UserID:     userID,
				CategoryID: categoryID,
				Name:       fmt.Sprintf("Groceries-%d", idx),
			})
			if err != nil &&
				!errors.Is(err, appcatalog.ErrParentCategoryArchived) &&
				!errors.Is(err, appcatalog.ErrCategoryNotFound) {
				errCh <- err
				return
			}
			errCh <- nil
		}(category.ID(), i)

		close(start)
		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				t.Fatalf("iteration %d unexpected race error: %v", i, err)
			}
		}

		found, err := categoryRepo.FindCategoryByID(ctx, userID, category.ID())
		if err != nil {
			t.Fatalf("find category %d: %v", i, err)
		}
		activeChildren, err := subcategoryRepo.ListByCategoryID(ctx, userID, category.ID(), false)
		if err != nil {
			t.Fatalf("list active subcategories %d: %v", i, err)
		}
		if found.ArchivedAt() != nil && len(activeChildren) > 0 {
			t.Fatalf("iteration %d invariant broken: archived category has %d active subcategories", i, len(activeChildren))
		}
	}
}

func TestArchiveCategoryAndRestoreSubcategoryConcurrencyKeepsInvariant(t *testing.T) {
	pool := openPostgresForAccountRepoTests(t)
	resetAccountsFixtures(t, pool)

	categoryRepo := NewCategoryRepository(pool)
	subcategoryRepo := NewSubcategoryRepository(pool)
	txManager := NewTxManager(pool)

	archiveService := appcatalog.NewArchiveCategoryService(
		categoryRepo,
		subcategoryRepo,
		txManager,
		fixedCategoryClock{now: time.Date(2026, 4, 28, 19, 30, 0, 0, time.UTC)},
	)
	restoreService := appcatalog.NewRestoreSubcategoryService(
		subcategoryRepo,
		categoryRepo,
		fixedCategoryClock{now: time.Date(2026, 4, 28, 19, 30, 1, 0, time.UTC)},
	)

	ctx := context.Background()
	userID := insertAccountTestUser(t, pool, "cat-restore-race@example.com")

	for i := 0; i < 20; i++ {
		category := newCategoryFixture(t, categoryFixtureParams{
			UserID: userID,
			Name:   fmt.Sprintf("Food-Restore-%d", i),
			Type:   "required",
		})
		if err := categoryRepo.Create(ctx, category); err != nil {
			t.Fatalf("create category %d: %v", i, err)
		}

		subArchivedAt := time.Date(2026, 4, 28, 18, 0, 0, i, time.UTC)
		subcategory := newSubcategoryFixture(t, subcategoryFixtureParams{
			UserID:     userID,
			CategoryID: category.ID(),
			Name:       fmt.Sprintf("Sub-%d", i),
			ArchivedAt: &subArchivedAt,
			CreatedAt:  subArchivedAt.Add(-time.Hour),
			UpdatedAt:  subArchivedAt,
		})
		if err := subcategoryRepo.Create(ctx, subcategory); err != nil {
			t.Fatalf("create archived subcategory %d: %v", i, err)
		}

		start := make(chan struct{})
		errCh := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)

		go func(categoryID shared.CategoryID) {
			defer wg.Done()
			<-start
			_, err := archiveService.Archive(ctx, userID, categoryID)
			errCh <- err
		}(category.ID())

		go func(subcategoryID shared.SubcategoryID) {
			defer wg.Done()
			<-start
			_, err := restoreService.Restore(ctx, userID, subcategoryID)
			if err != nil && !errors.Is(err, appcatalog.ErrParentCategoryArchived) {
				errCh <- err
				return
			}
			errCh <- nil
		}(subcategory.ID())

		close(start)
		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				t.Fatalf("iteration %d unexpected race error: %v", i, err)
			}
		}

		foundCategory, err := categoryRepo.FindCategoryByID(ctx, userID, category.ID())
		if err != nil {
			t.Fatalf("find category %d: %v", i, err)
		}
		activeChildren, err := subcategoryRepo.ListByCategoryID(ctx, userID, category.ID(), false)
		if err != nil {
			t.Fatalf("list active subcategories %d: %v", i, err)
		}
		if foundCategory.ArchivedAt() != nil && len(activeChildren) > 0 {
			t.Fatalf("iteration %d invariant broken: archived category has %d active subcategories", i, len(activeChildren))
		}
	}
}
