package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	appcatalog "moneo/internal/app/catalog"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SubcategoryRepository struct {
	pool *pgxpool.Pool
}

func NewSubcategoryRepository(pool *pgxpool.Pool) *SubcategoryRepository {
	return &SubcategoryRepository{pool: pool}
}

func (r *SubcategoryRepository) Create(ctx context.Context, subcategory domaincatalog.Subcategory) error {
	const query = `
INSERT INTO subcategories (
	id,
	user_id,
	category_id,
	name,
	sort_order,
	archived_at,
	created_at,
	updated_at
)
SELECT
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8
FROM categories c
WHERE c.id = $3
  AND c.user_id = $2
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(
		ctx,
		query,
		string(subcategory.ID()),
		string(subcategory.UserID()),
		string(subcategory.CategoryID()),
		subcategory.Name(),
		subcategory.SortOrder(),
		subcategory.ArchivedAt(),
		subcategory.CreatedAt(),
		subcategory.UpdatedAt(),
	)
	if err != nil {
		if isUniqueViolation(err, "ux_subcategories_category_name_active") {
			return appcatalog.ErrDuplicateActiveSubcategoryName
		}

		return fmt.Errorf("insert subcategory: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appcatalog.ErrCategoryNotFound
	}

	return nil
}

func (r *SubcategoryRepository) FindSubcategoryByID(
	ctx context.Context,
	userID shared.UserID,
	subcategoryID shared.SubcategoryID,
) (domaincatalog.Subcategory, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	category_id::text,
	name,
	sort_order,
	archived_at,
	created_at,
	updated_at
FROM subcategories
WHERE id = $1
  AND user_id = $2
  AND archived_at IS NULL
LIMIT 1
`

	db := databaseFromContext(ctx, r.pool)
	subcategory, err := scanSubcategory(db.QueryRow(ctx, query, string(subcategoryID), string(userID)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domaincatalog.Subcategory{}, appcatalog.ErrSubcategoryNotFound
		}
		return domaincatalog.Subcategory{}, fmt.Errorf("select subcategory by id: %w", err)
	}

	return subcategory, nil
}

func (r *SubcategoryRepository) ListSubcategoriesByUserID(
	ctx context.Context,
	userID shared.UserID,
) ([]domaincatalog.Subcategory, error) {
	return r.list(ctx, userID, nil, false)
}

func (r *SubcategoryRepository) ListByCategoryID(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
	includeArchived bool,
) ([]domaincatalog.Subcategory, error) {
	if err := r.ensureCategoryOwnedByUser(ctx, userID, categoryID); err != nil {
		return nil, err
	}

	return r.list(ctx, userID, &categoryID, includeArchived)
}

func (r *SubcategoryRepository) ArchiveByCategoryID(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
	archivedAt time.Time,
) error {
	if err := r.ensureCategoryOwnedByUser(ctx, userID, categoryID); err != nil {
		return err
	}

	const query = `
UPDATE subcategories
SET archived_at = $3,
    updated_at = $3
WHERE user_id = $1
  AND category_id = $2
  AND archived_at IS NULL
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(ctx, query, string(userID), string(categoryID), archivedAt); err != nil {
		return fmt.Errorf("archive subcategories by category id: %w", err)
	}

	return nil
}

func (r *SubcategoryRepository) RestoreByCategoryID(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
	updatedAt time.Time,
) error {
	if err := r.ensureCategoryOwnedByUser(ctx, userID, categoryID); err != nil {
		return err
	}

	const query = `
UPDATE subcategories
SET archived_at = NULL,
    updated_at = $3
WHERE user_id = $1
  AND category_id = $2
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(ctx, query, string(userID), string(categoryID), updatedAt); err != nil {
		return fmt.Errorf("restore subcategories by category id: %w", err)
	}

	return nil
}

func (r *SubcategoryRepository) list(
	ctx context.Context,
	userID shared.UserID,
	categoryID *shared.CategoryID,
	includeArchived bool,
) ([]domaincatalog.Subcategory, error) {
	query := `
SELECT
	id::text,
	user_id::text,
	category_id::text,
	name,
	sort_order,
	archived_at,
	created_at,
	updated_at
FROM subcategories
WHERE user_id = $1
`

	args := []any{string(userID)}
	if categoryID != nil {
		query += fmt.Sprintf("  AND category_id = $%d\n", len(args)+1)
		args = append(args, string(*categoryID))
	}
	if !includeArchived {
		query += "  AND archived_at IS NULL\n"
	}
	query += "ORDER BY sort_order ASC, created_at DESC, id"

	db := databaseFromContext(ctx, r.pool)
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select subcategories: %w", err)
	}
	defer rows.Close()

	subcategories := make([]domaincatalog.Subcategory, 0, 8)
	for rows.Next() {
		subcategory, scanErr := scanSubcategory(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan subcategory row: %w", scanErr)
		}
		subcategories = append(subcategories, subcategory)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate subcategory rows: %w", rows.Err())
	}

	return subcategories, nil
}

func (r *SubcategoryRepository) ensureCategoryOwnedByUser(
	ctx context.Context,
	userID shared.UserID,
	categoryID shared.CategoryID,
) error {
	const query = `
SELECT 1
FROM categories
WHERE id = $1
  AND user_id = $2
LIMIT 1
`

	db := databaseFromContext(ctx, r.pool)
	var marker int
	if err := db.QueryRow(ctx, query, string(categoryID), string(userID)).Scan(&marker); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appcatalog.ErrCategoryNotFound
		}
		return fmt.Errorf("ensure category ownership: %w", err)
	}

	return nil
}

type subcategoryScanner interface {
	Scan(dest ...any) error
}

func scanSubcategory(row subcategoryScanner) (domaincatalog.Subcategory, error) {
	var (
		id         string
		userID     string
		categoryID string
		name       string
		sortOrder  int
		archivedAt *time.Time
		createdAt  time.Time
		updatedAt  time.Time
	)

	if err := row.Scan(
		&id,
		&userID,
		&categoryID,
		&name,
		&sortOrder,
		&archivedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domaincatalog.Subcategory{}, err
	}

	subcategory, err := domaincatalog.NewSubcategoryWithParams(domaincatalog.NewSubcategoryParams{
		ID:         shared.SubcategoryID(id),
		UserID:     shared.UserID(userID),
		CategoryID: shared.CategoryID(categoryID),
		Name:       name,
		SortOrder:  sortOrder,
		ArchivedAt: archivedAt,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		return domaincatalog.Subcategory{}, fmt.Errorf("build subcategory from row: %w", err)
	}

	return subcategory, nil
}
