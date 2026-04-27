package bootstrap

import (
	"context"

	appcatalog "moneo/internal/app/catalog"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
)

type emptyCategoryRepository struct{}

func (emptyCategoryRepository) FindCategoryByID(
	_ context.Context,
	_ shared.UserID,
	_ shared.CategoryID,
) (domaincatalog.Category, error) {
	return domaincatalog.Category{}, appcatalog.ErrCategoryNotFound
}

func (emptyCategoryRepository) ListCategoriesByUserID(
	_ context.Context,
	_ shared.UserID,
) ([]domaincatalog.Category, error) {
	return []domaincatalog.Category{}, nil
}

type emptySubcategoryRepository struct{}

func (emptySubcategoryRepository) FindSubcategoryByID(
	_ context.Context,
	_ shared.UserID,
	_ shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
}

func (emptySubcategoryRepository) ListSubcategoriesByUserID(
	_ context.Context,
	_ shared.UserID,
) ([]domaincatalog.Subcategory, error) {
	return []domaincatalog.Subcategory{}, nil
}
