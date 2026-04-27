-- +goose Up
CREATE TABLE categories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    color TEXT,
    sort_order INTEGER NOT NULL DEFAULT 100,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT categories_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT categories_type_check CHECK (type IN ('required', 'flexible', 'saving', 'investment', 'debt', 'income')),
    CONSTRAINT categories_color_check CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$')
);

CREATE INDEX idx_categories_user_id
    ON categories (user_id);

CREATE INDEX idx_categories_user_archived
    ON categories (user_id, archived_at);

CREATE INDEX idx_categories_user_type
    ON categories (user_id, type);

CREATE UNIQUE INDEX ux_categories_user_name_active
    ON categories (user_id, lower(name))
    WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ux_categories_user_name_active;
DROP INDEX IF EXISTS idx_categories_user_type;
DROP INDEX IF EXISTS idx_categories_user_archived;
DROP INDEX IF EXISTS idx_categories_user_id;
DROP TABLE IF EXISTS categories;
