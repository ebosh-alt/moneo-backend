package identity

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var (
	ErrSessionIDRequired         = errors.New("session id required")
	ErrSessionUserIDRequired     = errors.New("session user id required")
	ErrSessionRefreshHashMissing = errors.New("session refresh token hash required")
	ErrSessionExpiresAtInvalid   = errors.New("session expires_at must be in the future")
)

type Session struct {
	ID               shared.SessionID
	UserID           shared.UserID
	RefreshTokenHash string
	UserAgent        *string
	IP               *string
	DeviceName       *string
	CreatedAt        time.Time
	LastUsedAt       *time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

func NewSession(
	id shared.SessionID,
	userID shared.UserID,
	refreshTokenHash string,
	now time.Time,
	expiresAt time.Time,
) (Session, error) {
	if strings.TrimSpace(string(id)) == "" {
		return Session{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(string(userID)) == "" {
		return Session{}, ErrSessionUserIDRequired
	}
	if strings.TrimSpace(refreshTokenHash) == "" {
		return Session{}, ErrSessionRefreshHashMissing
	}
	if !expiresAt.After(now) {
		return Session{}, ErrSessionExpiresAtInvalid
	}

	return Session{
		ID:               id,
		UserID:           userID,
		RefreshTokenHash: refreshTokenHash,
		UserAgent:        nil,
		IP:               nil,
		DeviceName:       nil,
		CreatedAt:        now,
		LastUsedAt:       nil,
		ExpiresAt:        expiresAt,
		RevokedAt:        nil,
	}, nil
}
