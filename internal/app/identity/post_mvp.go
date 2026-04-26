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

const (
	minPasswordResetTokenTTL = 15 * time.Minute
	maxPasswordResetTokenTTL = 60 * time.Minute
)

var (
	ErrOneTimeTokenNotFound          = errors.New("one-time token not found")
	ErrInvalidPasswordResetToken     = errors.New("invalid password reset token")
	ErrInvalidEmailVerificationToken = errors.New("invalid email verification token")
	ErrInvalidPasswordResetTokenTTL  = errors.New("invalid password reset token ttl")
	ErrInvalidVerificationTokenTTL   = errors.New("invalid email verification token ttl")
)

type PasswordCredentialUpdater interface {
	UpdatePassword(ctx context.Context, userID shared.UserID, passwordHash string, updatedAt time.Time) error
}

type EmailVerificationUpdater interface {
	MarkEmailVerified(ctx context.Context, userID shared.UserID, updatedAt time.Time) error
}

type PostMVPUserRepository interface {
	FindByID(ctx context.Context, userID shared.UserID) (domainidentity.User, error)
	FindByNormalizedEmail(ctx context.Context, normalizedEmail string) (domainidentity.User, error)
	PasswordCredentialUpdater
	EmailVerificationUpdater
}

type OneTimeTokenRepository interface {
	Create(ctx context.Context, token domainidentity.OneTimeToken) error
	FindActiveByHash(
		ctx context.Context,
		purpose domainidentity.OneTimeTokenPurpose,
		tokenHash string,
		now time.Time,
	) (domainidentity.OneTimeToken, error)
	MarkUsed(ctx context.Context, tokenID shared.OneTimeTokenID, usedAt time.Time) error
}

type OneTimeTokenIssuer interface {
	IssueOneTimeToken(ttl time.Duration) (token string, hash string, expiresAt time.Time, err error)
	HashOneTimeToken(token string) (string, error)
}

type OneTimeTokenIDGenerator interface {
	NewOneTimeTokenID() shared.OneTimeTokenID
}

type PasswordResetNotifier interface {
	SendPasswordReset(ctx context.Context, user domainidentity.User, token string) error
}

type EmailVerificationNotifier interface {
	SendEmailVerification(ctx context.Context, user domainidentity.User, token string) error
}

type AuthPostMVPConfig struct {
	PasswordResetTokenTTL     time.Duration
	EmailVerificationTokenTTL time.Duration
}

func DefaultAuthPostMVPConfig() AuthPostMVPConfig {
	return AuthPostMVPConfig{
		PasswordResetTokenTTL:     30 * time.Minute,
		EmailVerificationTokenTTL: 24 * time.Hour,
	}
}

type ForgotPasswordInput struct {
	Email string
}

type ResetPasswordInput struct {
	Token           string
	Password        string
	PasswordConfirm string
}

type SendVerificationEmailInput struct {
	UserID shared.UserID
}

type VerifyEmailInput struct {
	Token string
}

type AuthPostMVPService struct {
	users                     PostMVPUserRepository
	sessions                  SessionRepository
	tokens                    OneTimeTokenRepository
	tokenIssuer               OneTimeTokenIssuer
	tokenIDs                  OneTimeTokenIDGenerator
	passwordHasher            PasswordHasher
	clock                     Clock
	passwordResetNotifier     PasswordResetNotifier
	emailVerificationNotifier EmailVerificationNotifier
	config                    AuthPostMVPConfig
}

func NewAuthPostMVPService(
	users PostMVPUserRepository,
	sessions SessionRepository,
	tokens OneTimeTokenRepository,
	tokenIssuer OneTimeTokenIssuer,
	tokenIDs OneTimeTokenIDGenerator,
	passwordHasher PasswordHasher,
	clock Clock,
	passwordResetNotifier PasswordResetNotifier,
	emailVerificationNotifier EmailVerificationNotifier,
	config AuthPostMVPConfig,
) (*AuthPostMVPService, error) {
	if config.PasswordResetTokenTTL < minPasswordResetTokenTTL || config.PasswordResetTokenTTL > maxPasswordResetTokenTTL {
		return nil, ErrInvalidPasswordResetTokenTTL
	}
	if config.EmailVerificationTokenTTL <= 0 {
		return nil, ErrInvalidVerificationTokenTTL
	}
	if passwordResetNotifier == nil {
		passwordResetNotifier = noopPasswordResetNotifier{}
	}
	if emailVerificationNotifier == nil {
		emailVerificationNotifier = noopEmailVerificationNotifier{}
	}

	return &AuthPostMVPService{
		users:                     users,
		sessions:                  sessions,
		tokens:                    tokens,
		tokenIssuer:               tokenIssuer,
		tokenIDs:                  tokenIDs,
		passwordHasher:            passwordHasher,
		clock:                     clock,
		passwordResetNotifier:     passwordResetNotifier,
		emailVerificationNotifier: emailVerificationNotifier,
		config:                    config,
	}, nil
}

func (s *AuthPostMVPService) ForgotPassword(ctx context.Context, input ForgotPasswordInput) error {
	normalizedEmail, err := domainidentity.NormalizeEmail(input.Email)
	if err != nil {
		return err
	}

	user, err := s.users.FindByNormalizedEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}

		return fmt.Errorf("find user by normalized email: %w", err)
	}

	return s.createAndSendToken(
		ctx,
		user,
		domainidentity.OneTimeTokenPurposePasswordReset,
		s.config.PasswordResetTokenTTL,
		func(ctx context.Context, user domainidentity.User, token string) error {
			return s.passwordResetNotifier.SendPasswordReset(ctx, user, token)
		},
	)
}

