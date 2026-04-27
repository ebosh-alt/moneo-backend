package identity

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var (
	ErrOneTimeTokenIDRequired      = errors.New("one-time token id is required")
	ErrOneTimeTokenUserIDRequired  = errors.New("one-time token user id is required")
	ErrOneTimeTokenPurposeRequired = errors.New("one-time token purpose is required")
	ErrOneTimeTokenHashRequired    = errors.New("one-time token hash is required")
	ErrOneTimeTokenExpiresAtPast   = errors.New("one-time token expires_at must be in the future")
)

type OneTimeTokenPurpose string

const (
	OneTimeTokenPurposePasswordReset     OneTimeTokenPurpose = "password_reset"
	OneTimeTokenPurposeEmailVerification OneTimeTokenPurpose = "email_verification"
)

type OneTimeToken struct {
	ID        shared.OneTimeTokenID
	UserID    shared.UserID
	Purpose   OneTimeTokenPurpose
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func NewOneTimeToken(
	id shared.OneTimeTokenID,
	userID shared.UserID,
	purpose OneTimeTokenPurpose,
	tokenHash string,
	now time.Time,
	expiresAt time.Time,
) (OneTimeToken, error) {
	if strings.TrimSpace(string(id)) == "" {
		return OneTimeToken{}, ErrOneTimeTokenIDRequired
	}
	if strings.TrimSpace(string(userID)) == "" {
		return OneTimeToken{}, ErrOneTimeTokenUserIDRequired
	}
	if strings.TrimSpace(string(purpose)) == "" {
		return OneTimeToken{}, ErrOneTimeTokenPurposeRequired
	}
	if !isKnownOneTimeTokenPurpose(purpose) {
		return OneTimeToken{}, ErrOneTimeTokenPurposeRequired
	}
	if strings.TrimSpace(tokenHash) == "" {
		return OneTimeToken{}, ErrOneTimeTokenHashRequired
	}
	if !expiresAt.After(now) {
		return OneTimeToken{}, ErrOneTimeTokenExpiresAtPast
	}

	return OneTimeToken{
		ID:        id,
		UserID:    userID,
		Purpose:   purpose,
		TokenHash: tokenHash,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		UsedAt:    nil,
	}, nil
}

func isKnownOneTimeTokenPurpose(purpose OneTimeTokenPurpose) bool {
	switch purpose {
	case OneTimeTokenPurposePasswordReset, OneTimeTokenPurposeEmailVerification:
		return true
	default:
		return false
	}
}
