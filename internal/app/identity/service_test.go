package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
	"moneo/internal/infra/security"
)

func TestAuthServiceRegisterAndLoginWithNormalizedEmail(t *testing.T) {
	const rawPassword = "StrongPassw0rd!"

	repo := newInMemoryUserRepo()
	hasher := security.NewArgon2IDHasher(security.DefaultArgon2IDConfig())
	service := NewAuthService(
		repo,
		hasher,
		staticIDGenerator{next: shared.UserID("user-1")},
		staticClock{now: time.Date(2026, 4, 26, 16, 10, 0, 0, time.UTC)},
	)

	registered, err := service.Register(context.Background(), RegisterInput{
		Email:           "  USER@example.COM ",
		Password:        rawPassword,
		PasswordConfirm: rawPassword,
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	if registered.NormalizedEmail != "user@example.com" {
		t.Fatalf("expected normalized email user@example.com, got %q", registered.NormalizedEmail)
	}

	stored, err := repo.FindByNormalizedEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("cannot fetch stored user: %v", err)
	}

	if stored.PasswordHash == rawPassword {
		t.Fatal("password must not be stored in raw form")
	}
	if strings.Contains(stored.PasswordHash, rawPassword) {
		t.Fatal("password hash must not contain raw password")
	}

	_, err = service.Login(context.Background(), LoginInput{
		Email:    "user@EXAMPLE.com",
		Password: rawPassword,
	})
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
}

func TestAuthServiceLoginRejectsWrongPassword(t *testing.T) {
	repo := newInMemoryUserRepo()
	hasher := security.NewArgon2IDHasher(security.DefaultArgon2IDConfig())
	service := NewAuthService(
		repo,
		hasher,
		staticIDGenerator{next: shared.UserID("user-1")},
		staticClock{now: time.Date(2026, 4, 26, 16, 10, 0, 0, time.UTC)},
	)

	_, err := service.Register(context.Background(), RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	_, err = service.Login(context.Background(), LoginInput{
		Email:    "user@example.com",
		Password: "WrongPassw0rd!",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthServiceRegisterRejectsDuplicateEmail(t *testing.T) {
	repo := newInMemoryUserRepo()
	hasher := security.NewArgon2IDHasher(security.DefaultArgon2IDConfig())
	service := NewAuthService(
		repo,
		hasher,
		staticIDGenerator{next: shared.UserID("user-1")},
		staticClock{now: time.Date(2026, 4, 26, 16, 10, 0, 0, time.UTC)},
	)

	_, err := service.Register(context.Background(), RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("first register returned error: %v", err)
	}

	_, err = service.Register(context.Background(), RegisterInput{
		Email:           "USER@example.com",
		Password:        "AnotherStrongPassw0rd!",
		PasswordConfirm: "AnotherStrongPassw0rd!",
	})
	if !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("expected ErrEmailAlreadyRegistered, got %v", err)
	}
}

type inMemoryUserRepo struct {
	byNormalizedEmail map[string]domainidentity.User
}

func newInMemoryUserRepo() *inMemoryUserRepo {
	return &inMemoryUserRepo{
		byNormalizedEmail: make(map[string]domainidentity.User),
	}
}

func (r *inMemoryUserRepo) Create(_ context.Context, user domainidentity.User) error {
	if _, exists := r.byNormalizedEmail[user.NormalizedEmail]; exists {
		return ErrDuplicateEmail
	}

	r.byNormalizedEmail[user.NormalizedEmail] = user
	return nil
}

func (r *inMemoryUserRepo) FindByNormalizedEmail(_ context.Context, normalizedEmail string) (domainidentity.User, error) {
	user, ok := r.byNormalizedEmail[normalizedEmail]
	if !ok {
		return domainidentity.User{}, ErrUserNotFound
	}

	return user, nil
}

type staticIDGenerator struct {
	next shared.UserID
}

func (s staticIDGenerator) NewUserID() shared.UserID {
	return s.next
}

type staticClock struct {
	now time.Time
}

func (s staticClock) Now() time.Time {
	return s.now
}
