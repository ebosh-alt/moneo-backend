package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
)

type CategoryIDGenerator interface {
	NewCategoryID() shared.CategoryID
}

type CategoryClock interface {
	Now() time.Time
}

type CreateCategoryRepository interface {
	Create(ctx context.Context, category domaincatalog.Category) error
}

type CreateCategoryInput struct {
	UserID    shared.UserID
	Name      string
	Type      domaincatalog.CategoryType
	Color     *string
	SortOrder *int
}

type CreateCategoryService struct {
	repo  CreateCategoryRepository
	idgen CategoryIDGenerator
	clock CategoryClock
}

func NewCreateCategoryService(
	repo CreateCategoryRepository,
	idgen CategoryIDGenerator,
	clock CategoryClock,
) *CreateCategoryService {
	return &CreateCategoryService{
		repo:  repo,
		idgen: idgen,
		clock: clock,
	}
}

func (s *CreateCategoryService) Create(ctx context.Context, input CreateCategoryInput) (domaincatalog.Category, error) {
	now := s.clock.Now().UTC()
	category, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:        s.idgen.NewCategoryID(),
		UserID:    input.UserID,
		Name:      input.Name,
		Type:      input.Type,
		Color:     input.Color,
		SortOrder: resolveCategorySortOrder(input.SortOrder),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return domaincatalog.Category{}, err
	}

	if err := s.repo.Create(ctx, category); err != nil {
		if errors.Is(err, ErrDuplicateActiveCategoryName) {
			return domaincatalog.Category{}, ErrCategoryNameAlreadyExists
		}

		return domaincatalog.Category{}, fmt.Errorf("create category: %w", err)
	}

	return category, nil
}

type ListCategoriesRepository interface {
	ListByUserIDWithArchive(ctx context.Context, userID shared.UserID, includeArchived bool) ([]domaincatalog.Category, error)
}

type CategorySort string

const (
	CategorySortSortOrderAsc  CategorySort = "sortOrder:asc"
	CategorySortNameAsc       CategorySort = "name:asc"
	CategorySortCreatedAtDesc CategorySort = "createdAt:desc"
)

type ListCategoriesInput struct {
	UserID          shared.UserID
	IncludeArchived bool
	Type            *domaincatalog.CategoryType
	Sort            CategorySort
}

type ListCategoriesService struct {
	repo ListCategoriesRepository
}

func NewListCategoriesService(repo ListCategoriesRepository) *ListCategoriesService {
	return &ListCategoriesService{repo: repo}
}

func (s *ListCategoriesService) ListByUser(
	ctx context.Context,
	input ListCategoriesInput,
) ([]domaincatalog.Category, error) {
	categories, err := s.repo.ListByUserIDWithArchive(ctx, input.UserID, input.IncludeArchived)
	if err != nil {
		return nil, fmt.Errorf("list categories by user id: %w", err)
	}

	filtered := make([]domaincatalog.Category, 0, len(categories))
	for _, category := range categories {
		if input.Type != nil && category.Type() != *input.Type {
			continue
		}
		filtered = append(filtered, category)
	}

	sortMode := input.Sort
	if sortMode == "" {
		sortMode = CategorySortSortOrderAsc
	}

	slices.SortFunc(filtered, func(left, right domaincatalog.Category) int {
		switch sortMode {
		case CategorySortNameAsc:
			leftName := strings.ToLower(left.Name())
			rightName := strings.ToLower(right.Name())
			if leftName < rightName {
				return -1
			}
			if leftName > rightName {
				return 1
			}
		case CategorySortCreatedAtDesc:
			if left.CreatedAt().After(right.CreatedAt()) {
				return -1
			}
			if left.CreatedAt().Before(right.CreatedAt()) {
				return 1
			}
		default:
			if left.SortOrder() < right.SortOrder() {
				return -1
			}
			if left.SortOrder() > right.SortOrder() {
				return 1
			}
		}

		leftID := string(left.ID())
		rightID := string(right.ID())
		if leftID < rightID {
			return -1
		}
		if leftID > rightID {
			return 1
		}
		return 0
	})

	return filtered, nil
}

