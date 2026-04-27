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
	createdAt  time.Time
	updatedAt  time.Time
}

func NewSubcategory(
	id shared.SubcategoryID,
	userID shared.UserID,
	categoryID shared.CategoryID,
	name string,
	createdAt time.Time,
	updatedAt time.Time,
) (Subcategory, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return Subcategory{}, ErrInvalidSubcategoryName
	}

	return Subcategory{
		id:         id,
		userID:     userID,
		categoryID: categoryID,
		name:       trimmedName,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
	}, nil
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

func (s Subcategory) CreatedAt() time.Time {
	return s.createdAt
}

func (s Subcategory) UpdatedAt() time.Time {
	return s.updatedAt
}
