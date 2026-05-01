-- +goose Up
CREATE UNIQUE INDEX ux_accounts_id_user
    ON accounts (id, user_id);

CREATE UNIQUE INDEX ux_categories_id_user
    ON categories (id, user_id);

CREATE UNIQUE INDEX ux_subcategories_id_category_user
    ON subcategories (id, category_id, user_id);

CREATE TABLE transactions (
                              id UUID PRIMARY KEY,
                              user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
                              type TEXT NOT NULL,
                              status TEXT NOT NULL,
                              amount_minor BIGINT NOT NULL,
	                              currency CHAR(3) NOT NULL,
	                              occurred_at TIMESTAMPTZ,
	                              planned_at TIMESTAMPTZ,
	                              posted_at TIMESTAMPTZ,
	                              cancelled_at TIMESTAMPTZ,
	                              account_from_id UUID,
                              account_to_id UUID,
                              category_id UUID,
                              subcategory_id UUID,
                              budget_member_id UUID,
                              income_source_id UUID,
                              debt_id UUID,
                              goal_id UUID,
                              investment_id UUID,
                              recurring_payment_id UUID,
                              comment TEXT,
                              created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                              updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                              CONSTRAINT transactions_type_check CHECK (type IN ('income', 'expense', 'transfer', 'investment', 'saving')),
                              CONSTRAINT transactions_status_check CHECK (status IN ('planned', 'posted', 'cancelled')),
                              CONSTRAINT transactions_currency_check CHECK (currency IN ('RUB', 'USD', 'EUR')),
                              CONSTRAINT transactions_amount_non_negative CHECK (amount_minor >= 0),
                              CONSTRAINT transactions_subcategory_requires_category CHECK (subcategory_id IS NULL OR category_id IS NOT NULL),
                              CONSTRAINT transactions_accounts_must_differ CHECK (account_from_id IS NULL OR account_to_id IS NULL OR account_from_id <> account_to_id),
                              CONSTRAINT fk_transactions_account_from_owner FOREIGN KEY (account_from_id, user_id)
                                  REFERENCES accounts (id, user_id),
                              CONSTRAINT fk_transactions_account_to_owner FOREIGN KEY (account_to_id, user_id)
                                  REFERENCES accounts (id, user_id),
                              CONSTRAINT fk_transactions_category_owner FOREIGN KEY (category_id, user_id)
                                  REFERENCES categories (id, user_id),
                              CONSTRAINT fk_transactions_subcategory_owner_and_category FOREIGN KEY (subcategory_id, category_id, user_id)
                                  REFERENCES subcategories (id, category_id, user_id)
);

CREATE INDEX idx_transactions_user_id
    ON transactions (user_id);

CREATE INDEX idx_transactions_user_occurred_at
    ON transactions (user_id, occurred_at DESC);

CREATE INDEX idx_transactions_user_planned_at
    ON transactions (user_id, planned_at DESC);

CREATE INDEX idx_transactions_user_effective_date
    ON transactions (user_id, COALESCE(occurred_at, planned_at) DESC);

CREATE INDEX idx_transactions_user_effective_month
    ON transactions (
        user_id,
        date_trunc('month', COALESCE(occurred_at, planned_at) AT TIME ZONE 'UTC')
    );

CREATE INDEX idx_transactions_user_account_from
    ON transactions (user_id, account_from_id, COALESCE(occurred_at, planned_at) DESC);

CREATE INDEX idx_transactions_user_account_to
    ON transactions (user_id, account_to_id, COALESCE(occurred_at, planned_at) DESC);

CREATE INDEX idx_transactions_user_category
    ON transactions (user_id, category_id, COALESCE(occurred_at, planned_at) DESC);

CREATE INDEX idx_transactions_user_type_status
    ON transactions (user_id, type, status, COALESCE(occurred_at, planned_at) DESC);

CREATE INDEX idx_transactions_user_comment_search
    ON transactions (user_id, lower(comment))
    WHERE comment IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_user_comment_search;
DROP INDEX IF EXISTS idx_transactions_user_type_status;
DROP INDEX IF EXISTS idx_transactions_user_category;
DROP INDEX IF EXISTS idx_transactions_user_account_to;
DROP INDEX IF EXISTS idx_transactions_user_account_from;
DROP INDEX IF EXISTS idx_transactions_user_effective_month;
DROP INDEX IF EXISTS idx_transactions_user_effective_date;
DROP INDEX IF EXISTS idx_transactions_user_planned_at;
DROP INDEX IF EXISTS idx_transactions_user_occurred_at;
DROP INDEX IF EXISTS idx_transactions_user_id;
DROP TABLE IF EXISTS transactions;

DROP INDEX IF EXISTS ux_subcategories_id_category_user;
DROP INDEX IF EXISTS ux_categories_id_user;
DROP INDEX IF EXISTS ux_accounts_id_user;