type UpdateCategoryRepository interface {
	FindCategoryByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
	UpdateByID(ctx context.Context, category domaincatalog.Category, expectedUpdatedAt time.Time) error
}

type UpdateCategoryInput struct {
	UserID     shared.UserID
	CategoryID shared.CategoryID
	Name       *string
	Type       *domaincatalog.CategoryType
	Color      *string
	SortOrder  *int
}

type UpdateCategoryService struct {
	repo  UpdateCategoryRepository
	clock CategoryClock
}

func NewUpdateCategoryService(repo UpdateCategoryRepository, clock CategoryClock) *UpdateCategoryService {
	return &UpdateCategoryService{
		repo:  repo,
		clock: clock,
	}
}

func (s *UpdateCategoryService) Update(ctx context.Context, input UpdateCategoryInput) (domaincatalog.Category, error) {
	category, err := s.repo.FindCategoryByID(ctx, input.UserID, input.CategoryID)
	if err != nil {
		return domaincatalog.Category{}, fmt.Errorf("find category by id: %w", err)
	}

	name := category.Name()
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}

	categoryType := category.Type()
	if input.Type != nil {
		categoryType = *input.Type
	}

	color := category.Color()
	if input.Color != nil {
		colorValue := strings.TrimSpace(*input.Color)
		color = &colorValue
	}

	sortOrder := category.SortOrder()
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	updatedAt := s.clock.Now().UTC()
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

	if err := s.repo.UpdateByID(ctx, updated, category.UpdatedAt()); err != nil {
		if errors.Is(err, ErrDuplicateActiveCategoryName) {
			return domaincatalog.Category{}, ErrCategoryNameAlreadyExists
		}
		if errors.Is(err, ErrConcurrentCategoryUpdate) {
			return domaincatalog.Category{}, ErrConcurrentCategoryUpdate
		}

		return domaincatalog.Category{}, fmt.Errorf("update category by id: %w", err)
	}

	return updated, nil
}

type CategorySubcategoryArchiveRepository interface {
	ArchiveByCategoryID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID, archivedAt time.Time) error
	RestoreByCategoryID(
		ctx context.Context,
		userID shared.UserID,
		categoryID shared.CategoryID,
		updatedAt time.Time,
		cascadeArchivedAt time.Time,
	) error
}

type CategoryTxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type ArchiveCategoryRepository interface {
	FindCategoryByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
	ArchiveByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID, archivedAt time.Time) error
}

type ArchiveCategoryService struct {
	repo          ArchiveCategoryRepository
	subcategories CategorySubcategoryArchiveRepository
	txManager     CategoryTxManager
	clock         CategoryClock
}

func NewArchiveCategoryService(
	repo ArchiveCategoryRepository,
	subcategories CategorySubcategoryArchiveRepository,
	txManager CategoryTxManager,
	clock CategoryClock,
) *ArchiveCategoryService {
	if txManager == nil {
		txManager = noopCategoryTxManager{}
	}

	return &ArchiveCategoryService{
		repo:          repo,
		subcategories: subcategories,
		txManager:     txManager,
		clock:         clock,
	}
}

func (s *ArchiveCategoryService) Archive(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) (domaincatalog.Category, error) {
	var archivedCategory domaincatalog.Category
	if err := s.txManager.WithinTx(ctx, func(txCtx context.Context) error {
		category, err := s.repo.FindCategoryByID(txCtx, userID, categoryID)
		if err != nil {
			return fmt.Errorf("find category by id: %w", err)
		}
		if category.ArchivedAt() != nil {
			archivedCategory = category
			return nil
		}

		archivedAt := s.clock.Now().UTC()
		if err := s.repo.ArchiveByID(txCtx, userID, categoryID, archivedAt); err != nil {
			return fmt.Errorf("archive category by id: %w", err)
		}
		if s.subcategories != nil {
			if err := s.subcategories.ArchiveByCategoryID(txCtx, userID, categoryID, archivedAt); err != nil {
				return fmt.Errorf("archive subcategories by category id: %w", err)
			}
		}

		archived, buildErr := buildArchivedCategory(category, archivedAt)
		if buildErr != nil {
			return buildErr
		}
		archivedCategory = archived
		return nil
	}); err != nil {
		return domaincatalog.Category{}, err
	}

	return archivedCategory, nil
}

