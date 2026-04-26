package identity

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"
)

const minPasswordLength = 8

var (
	ErrInvalidEmail            = errors.New("invalid email")
	ErrInvalidPassword         = errors.New("invalid password")
	ErrPasswordConfirmMismatch = errors.New("password confirm mismatch")
)

type RegisterCredentials struct {
	Email           string
	Password        string
	PasswordConfirm string
}

func (c RegisterCredentials) Validate() (string, error) {
	normalizedEmail, err := NormalizeEmail(c.Email)
	if err != nil {
		return "", err
	}

	if err := ValidatePassword(c.Password); err != nil {
		return "", err
	}

	if c.Password != c.PasswordConfirm {
		return "", ErrPasswordConfirmMismatch
	}

	return normalizedEmail, nil
}

type LoginCredentials struct {
	Email    string
	Password string
}

func (c LoginCredentials) Validate() (string, error) {
	normalizedEmail, err := NormalizeEmail(c.Email)
	if err != nil {
		return "", err
	}

	if err := ValidatePassword(c.Password); err != nil {
		return "", err
	}

	return normalizedEmail, nil
}

func NormalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if !isValidEmail(trimmed) {
		return "", ErrInvalidEmail
	}

	return strings.ToLower(trimmed), nil
}

func ValidatePassword(password string) error {
	if len(password) < minPasswordLength {
		return ErrInvalidPassword
	}

	var hasLetter bool
	var hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}

	if !hasLetter || !hasDigit {
		return ErrInvalidPassword
	}

	return nil
}

func isValidEmail(email string) bool {
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return false
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return false
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	localPart := parts[0]
	domainPart := parts[1]
	if localPart == "" || domainPart == "" || !strings.Contains(domainPart, ".") {
		return false
	}

	return true
}