func (s *AuthPostMVPService) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	if strings.TrimSpace(input.Token) == "" {
		return ErrInvalidPasswordResetToken
	}
	if err := domainidentity.ValidatePassword(input.Password); err != nil {
		return err
	}
	if input.Password != input.PasswordConfirm {
		return domainidentity.ErrPasswordConfirmMismatch
	}

	token, err := s.findActiveOneTimeToken(
		ctx,
		domainidentity.OneTimeTokenPurposePasswordReset,
		input.Token,
		ErrInvalidPasswordResetToken,
	)
	if err != nil {
		return err
	}

	now := s.clock.Now().UTC()
	if err := s.tokens.MarkUsed(ctx, token.ID, now); err != nil {
		if errors.Is(err, ErrOneTimeTokenNotFound) {
			return ErrInvalidPasswordResetToken
		}
		return fmt.Errorf("mark password reset token used: %w", err)
	}

	passwordHash, err := s.passwordHasher.Hash(input.Password)
	if err != nil {
		return fmt.Errorf("hash updated password: %w", err)
	}

	if err := s.users.UpdatePassword(ctx, token.UserID, passwordHash, now); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidPasswordResetToken
		}
		return fmt.Errorf("update user password: %w", err)
	}

	if err := s.sessions.RevokeAllByUserID(ctx, token.UserID, now); err != nil {
		return fmt.Errorf("revoke all user sessions after password reset: %w", err)
	}

	return nil
}

func (s *AuthPostMVPService) SendVerificationEmail(ctx context.Context, input SendVerificationEmailInput) error {
	user, err := s.users.FindByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidAccessToken
		}
		return fmt.Errorf("find user by id: %w", err)
	}

	if user.EmailVerified {
		return nil
	}

	return s.createAndSendToken(
		ctx,
		user,
		domainidentity.OneTimeTokenPurposeEmailVerification,
		s.config.EmailVerificationTokenTTL,
		func(ctx context.Context, user domainidentity.User, token string) error {
			return s.emailVerificationNotifier.SendEmailVerification(ctx, user, token)
		},
	)
}

func (s *AuthPostMVPService) VerifyEmail(ctx context.Context, input VerifyEmailInput) error {
	if strings.TrimSpace(input.Token) == "" {
		return ErrInvalidEmailVerificationToken
	}

	token, err := s.findActiveOneTimeToken(
		ctx,
		domainidentity.OneTimeTokenPurposeEmailVerification,
		input.Token,
		ErrInvalidEmailVerificationToken,
	)
	if err != nil {
		return err
	}

	now := s.clock.Now().UTC()
	if err := s.tokens.MarkUsed(ctx, token.ID, now); err != nil {
		if errors.Is(err, ErrOneTimeTokenNotFound) {
			return ErrInvalidEmailVerificationToken
		}
		return fmt.Errorf("mark email verification token used: %w", err)
	}

	if err := s.users.MarkEmailVerified(ctx, token.UserID, now); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidEmailVerificationToken
		}
		return fmt.Errorf("mark user email verified: %w", err)
	}

	return nil
}

func (s *AuthPostMVPService) createAndSendToken(
	ctx context.Context,
	user domainidentity.User,
	purpose domainidentity.OneTimeTokenPurpose,
	ttl time.Duration,
	sendFn func(context.Context, domainidentity.User, string) error,
) error {
	rawToken, tokenHash, expiresAt, err := s.tokenIssuer.IssueOneTimeToken(ttl)
	if err != nil {
		return fmt.Errorf("issue %s token: %w", purpose, err)
	}

	now := s.clock.Now().UTC()
	oneTimeToken, err := domainidentity.NewOneTimeToken(
		s.tokenIDs.NewOneTimeTokenID(),
		user.ID,
		purpose,
		tokenHash,
		now,
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("build %s token: %w", purpose, err)
	}

	if err := s.tokens.Create(ctx, oneTimeToken); err != nil {
		return fmt.Errorf("store %s token: %w", purpose, err)
	}

	if err := sendFn(ctx, user, rawToken); err != nil {
		return fmt.Errorf("send %s token: %w", purpose, err)
	}

	return nil
}

func (s *AuthPostMVPService) findActiveOneTimeToken(
	ctx context.Context,
	purpose domainidentity.OneTimeTokenPurpose,
	rawToken string,
	invalidTokenErr error,
) (domainidentity.OneTimeToken, error) {
	tokenHash, err := s.tokenIssuer.HashOneTimeToken(rawToken)
	if err != nil {
		return domainidentity.OneTimeToken{}, invalidTokenErr
	}

	token, err := s.tokens.FindActiveByHash(ctx, purpose, tokenHash, s.clock.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrOneTimeTokenNotFound) {
			return domainidentity.OneTimeToken{}, invalidTokenErr
		}
		return domainidentity.OneTimeToken{}, fmt.Errorf("find active %s token: %w", purpose, err)
	}

	return token, nil
}

type noopPasswordResetNotifier struct{}

func (noopPasswordResetNotifier) SendPasswordReset(context.Context, domainidentity.User, string) error {
	return nil
}

type noopEmailVerificationNotifier struct{}

func (noopEmailVerificationNotifier) SendEmailVerification(context.Context, domainidentity.User, string) error {
	return nil
}