type RestoreCategoryRepository interface {
	FindCategoryByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
	RestoreByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID, updatedAt time.Time) error
}

type RestoreCategoryService struct {
	repo          RestoreCategoryRepository
	subcategories CategorySubcategoryArchiveRepository
	txManager     CategoryTxManager
	clock         CategoryClock
}

func NewRestoreCategoryService(
	repo RestoreCategoryRepository,
	subcategories CategorySubcategoryArchiveRepository,
	txManager CategoryTxManager,
	clock CategoryClock,
) *RestoreCategoryService {
	if txManager == nil {
		txManager = noopCategoryTxManager{}
	}

	return &RestoreCategoryService{
		repo:          repo,
		subcategories: subcategories,
		txManager:     txManager,
		clock:         clock,
	}
}

func (s *RestoreCategoryService) Restore(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) (domaincatalog.Category, error) {
	var restoredCategory domaincatalog.Category
	if err := s.txManager.WithinTx(ctx, func(txCtx context.Context) error {
		category, err := s.repo.FindCategoryByID(txCtx, userID, categoryID)
		if err != nil {
			return fmt.Errorf("find category by id: %w", err)
		}
		if category.ArchivedAt() == nil {
			restoredCategory = category
			return nil
		}

		updatedAt := s.clock.Now().UTC()
		if err := s.repo.RestoreByID(txCtx, userID, categoryID, updatedAt); err != nil {
			if errors.Is(err, ErrDuplicateActiveCategoryName) {
				return ErrCategoryNameAlreadyExists
			}
			return fmt.Errorf("restore category by id: %w", err)
		}
		if s.subcategories != nil {
			if err := s.subcategories.RestoreByCategoryID(
				txCtx,
				userID,
				categoryID,
				updatedAt,
				*category.ArchivedAt(),
			); err != nil {
				return fmt.Errorf("restore subcategories by category id: %w", err)
			}
		}

		restored, buildErr := buildRestoredCategory(category, updatedAt)
		if buildErr != nil {
			return buildErr
		}
		restoredCategory = restored
		return nil
	}); err != nil {
		return domaincatalog.Category{}, err
	}

	return restoredCategory, nil
}

func buildArchivedCategory(
	category domaincatalog.Category,
	archivedAt time.Time,
) (domaincatalog.Category, error) {
	archivedAtCopy := archivedAt
	return domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:         category.ID(),
		UserID:     category.UserID(),
		Name:       category.Name(),
		Type:       category.Type(),
		Color:      category.Color(),
		SortOrder:  category.SortOrder(),
		ArchivedAt: &archivedAtCopy,
		CreatedAt:  category.CreatedAt(),
		UpdatedAt:  archivedAtCopy,
	})
}

func buildRestoredCategory(
	category domaincatalog.Category,
	updatedAt time.Time,
) (domaincatalog.Category, error) {
	return domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:        category.ID(),
		UserID:    category.UserID(),
		Name:      category.Name(),
		Type:      category.Type(),
		Color:     category.Color(),
		SortOrder: category.SortOrder(),
		CreatedAt: category.CreatedAt(),
		UpdatedAt: updatedAt,
	})
}

func resolveCategorySortOrder(sortOrder *int) int {
	if sortOrder == nil {
		return 100
	}
	return *sortOrder
}

type noopCategoryTxManager struct{}

func (noopCategoryTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
