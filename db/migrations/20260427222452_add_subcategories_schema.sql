-- +goose Up
CREATE TABLE subcategories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 100,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT subcategories_name_not_empty CHECK (length(trim(name)) > 0)
);

CREATE INDEX idx_subcategories_user_id
    ON subcategories (user_id);

CREATE INDEX idx_subcategories_category_id
    ON subcategories (category_id);

CREATE INDEX idx_subcategories_user_archived
    ON subcategories (user_id, archived_at);

CREATE UNIQUE INDEX ux_subcategories_category_name_active
    ON subcategories (category_id, lower(name))
    WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ux_subcategories_category_name_active;
DROP INDEX IF EXISTS idx_subcategories_user_archived;
DROP INDEX IF EXISTS idx_subcategories_category_id;
DROP INDEX IF EXISTS idx_subcategories_user_id;
DROP TABLE IF EXISTS subcategories;
