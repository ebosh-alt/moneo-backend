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

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(
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

	db := databaseFromContext(ctx, r.pool)
	if err := db.QueryRow(ctx, query, normalizedEmail).Scan(
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

	db := databaseFromContext(ctx, r.pool)
	if err := db.QueryRow(ctx, query, string(userID)).Scan(
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

func (r *AuthUserRepository) UpdatePassword(ctx context.Context, userID shared.UserID, passwordHash string, updatedAt time.Time) error {
	const query = `
UPDATE users
SET password_hash = $2,
    updated_at = $3
WHERE id = $1
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(userID), passwordHash, updatedAt)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appidentity.ErrUserNotFound
	}

	return nil
}

func (r *AuthUserRepository) MarkEmailVerified(ctx context.Context, userID shared.UserID, updatedAt time.Time) error {
	const query = `
UPDATE users
SET email_verified = TRUE,
    updated_at = $2
WHERE id = $1
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(userID), updatedAt)
	if err != nil {
		return fmt.Errorf("mark user email verified: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appidentity.ErrUserNotFound
	}

	return nil
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
	user_agent,
	ip,
	device_name,
	created_at,
	expires_at,
	revoked_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(
		ctx,
		query,
		string(session.ID),
		string(session.UserID),
		session.RefreshTokenHash,
		session.UserAgent,
		session.IP,
		session.DeviceName,
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
	user_agent,
	ip::text,
	device_name,
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
		userAgent  *string
		ip         *string
		deviceName *string
		createdAt  time.Time
		lastUsedAt *time.Time
		expiresAt  time.Time
		revokedAt  *time.Time
	)

	db := databaseFromContext(ctx, r.pool)
	if err := db.QueryRow(ctx, query, refreshTokenHash).Scan(
		&id,
		&userID,
		&hash,
		&userAgent,
		&ip,
		&deviceName,
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
		UserAgent:        userAgent,
		IP:               ip,
		DeviceName:       deviceName,
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
	user_agent,
	ip::text,
	device_name,
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
		userAgent  *string
		ip         *string
		deviceName *string
		createdAt  time.Time
		lastUsedAt *time.Time
		expiresAt  time.Time
		revokedAt  *time.Time
	)

	db := databaseFromContext(ctx, r.pool)
	if err := db.QueryRow(ctx, query, string(sessionID)).Scan(
		&id,
		&userID,
		&hash,
		&userAgent,
		&ip,
		&deviceName,
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
		UserAgent:        userAgent,
		IP:               ip,
		DeviceName:       deviceName,
		CreatedAt:        createdAt,
		LastUsedAt:       lastUsedAt,
		ExpiresAt:        expiresAt,
		RevokedAt:        revokedAt,
	}, nil
}

func (r *AuthSessionRepository) ListActiveByUserID(ctx context.Context, userID shared.UserID, now time.Time) ([]domainidentity.Session, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	refresh_token_hash,
	user_agent,
	ip::text,
	device_name,
	created_at,
	last_used_at,
	expires_at,
	revoked_at
FROM sessions
WHERE user_id = $1
  AND revoked_at IS NULL
  AND expires_at > $2
ORDER BY created_at DESC
`

	db := databaseFromContext(ctx, r.pool)
	rows, err := db.Query(ctx, query, string(userID), now)
	if err != nil {
		return nil, fmt.Errorf("select active sessions by user id: %w", err)
	}
	defer rows.Close()

	result := make([]domainidentity.Session, 0, 4)
	for rows.Next() {
		var (
			id            string
			scannedUserID string
			hash          string
			userAgent     *string
			ip            *string
			deviceName    *string
			createdAt     time.Time
			lastUsedAt    *time.Time
			expiresAt     time.Time
			revokedAt     *time.Time
		)

		if err := rows.Scan(
			&id,
			&scannedUserID,
			&hash,
			&userAgent,
			&ip,
			&deviceName,
			&createdAt,
			&lastUsedAt,
			&expiresAt,
			&revokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan active session: %w", err)
		}

		result = append(result, domainidentity.Session{
			ID:               shared.SessionID(id),
			UserID:           shared.UserID(scannedUserID),
			RefreshTokenHash: hash,
			UserAgent:        userAgent,
			IP:               ip,
			DeviceName:       deviceName,
			CreatedAt:        createdAt,
			LastUsedAt:       lastUsedAt,
			ExpiresAt:        expiresAt,
			RevokedAt:        revokedAt,
		})
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate active sessions rows: %w", rows.Err())
	}

	return result, nil
}

func (r *AuthSessionRepository) TouchLastUsedAt(ctx context.Context, sessionID shared.SessionID, lastUsedAt time.Time) error {
	const query = `
UPDATE sessions
SET last_used_at = $2
WHERE id = $1
  AND revoked_at IS NULL
  AND expires_at > $2
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(sessionID), lastUsedAt)
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

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(sessionID), revokedAt)
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

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(ctx, query, string(userID), revokedAt); err != nil {
		return fmt.Errorf("revoke all sessions by user id: %w", err)
	}

	return nil
}

type AuthOneTimeTokenRepository struct {
	pool *pgxpool.Pool
}

func NewAuthOneTimeTokenRepository(pool *pgxpool.Pool) *AuthOneTimeTokenRepository {
	return &AuthOneTimeTokenRepository{pool: pool}
}

func (r *AuthOneTimeTokenRepository) Create(ctx context.Context, token domainidentity.OneTimeToken) error {
	const query = `
INSERT INTO auth_one_time_tokens (
	id,
	user_id,
	purpose,
	token_hash,
	created_at,
	expires_at,
	used_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`

	db := databaseFromContext(ctx, r.pool)
	if _, err := db.Exec(
		ctx,
		query,
		string(token.ID),
		string(token.UserID),
		string(token.Purpose),
		token.TokenHash,
		token.CreatedAt,
		token.ExpiresAt,
		token.UsedAt,
	); err != nil {
		return fmt.Errorf("insert one-time token: %w", err)
	}

	return nil
}

func (r *AuthOneTimeTokenRepository) FindActiveByHash(
	ctx context.Context,
	purpose domainidentity.OneTimeTokenPurpose,
	tokenHash string,
	now time.Time,
) (domainidentity.OneTimeToken, error) {
	const query = `
SELECT
	id::text,
	user_id::text,
	purpose,
	token_hash,
	created_at,
	expires_at,
	used_at
FROM auth_one_time_tokens
WHERE purpose = $1
  AND token_hash = $2
  AND used_at IS NULL
  AND expires_at > $3
LIMIT 1
`

	var (
		id         string
		userID     string
		rawPurpose string
		hash       string
		createdAt  time.Time
		expiresAt  time.Time
		usedAt     *time.Time
	)

	db := databaseFromContext(ctx, r.pool)
	if err := db.QueryRow(ctx, query, string(purpose), tokenHash, now).Scan(
		&id,
		&userID,
		&rawPurpose,
		&hash,
		&createdAt,
		&expiresAt,
		&usedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainidentity.OneTimeToken{}, appidentity.ErrOneTimeTokenNotFound
		}

		return domainidentity.OneTimeToken{}, fmt.Errorf("select active one-time token by hash: %w", err)
	}

	parsedPurpose, err := parseOneTimeTokenPurpose(rawPurpose)
	if err != nil {
		return domainidentity.OneTimeToken{}, err
	}

	return domainidentity.OneTimeToken{
		ID:        shared.OneTimeTokenID(id),
		UserID:    shared.UserID(userID),
		Purpose:   parsedPurpose,
		TokenHash: hash,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		UsedAt:    usedAt,
	}, nil
}

func (r *AuthOneTimeTokenRepository) MarkUsed(ctx context.Context, tokenID shared.OneTimeTokenID, usedAt time.Time) error {
	const query = `
UPDATE auth_one_time_tokens
SET used_at = $2
WHERE id = $1
  AND used_at IS NULL
  AND expires_at > $2
`

	db := databaseFromContext(ctx, r.pool)
	commandTag, err := db.Exec(ctx, query, string(tokenID), usedAt)
	if err != nil {
		return fmt.Errorf("mark one-time token used: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return appidentity.ErrOneTimeTokenNotFound
	}

	return nil
}

func parseOneTimeTokenPurpose(raw string) (domainidentity.OneTimeTokenPurpose, error) {
	switch strings.TrimSpace(raw) {
	case string(domainidentity.OneTimeTokenPurposePasswordReset):
		return domainidentity.OneTimeTokenPurposePasswordReset, nil
	case string(domainidentity.OneTimeTokenPurposeEmailVerification):
		return domainidentity.OneTimeTokenPurposeEmailVerification, nil
	default:
		return "", fmt.Errorf("unknown one-time token purpose %q", raw)
	}
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
