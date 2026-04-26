package identity

import (
	"context"
	"errors"
	"fmt"

	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
)

type AccessTokenVerifier interface {
	VerifyAccessTokenIdentity(accessToken string) (shared.UserID, shared.SessionID, error)
}

type AccessUserRepository interface {
	FindByID(ctx context.Context, userID shared.UserID) (domainidentity.User, error)
}

type AccessSessionRepository interface {
	FindByID(ctx context.Context, sessionID shared.SessionID) (domainidentity.Session, error)
}

type AccessAuthService struct {
	tokens   AccessTokenVerifier
	users    AccessUserRepository
	sessions AccessSessionRepository
}

func NewAccessAuthService(
	tokens AccessTokenVerifier,
	users AccessUserRepository,
	sessions AccessSessionRepository,
) *AccessAuthService {
	return &AccessAuthService{
		tokens:   tokens,
		users:    users,
		sessions: sessions,
	}
}

func (s *AccessAuthService) Authenticate(ctx context.Context, accessToken string) (domainidentity.User, domainidentity.Session, error) {
	userID, sessionID, err := s.tokens.VerifyAccessTokenIdentity(accessToken)
	if err != nil {
		return domainidentity.User{}, domainidentity.Session{}, ErrInvalidAccessToken
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return domainidentity.User{}, domainidentity.Session{}, ErrInvalidAccessToken
		}
		return domainidentity.User{}, domainidentity.Session{}, fmt.Errorf("find user by id: %w", err)
	}

	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return domainidentity.User{}, domainidentity.Session{}, ErrInvalidAccessToken
		}
		return domainidentity.User{}, domainidentity.Session{}, fmt.Errorf("find session by id: %w", err)
	}

	if session.UserID != user.ID {
		return domainidentity.User{}, domainidentity.Session{}, ErrInvalidAccessToken
	}
	if session.RevokedAt != nil {
		return domainidentity.User{}, domainidentity.Session{}, ErrInvalidAccessToken
	}

	return user, session, nil
}
