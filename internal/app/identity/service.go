package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
)

var (
	ErrDuplicateEmail         = errors.New("duplicate email")
	ErrUserNotFound           = errors.New("user not found")
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrInvalidCredentials     = errors.New("invalid credentials")
)

type UserRepository interface {
	Create(ctx context.Context, user domainidentity.User) error
	FindByNormalizedEmail(ctx context.Context, normalizedEmail string) (domainidentity.User, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password string, encodedHash string) (bool, error)
}

type UserIDGenerator interface {
	NewUserID() shared.UserID
}

type Clock interface {
	Now() time.Time
}

type AuthService struct {
	users  UserRepository
	hasher PasswordHasher
	idgen  UserIDGenerator
	clock  Clock
}

type RegisterInput struct {
	Email           string
	Password        string
	PasswordConfirm string
}

type LoginInput struct {
	Email    string
	Password string
}

func NewAuthService(
	users UserRepository,
	hasher PasswordHasher,
	idgen UserIDGenerator,
	clock Clock,
) *AuthService {
	return &AuthService{
		users:  users,
		hasher: hasher,
		idgen:  idgen,
		clock:  clock,
	}
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (domainidentity.User, error) {
	credentials := domainidentity.RegisterCredentials{
		Email:           input.Email,
		Password:        input.Password,
		PasswordConfirm: input.PasswordConfirm,
	}

	normalizedEmail, err := credentials.Validate()
	if err != nil {
		return domainidentity.User{}, err
	}

	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return domainidentity.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := domainidentity.NewUser(
		s.idgen.NewUserID(),
		strings.TrimSpace(input.Email),
		normalizedEmail,
		passwordHash,
		s.clock.Now(),
	)
	if err != nil {
		return domainidentity.User{}, fmt.Errorf("build user: %w", err)
	}

	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			return domainidentity.User{}, ErrEmailAlreadyRegistered
		}

		return domainidentity.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (domainidentity.User, error) {
	credentials := domainidentity.LoginCredentials{
		Email:    input.Email,
		Password: input.Password,
	}

	normalizedEmail, err := credentials.Validate()
	if err != nil {
		return domainidentity.User{}, ErrInvalidCredentials
	}

	user, err := s.users.FindByNormalizedEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return domainidentity.User{}, ErrInvalidCredentials
		}
		return domainidentity.User{}, fmt.Errorf("find user by normalized email: %w", err)
	}

	ok, err := s.hasher.Verify(input.Password, user.PasswordHash)
	if err != nil {
		return domainidentity.User{}, ErrInvalidCredentials
	}
	if !ok {
		return domainidentity.User{}, ErrInvalidCredentials
	}

	return user, nil
}
