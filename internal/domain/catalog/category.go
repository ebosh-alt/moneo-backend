package catalog

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var (
	ErrInvalidCategoryName  = errors.New("invalid category name")
	ErrInvalidCategoryType  = errors.New("invalid category type")
	ErrInvalidCategoryColor = errors.New("invalid category color")
)

var categoryColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type CategoryType string

const (
	CategoryTypeRequired   CategoryType = "required"
	CategoryTypeFlexible   CategoryType = "flexible"
	CategoryTypeSaving     CategoryType = "saving"
	CategoryTypeInvestment CategoryType = "investment"
	CategoryTypeDebt       CategoryType = "debt"
	CategoryTypeIncome     CategoryType = "income"
)

type Category struct {
	id           shared.CategoryID
	userID       shared.UserID
	name         string
	categoryType CategoryType
	color        *string
	sortOrder    int
	archivedAt   *time.Time
	createdAt    time.Time
	updatedAt    time.Time
}

type NewCategoryParams struct {
	ID         shared.CategoryID
	UserID     shared.UserID
	Name       string
	Type       CategoryType
	Color      *string
	SortOrder  int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewCategoryWithParams(params NewCategoryParams) (Category, error) {
	trimmedName := strings.TrimSpace(params.Name)
	if trimmedName == "" {
		return Category{}, ErrInvalidCategoryName
	}

	if !params.Type.IsSupported() {
		return Category{}, ErrInvalidCategoryType
	}

	var color *string
	if params.Color != nil {
		trimmedColor := strings.TrimSpace(*params.Color)
		if !categoryColorPattern.MatchString(trimmedColor) {
			return Category{}, ErrInvalidCategoryColor
		}
		color = &trimmedColor
	}

	return Category{
		id:           params.ID,
		userID:       params.UserID,
		name:         trimmedName,
		categoryType: params.Type,
		color:        color,
		sortOrder:    params.SortOrder,
		archivedAt:   params.ArchivedAt,
		createdAt:    params.CreatedAt,
		updatedAt:    params.UpdatedAt,
	}, nil
}

func NewCategory(
	id shared.CategoryID,
	userID shared.UserID,
	name string,
	createdAt time.Time,
	updatedAt time.Time,
) (Category, error) {
	return NewCategoryWithParams(NewCategoryParams{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Type:      CategoryTypeFlexible,
		SortOrder: 100,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	})
}

func ParseCategoryType(value string) (CategoryType, error) {
	categoryType := CategoryType(value)
	if !categoryType.IsSupported() {
		return "", ErrInvalidCategoryType
	}
	return categoryType, nil
}

func (t CategoryType) IsSupported() bool {
	switch t {
	case CategoryTypeRequired,
		CategoryTypeFlexible,
		CategoryTypeSaving,
		CategoryTypeInvestment,
		CategoryTypeDebt,
		CategoryTypeIncome:
		return true
	default:
		return false
	}
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

func (c Category) Type() CategoryType {
	return c.categoryType
}

func (c Category) Color() *string {
	return c.color
}

func (c Category) SortOrder() int {
	return c.sortOrder
}

func (c Category) ArchivedAt() *time.Time {
	return c.archivedAt
}

func (c Category) CreatedAt() time.Time {
	return c.createdAt
}

func (c Category) UpdatedAt() time.Time {
	return c.updatedAt
}
