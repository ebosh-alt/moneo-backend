package catalog

import (
	"context"
	"fmt"

	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
)

type CategoryQueryRepository interface {
	FindCategoryByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
	ListCategoriesByUserID(ctx context.Context, userID shared.UserID) ([]domaincatalog.Category, error)
}

type CategoryQueryService struct {
	repo CategoryQueryRepository
}

func NewCategoryQueryService(repo CategoryQueryRepository) *CategoryQueryService {
	return &CategoryQueryService{repo: repo}
}

func (s *CategoryQueryService) GetByID(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) (domaincatalog.Category, error) {
	category, err := s.repo.FindCategoryByID(ctx, userID, categoryID)
	if err != nil {
		return domaincatalog.Category{}, fmt.Errorf("find category by id: %w", err)
	}

	return category, nil
}

func (s *CategoryQueryService) ListByUserID(ctx context.Context, userID shared.UserID) ([]domaincatalog.Category, error) {
	categories, err := s.repo.ListCategoriesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list categories by user id: %w", err)
	}

	return categories, nil
}
