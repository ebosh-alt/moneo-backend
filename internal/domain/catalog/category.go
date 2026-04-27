package catalog

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var ErrInvalidCategoryName = errors.New("invalid category name")

type Category struct {
	id        shared.CategoryID
	userID    shared.UserID
	name      string
	createdAt time.Time
	updatedAt time.Time
}

func NewCategory(
	id shared.CategoryID,
	userID shared.UserID,
	name string,
	createdAt time.Time,
	updatedAt time.Time,
) (Category, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return Category{}, ErrInvalidCategoryName
	}

	return Category{
		id:        id,
		userID:    userID,
		name:      trimmedName,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

func (c Category) ID() shared.CategoryID {
	return c.id
}

func (c Category) UserID() shared.UserID {
	return c.userID
}

func (c Category) Name() string {
	return c.name
}

func (c Category) CreatedAt() time.Time {
	return c.createdAt
}

func (c Category) UpdatedAt() time.Time {
	return c.updatedAt
}
