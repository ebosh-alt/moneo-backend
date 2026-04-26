-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    normalized_email TEXT GENERATED ALWAYS AS (lower(trim(email))) STORED,
    password_hash TEXT NOT NULL,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_users_normalized_email ON users (normalized_email);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    user_agent TEXT,
    ip INET,
    device_name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_sessions_active_refresh_token_hash
    ON sessions (refresh_token_hash)
    WHERE revoked_at IS NULL;

CREATE INDEX idx_sessions_active_user_id_created_at
    ON sessions (user_id, created_at DESC)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_sessions_active_user_id_created_at;
DROP INDEX IF EXISTS uq_sessions_active_refresh_token_hash;
DROP TABLE IF EXISTS sessions;

DROP INDEX IF EXISTS uq_users_normalized_email;
DROP TABLE IF EXISTS users;
