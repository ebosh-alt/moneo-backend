-- +goose Up
CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    balance_minor BIGINT NOT NULL DEFAULT 0,
    initial_balance_minor BIGINT NOT NULL DEFAULT 0,
    include_in_net_worth BOOLEAN NOT NULL DEFAULT TRUE,
    include_in_daily_budget BOOLEAN NOT NULL DEFAULT TRUE,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT accounts_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT accounts_type_check CHECK (
        type IN (
            'cash',
            'debit_card',
            'savings',
            'brokerage',
            'credit_card',
            'deposit',
            'debt',
            'other'
        )
    ),
    CONSTRAINT accounts_currency_check CHECK (currency IN ('RUB', 'USD', 'EUR'))
);

CREATE INDEX idx_accounts_user_id
    ON accounts (user_id);

CREATE INDEX idx_accounts_user_archived
    ON accounts (user_id, archived_at);

CREATE UNIQUE INDEX ux_accounts_user_name_active
    ON accounts (user_id, lower(name))
    WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ux_accounts_user_name_active;
DROP INDEX IF EXISTS idx_accounts_user_archived;
DROP INDEX IF EXISTS idx_accounts_user_id;
DROP TABLE IF EXISTS accounts;
