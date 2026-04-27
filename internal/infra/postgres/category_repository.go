package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appcatalog "moneo/internal/app/catalog"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CategoryRepository struct {
	pool *pgxpool.Pool
}

func NewCategoryRepository(pool *pgxpool.Pool) *CategoryRepository {
	return &CategoryRepository{pool: pool}
}

type CategoryListInput struct {
	UserID          shared.UserID
	IncludeArchived bool
	Type            *domaincatalog.CategoryType
}

func (r *CategoryRepository) Create(ctx context.Context, category domaincatalog.Category) error {
	const query = `
INSERT INTO categories (
	id,
	user_id,
	name,
	type,
	color,
	sort_order,
	archived_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(
		ctx,
		query,
		string(category.ID()),
		string(category.UserID()),
		category.Name(),
		string(category.Type()),
		category.Color(),
		category.SortOrder(),
		category.ArchivedAt(),
		category.CreatedAt(),
		category.UpdatedAt(),
	); err != nil {
		if isUniqueViolation(err, "ux_categories_user_name_active") {
			return appcatalog.ErrDuplicateActiveCategoryName
		}

		return fmt.Errorf("insert category: %w", err)
	}

	return nil
}

func (r *CategoryRepository) FindCategoryByID(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) (domaincatalog.Category, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	name,
	type,
	color,
	sort_order,
	archived_at,
	created_at,
	updated_at
FROM categories
WHERE id = $1
  AND user_id = $2
LIMIT 1
`

	db := databaseFromContext(ctx, r.pool)
	category, err := scanCategory(db.QueryRow(ctx, query, string(categoryID), string(userID)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domaincatalog.Category{}, appcatalog.ErrCategoryNotFound
		}
		return domaincatalog.Category{}, fmt.Errorf("select category by id: %w", err)
	}

	return category, nil
}

func (r *CategoryRepository) ListCategoriesByUserID(
	ctx context.Context,
	userID shared.UserID,
) ([]domaincatalog.Category, error) {
	return r.ListByUserID(ctx, CategoryListInput{
		UserID:          userID,
		IncludeArchived: false,
	})
}

func (r *CategoryRepository) ListByUserID(
	ctx context.Context,
	input CategoryListInput,
) ([]domaincatalog.Category, error) {
	query := `
SELECT
	id::text,
	user_id::text,
	name,
	type,
	color,
	sort_order,
	archived_at,
	created_at,
	updated_at
FROM categories
WHERE user_id = $1
`

	args := []any{string(input.UserID)}
	if !input.IncludeArchived {
		query += "  AND archived_at IS NULL\n"
	}
	if input.Type != nil {
		query += fmt.Sprintf("  AND type = $%d\n", len(args)+1)
		args = append(args, string(*input.Type))
	}
	query += "ORDER BY sort_order ASC, created_at DESC, id"

	db := databaseFromContext(ctx, r.pool)
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select categories by user id: %w", err)
	}
	defer rows.Close()

	categories := make([]domaincatalog.Category, 0, 8)
	for rows.Next() {
		category, scanErr := scanCategory(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan category row: %w", scanErr)
		}
		categories = append(categories, category)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate category rows: %w", rows.Err())
	}

	return categories, nil
}

type categoryScanner interface {
	Scan(dest ...any) error
}

func scanCategory(row categoryScanner) (domaincatalog.Category, error) {
	var (
		id         string
		userID     string
		name       string
		typeRaw    string
		color      *string
		sortOrder  int
		archivedAt *time.Time
		createdAt  time.Time
		updatedAt  time.Time
	)

	if err := row.Scan(
		&id,
		&userID,
		&name,
		&typeRaw,
		&color,
		&sortOrder,
		&archivedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domaincatalog.Category{}, err
	}

	categoryType, err := domaincatalog.ParseCategoryType(strings.TrimSpace(typeRaw))
	if err != nil {
		return domaincatalog.Category{}, fmt.Errorf("parse category type %q: %w", typeRaw, err)
	}

	category, err := domaincatalog.NewCategoryWithParams(domaincatalog.NewCategoryParams{
		ID:         shared.CategoryID(id),
		UserID:     shared.UserID(userID),
		Name:       name,
		Type:       categoryType,
		Color:      color,
		SortOrder:  sortOrder,
		ArchivedAt: archivedAt,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		return domaincatalog.Category{}, fmt.Errorf("build category from row: %w", err)
	}

	return category, nil
}
