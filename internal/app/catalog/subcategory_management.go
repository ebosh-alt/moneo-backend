package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
)

type SubcategoryIDGenerator interface {
	NewSubcategoryID() shared.SubcategoryID
}

type SubcategoryClock interface {
	Now() time.Time
}

type SubcategoryParentCategoryReader interface {
	FindCategoryByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
}

type CreateSubcategoryRepository interface {
	Create(ctx context.Context, subcategory domaincatalog.Subcategory) error
}

type CreateSubcategoryInput struct {
	UserID     shared.UserID
	CategoryID shared.CategoryID
	Name       string
	SortOrder  *int
}

type CreateSubcategoryService struct {
	repo       CreateSubcategoryRepository
	categories SubcategoryParentCategoryReader
	idgen      SubcategoryIDGenerator
	clock      SubcategoryClock
}

func NewCreateSubcategoryService(
	repo CreateSubcategoryRepository,
	categories SubcategoryParentCategoryReader,
	idgen SubcategoryIDGenerator,
	clock SubcategoryClock,
) *CreateSubcategoryService {
	return &CreateSubcategoryService{
		repo:       repo,
		categories: categories,
		idgen:      idgen,
		clock:      clock,
	}
}

func (s *CreateSubcategoryService) Create(
	ctx context.Context,
	input CreateSubcategoryInput,
) (domaincatalog.Subcategory, error) {
	parentCategory, err := s.categories.FindCategoryByID(ctx, input.UserID, input.CategoryID)
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find parent category by id: %w", err)
	}
	if parentCategory.ArchivedAt() != nil {
		return domaincatalog.Subcategory{}, ErrParentCategoryArchived
	}

	now := s.clock.Now().UTC()
	subcategory, err := domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         s.idgen.NewSubcategoryID(),
		UserID:     input.UserID,
		CategoryID: input.CategoryID,
		Name:       input.Name,
		SortOrder:  resolveSubcategorySortOrder(input.SortOrder),
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		return domaincatalog.Subcategory{}, err
	}

	if err := s.repo.Create(ctx, subcategory); err != nil {
		if errors.Is(err, ErrDuplicateActiveSubcategoryName) {
			return domaincatalog.Subcategory{}, ErrSubcategoryNameAlreadyExists
		}

		return domaincatalog.Subcategory{}, fmt.Errorf("create subcategory: %w", err)
	}

	return subcategory, nil
}

type ListSubcategoriesByCategoryRepository interface {
	ListByCategoryID(
		ctx context.Context,
		userID shared.UserID,
		categoryID shared.CategoryID,
		includeArchived bool,
	) ([]domaincatalog.Subcategory, error)
}

type ListSubcategoriesByCategoryInput struct {
	UserID          shared.UserID
	CategoryID      shared.CategoryID
	IncludeArchived bool
}

type ListSubcategoriesByCategoryService struct {
	repo       ListSubcategoriesByCategoryRepository
	categories SubcategoryParentCategoryReader
}

func NewListSubcategoriesByCategoryService(
	repo ListSubcategoriesByCategoryRepository,
	categories SubcategoryParentCategoryReader,
) *ListSubcategoriesByCategoryService {
	return &ListSubcategoriesByCategoryService{
		repo:       repo,
		categories: categories,
	}
}

func (s *ListSubcategoriesByCategoryService) List(
	ctx context.Context,
	input ListSubcategoriesByCategoryInput,
) ([]domaincatalog.Subcategory, error) {
	if _, err := s.categories.FindCategoryByID(ctx, input.UserID, input.CategoryID); err != nil {
		return nil, fmt.Errorf("find parent category by id: %w", err)
	}

	subcategories, err := s.repo.ListByCategoryID(
		ctx,
		input.UserID,
		input.CategoryID,
		input.IncludeArchived,
	)
	if err != nil {
		return nil, fmt.Errorf("list subcategories by category id: %w", err)
	}

	return subcategories, nil
}

type UpdateSubcategoryRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID) (domaincatalog.Subcategory, error)
	UpdateByID(ctx context.Context, subcategory domaincatalog.Subcategory) error
}

type UpdateSubcategoryInput struct {
	UserID        shared.UserID
	SubcategoryID shared.SubcategoryID
	Name          *string
	SortOrder     *int
}

type UpdateSubcategoryService struct {
	repo  UpdateSubcategoryRepository
	clock SubcategoryClock
}

func NewUpdateSubcategoryService(
	repo UpdateSubcategoryRepository,
	clock SubcategoryClock,
) *UpdateSubcategoryService {
	return &UpdateSubcategoryService{
		repo:  repo,
		clock: clock,
	}
}

