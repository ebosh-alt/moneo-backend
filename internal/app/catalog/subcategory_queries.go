package catalog

import (
	"context"
	"fmt"

	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"
)

type SubcategoryQueryRepository interface {
	FindSubcategoryByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID) (domaincatalog.Subcategory, error)
	ListSubcategoriesByUserID(ctx context.Context, userID shared.UserID) ([]domaincatalog.Subcategory, error)
}

type SubcategoryQueryService struct {
	repo SubcategoryQueryRepository
}

func NewSubcategoryQueryService(repo SubcategoryQueryRepository) *SubcategoryQueryService {
	return &SubcategoryQueryService{repo: repo}
}

func (s *SubcategoryQueryService) GetByID(
	ctx context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	subcategory, err := s.repo.FindSubcategoryByID(ctx, userID, subcategoryID)
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("find subcategory by id: %w", err)
	}

	return subcategory, nil
}

func (s *SubcategoryQueryService) ListByUserID(
	ctx context.Context,
	userID shared.UserID,
) ([]domaincatalog.Subcategory, error) {
	subcategories, err := s.repo.ListSubcategoriesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list subcategories by user id: %w", err)
	}

	return subcategories, nil
}
