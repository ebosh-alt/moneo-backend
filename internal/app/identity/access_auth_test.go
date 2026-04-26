package identity

import (
	"context"
	"testing"
	"time"

	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
)

func TestAccessAuthServiceRejectsExpiredSession(t *testing.T) {
	service := NewAccessAuthService(
		staticAccessTokenVerifier{
			userID:    shared.UserID("user-1"),
			sessionID: shared.SessionID("session-1"),
		},
		staticAccessUserRepo{
			user: domainidentity.User{ID: shared.UserID("user-1")},
		},
		staticAccessSessionRepo{
			session: domainidentity.Session{
				ID:        shared.SessionID("session-1"),
				UserID:    shared.UserID("user-1"),
				ExpiresAt: time.Now().UTC().Add(-time.Second),
			},
		},
	)

	_, _, err := service.Authenticate(context.Background(), "any-token")
	if err != ErrInvalidAccessToken {
		t.Fatalf("expected ErrInvalidAccessToken for expired session, got %v", err)
	}
}

type staticAccessTokenVerifier struct {
	userID    shared.UserID
	sessionID shared.SessionID
	err       error
}

func (v staticAccessTokenVerifier) VerifyAccessTokenIdentity(string) (shared.UserID, shared.SessionID, error) {
	if v.err != nil {
		return "", "", v.err
	}
	return v.userID, v.sessionID, nil
}

type staticAccessUserRepo struct {
	user domainidentity.User
	err  error
}

func (r staticAccessUserRepo) FindByID(context.Context, shared.UserID) (domainidentity.User, error) {
	if r.err != nil {
		return domainidentity.User{}, r.err
	}
	return r.user, nil
}

type staticAccessSessionRepo struct {
	session domainidentity.Session
	err     error
}

func (r staticAccessSessionRepo) FindByID(context.Context, shared.SessionID) (domainidentity.Session, error) {
	if r.err != nil {
		return domainidentity.Session{}, r.err
	}
	return r.session, nil
}
