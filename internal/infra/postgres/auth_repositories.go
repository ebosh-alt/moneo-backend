package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appidentity "moneo/internal/app/identity"
	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthUserRepository struct {
	pool *pgxpool.Pool
}

func NewAuthUserRepository(pool *pgxpool.Pool) *AuthUserRepository {
	return &AuthUserRepository{pool: pool}
}

func (r *AuthUserRepository) Create(ctx context.Context, user domainidentity.User) error {
	const query = `
INSERT INTO users (
	id,
	email,
	password_hash,
	email_verified,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6)
`

	if _, err := r.pool.Exec(
		ctx,
		query,
		string(user.ID),
		user.Email,
		user.PasswordHash,
		user.EmailVerified,
		user.CreatedAt,
		user.UpdatedAt,
	); err != nil {
		if isUniqueViolation(err, "uq_users_normalized_email") {
			return appidentity.ErrDuplicateEmail
		}

		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

func (r *AuthUserRepository) FindByNormalizedEmail(ctx context.Context, normalizedEmail string) (domainidentity.User, error) {
	const query = `
SELECT
	id::text,
	email,
	normalized_email,
	password_hash,
	email_verified,
	created_at,
	updated_at
FROM users
WHERE normalized_email = $1
LIMIT 1
`

	var (
		id               string
		email            string
		storedNormalized string
		passwordHash     string
		emailVerified    bool
		createdAt        time.Time
		updatedAt        time.Time
	)

	if err := r.pool.QueryRow(ctx, query, normalizedEmail).Scan(
		&id,
		&email,
		&storedNormalized,
		&passwordHash,
		&emailVerified,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainidentity.User{}, appidentity.ErrUserNotFound
		}

		return domainidentity.User{}, fmt.Errorf("select user by normalized email: %w", err)
	}

	return domainidentity.User{
		ID:              shared.UserID(id),
		Email:           email,
		NormalizedEmail: storedNormalized,
		PasswordHash:    passwordHash,
		EmailVerified:   emailVerified,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func (r *AuthUserRepository) FindByID(ctx context.Context, userID shared.UserID) (domainidentity.User, error) {
	const query = `
SELECT
	id::text,
	email,
	normalized_email,
	password_hash,
	email_verified,
	created_at,
	updated_at
FROM users
WHERE id = $1
LIMIT 1
`

	var (
		id               string
		email            string
		storedNormalized string
		passwordHash     string
		emailVerified    bool
		createdAt        time.Time
		updatedAt        time.Time
	)

	if err := r.pool.QueryRow(ctx, query, string(userID)).Scan(
		&id,
		&email,
		&storedNormalized,
		&passwordHash,
		&emailVerified,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainidentity.User{}, appidentity.ErrUserNotFound
		}

		return domainidentity.User{}, fmt.Errorf("select user by id: %w", err)
	}

	return domainidentity.User{
		ID:              shared.UserID(id),
		Email:           email,
		NormalizedEmail: storedNormalized,
		PasswordHash:    passwordHash,
		EmailVerified:   emailVerified,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

type AuthSessionRepository struct {
	pool *pgxpool.Pool
}

func NewAuthSessionRepository(pool *pgxpool.Pool) *AuthSessionRepository {
	return &AuthSessionRepository{pool: pool}
}

func (r *AuthSessionRepository) Create(ctx context.Context, session domainidentity.Session) error {
	const query = `
INSERT INTO sessions (
	id,
	user_id,
	refresh_token_hash,
	created_at,
	expires_at,
	revoked_at
)
VALUES ($1, $2, $3, $4, $5, $6)
`

	if _, err := r.pool.Exec(
		ctx,
		query,
		string(session.ID),
		string(session.UserID),
		session.RefreshTokenHash,
		session.CreatedAt,
		session.ExpiresAt,
		session.RevokedAt,
	); err != nil {
		if isUniqueViolation(err, "uq_sessions_active_refresh_token_hash") {
			return appidentity.ErrDuplicateSessionRefreshToken
		}

		return fmt.Errorf("insert session: %w", err)
	}

	return nil
}

func (r *AuthSessionRepository) FindByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (domainidentity.Session, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	refresh_token_hash,
	created_at,
	last_used_at,
	expires_at,
	revoked_at
FROM sessions
WHERE refresh_token_hash = $1
LIMIT 1
`

	var (
		id         string
		userID     string
		hash       string
		createdAt  time.Time
		lastUsedAt *time.Time
		expiresAt  time.Time
		revokedAt  *time.Time
	)

	if err := r.pool.QueryRow(ctx, query, refreshTokenHash).Scan(
		&id,
		&userID,
		&hash,
		&createdAt,
		&lastUsedAt,
		&expiresAt,
		&revokedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainidentity.Session{}, appidentity.ErrSessionNotFound
		}

		return domainidentity.Session{}, fmt.Errorf("select session by refresh token hash: %w", err)
	}

	return domainidentity.Session{
		ID:               shared.SessionID(id),
		UserID:           shared.UserID(userID),
		RefreshTokenHash: hash,
		CreatedAt:        createdAt,
		LastUsedAt:       lastUsedAt,
		ExpiresAt:        expiresAt,
		RevokedAt:        revokedAt,
	}, nil
}

func (r *AuthSessionRepository) FindByID(ctx context.Context, sessionID shared.SessionID) (domainidentity.Session, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	refresh_token_hash,
	created_at,
	last_used_at,
	expires_at,
	revoked_at
FROM sessions
WHERE id = $1
LIMIT 1
`

	var (
		id         string
		userID     string
		hash       string
		createdAt  time.Time
		lastUsedAt *time.Time
		expiresAt  time.Time
		revokedAt  *time.Time
	)

	if err := r.pool.QueryRow(ctx, query, string(sessionID)).Scan(
		&id,
		&userID,
		&hash,
		&createdAt,
		&lastUsedAt,
		&expiresAt,
		&revokedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainidentity.Session{}, appidentity.ErrSessionNotFound
		}

		return domainidentity.Session{}, fmt.Errorf("select session by id: %w", err)
	}

	return domainidentity.Session{
		ID:               shared.SessionID(id),
		UserID:           shared.UserID(userID),
		RefreshTokenHash: hash,
		CreatedAt:        createdAt,
		LastUsedAt:       lastUsedAt,
		ExpiresAt:        expiresAt,
		RevokedAt:        revokedAt,
	}, nil
}

func (r *AuthSessionRepository) TouchLastUsedAt(ctx context.Context, sessionID shared.SessionID, lastUsedAt time.Time) error {
	const query = `
UPDATE sessions
SET last_used_at = $2
WHERE id = $1
`

	commandTag, err := r.pool.Exec(ctx, query, string(sessionID), lastUsedAt)
	if err != nil {
		return fmt.Errorf("update session last_used_at: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appidentity.ErrSessionNotFound
	}

	return nil
}

func (r *AuthSessionRepository) RevokeByID(ctx context.Context, sessionID shared.SessionID, revokedAt time.Time) error {
	const query = `
UPDATE sessions
SET revoked_at = $2
WHERE id = $1
  AND revoked_at IS NULL
`

	commandTag, err := r.pool.Exec(ctx, query, string(sessionID), revokedAt)
	if err != nil {
		return fmt.Errorf("revoke session by id: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appidentity.ErrSessionNotFound
	}

	return nil
}

func (r *AuthSessionRepository) RevokeAllByUserID(ctx context.Context, userID shared.UserID, revokedAt time.Time) error {
	const query = `
UPDATE sessions
SET revoked_at = $2
WHERE user_id = $1
  AND revoked_at IS NULL
  AND expires_at > $2
`

	if _, err := r.pool.Exec(ctx, query, string(userID), revokedAt); err != nil {
		return fmt.Errorf("revoke all sessions by user id: %w", err)
	}

	return nil
}

func isUniqueViolation(err error, indexName string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if strings.TrimSpace(pgErr.ConstraintName) != strings.TrimSpace(indexName) {
		return false
	}

	return true
}
