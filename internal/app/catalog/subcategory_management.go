package catalog

import (
	"context"
	"errors"
	"fmt"
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
	if _, err := s.categories.FindCategoryByID(ctx, input.UserID, input.CategoryID); err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find parent category by id: %w", err)
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

func resolveSubcategorySortOrder(sortOrder *int) int {
	if sortOrder == nil {
		return 100
	}

	return *sortOrder
}