func (s *UpdateSubcategoryService) Update(
	ctx context.Context,
	input UpdateSubcategoryInput,
) (domaincatalog.Subcategory, error) {
	subcategory, err := s.repo.FindByID(ctx, input.UserID, input.SubcategoryID)
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find subcategory by id: %w", err)
	}

	name := subcategory.Name()
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}

	sortOrder := subcategory.SortOrder()
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	updatedAt := s.clock.Now().UTC()
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

	if err := s.repo.UpdateByID(ctx, updated); err != nil {
		if errors.Is(err, ErrDuplicateActiveSubcategoryName) {
			return domaincatalog.Subcategory{}, ErrSubcategoryNameAlreadyExists
		}
		return domaincatalog.Subcategory{}, fmt.Errorf("update subcategory by id: %w", err)
	}

	return updated, nil
}

type ArchiveSubcategoryRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID) (domaincatalog.Subcategory, error)
	ArchiveByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID, archivedAt time.Time) error
}

type ArchiveSubcategoryService struct {
	repo  ArchiveSubcategoryRepository
	clock SubcategoryClock
}

func NewArchiveSubcategoryService(
	repo ArchiveSubcategoryRepository,
	clock SubcategoryClock,
) *ArchiveSubcategoryService {
	return &ArchiveSubcategoryService{
		repo:  repo,
		clock: clock,
	}
}

func (s *ArchiveSubcategoryService) Archive(
	ctx context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	subcategory, err := s.repo.FindByID(ctx, userID, subcategoryID)
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find subcategory by id: %w", err)
	}
	if subcategory.ArchivedAt() != nil {
		return subcategory, nil
	}

	archivedAt := s.clock.Now().UTC()
	if err := s.repo.ArchiveByID(ctx, userID, subcategoryID, archivedAt); err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("archive subcategory by id: %w", err)
	}

	return buildArchivedSubcategory(subcategory, archivedAt)
}

type RestoreSubcategoryRepository interface {
	FindByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID) (domaincatalog.Subcategory, error)
	RestoreByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID, updatedAt time.Time) error
}

type RestoreSubcategoryService struct {
	repo       RestoreSubcategoryRepository
	categories SubcategoryParentCategoryReader
	clock      SubcategoryClock
}

func NewRestoreSubcategoryService(
	repo RestoreSubcategoryRepository,
	categories SubcategoryParentCategoryReader,
	clock SubcategoryClock,
) *RestoreSubcategoryService {
	return &RestoreSubcategoryService{
		repo:       repo,
		categories: categories,
		clock:      clock,
	}
}

func (s *RestoreSubcategoryService) Restore(
	ctx context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	subcategory, err := s.repo.FindByID(ctx, userID, subcategoryID)
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find subcategory by id: %w", err)
	}
	if subcategory.ArchivedAt() == nil {
		return subcategory, nil
	}

	parentCategory, err := s.categories.FindCategoryByID(ctx, userID, subcategory.CategoryID())
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find parent category by id: %w", err)
	}
	if parentCategory.ArchivedAt() != nil {
		return domaincatalog.Subcategory{}, ErrParentCategoryArchived
	}

	updatedAt := s.clock.Now().UTC()
	if err := s.repo.RestoreByID(ctx, userID, subcategoryID, updatedAt); err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("restore subcategory by id: %w", err)
	}

	return buildRestoredSubcategory(subcategory, updatedAt)
}

func buildArchivedSubcategory(
	subcategory domaincatalog.Subcategory,
	archivedAt time.Time,
) (domaincatalog.Subcategory, error) {
	archivedAtCopy := archivedAt
	return domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         subcategory.ID(),
		UserID:     subcategory.UserID(),
		CategoryID: subcategory.CategoryID(),
		Name:       subcategory.Name(),
		SortOrder:  subcategory.SortOrder(),
		ArchivedAt: &archivedAtCopy,
		CreatedAt:  subcategory.CreatedAt(),
		UpdatedAt:  archivedAtCopy,
	})
}

func buildRestoredSubcategory(
	subcategory domaincatalog.Subcategory,
	updatedAt time.Time,
) (domaincatalog.Subcategory, error) {
	return domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         subcategory.ID(),
		UserID:     subcategory.UserID(),
		CategoryID: subcategory.CategoryID(),
		Name:       subcategory.Name(),
		SortOrder:  subcategory.SortOrder(),
		ArchivedAt: nil,
		CreatedAt:  subcategory.CreatedAt(),
		UpdatedAt:  updatedAt,
	})
}

func resolveSubcategorySortOrder(sortOrder *int) int {
	if sortOrder == nil {
		return 100
	}

	return *sortOrder
}
