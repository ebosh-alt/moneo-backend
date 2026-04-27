package identity

import (
	"errors"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

var ErrPasswordHashRequired = errors.New("password hash required")

type User struct {
	ID              shared.UserID
	Email           string
	NormalizedEmail string
	PasswordHash    string
	EmailVerified   bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewUser(
	id shared.UserID,
	email string,
	normalizedEmail string,
	passwordHash string,
	now time.Time,
) (User, error) {
	if strings.TrimSpace(passwordHash) == "" {
		return User{}, ErrPasswordHashRequired
	}

	return User{
		ID:              id,
		Email:           strings.TrimSpace(email),
		NormalizedEmail: normalizedEmail,
		PasswordHash:    passwordHash,
		EmailVerified:   false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}
