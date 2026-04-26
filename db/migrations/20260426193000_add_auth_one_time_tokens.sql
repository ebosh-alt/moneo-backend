-- +goose Up
CREATE TABLE auth_one_time_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('password_reset', 'email_verification')),
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);

CREATE INDEX idx_auth_one_time_tokens_active_lookup
    ON auth_one_time_tokens (purpose, token_hash)
    WHERE used_at IS NULL;

CREATE INDEX idx_auth_one_time_tokens_user_purpose
    ON auth_one_time_tokens (user_id, purpose, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_auth_one_time_tokens_user_purpose;
DROP INDEX IF EXISTS idx_auth_one_time_tokens_active_lookup;
DROP TABLE IF EXISTS auth_one_time_tokens;
