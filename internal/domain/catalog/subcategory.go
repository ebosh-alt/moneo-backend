package catalog

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var ErrInvalidSubcategoryName = errors.New("invalid subcategory name")

type Subcategory struct {
	id         shared.SubcategoryID
	userID     shared.UserID
	categoryID shared.CategoryID
	name       string
	sortOrder  int
	archivedAt *time.Time
	createdAt  time.Time
	updatedAt  time.Time
}

type NewSubcategoryParams struct {
	ID         shared.SubcategoryID
	UserID     shared.UserID
	CategoryID shared.CategoryID
	Name       string
	SortOrder  int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewSubcategoryWithParams(params NewSubcategoryParams) (Subcategory, error) {
	trimmedName := strings.TrimSpace(params.Name)
	if trimmedName == "" {
		return Subcategory{}, ErrInvalidSubcategoryName
	}

	return Subcategory{
		id:         params.ID,
		userID:     params.UserID,
		categoryID: params.CategoryID,
		name:       trimmedName,
		sortOrder:  params.SortOrder,
		archivedAt: params.ArchivedAt,
		createdAt:  params.CreatedAt,
		updatedAt:  params.UpdatedAt,
	}, nil
}

func NewSubcategory(
	id shared.SubcategoryID,
	userID shared.UserID,
	categoryID shared.CategoryID,
	name string,
	createdAt time.Time,
	updatedAt time.Time,
) (Subcategory, error) {
	return NewSubcategoryWithParams(NewSubcategoryParams{
		ID:         id,
		UserID:     userID,
		CategoryID: categoryID,
		Name:       name,
		SortOrder:  100,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	})
}

func (s Subcategory) ID() shared.SubcategoryID {
	return s.id
}

func (s Subcategory) UserID() shared.UserID {
	return s.userID
}

func (s Subcategory) CategoryID() shared.CategoryID {
	return s.categoryID
}

func (s Subcategory) Name() string {
	return s.name
}

func (s Subcategory) SortOrder() int {
	return s.sortOrder
}

func (s Subcategory) ArchivedAt() *time.Time {
	return s.archivedAt
}

func (s Subcategory) CreatedAt() time.Time {
	return s.createdAt
}

func (s Subcategory) UpdatedAt() time.Time {
	return s.updatedAt
}
