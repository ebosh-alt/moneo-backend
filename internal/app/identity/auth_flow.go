package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
)

var ErrDuplicateSessionRefreshToken = errors.New("duplicate session refresh token")
var ErrSessionNotFound = errors.New("session not found")
var ErrInvalidRefreshToken = errors.New("invalid refresh token")
var ErrInvalidAccessToken = errors.New("invalid access token")

type SessionRepository interface {
	Create(ctx context.Context, session domainidentity.Session) error
	FindByID(ctx context.Context, sessionID shared.SessionID) (domainidentity.Session, error)
	ListActiveByUserID(ctx context.Context, userID shared.UserID, now time.Time) ([]domainidentity.Session, error)
	FindByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (domainidentity.Session, error)
	TouchLastUsedAt(ctx context.Context, sessionID shared.SessionID, lastUsedAt time.Time) error
	RevokeByID(ctx context.Context, sessionID shared.SessionID, revokedAt time.Time) error
	RevokeAllByUserID(ctx context.Context, userID shared.UserID, revokedAt time.Time) error
}

type SessionIDGenerator interface {
	NewSessionID() shared.SessionID
}

type TokenIssuer interface {
	IssueAccessToken(userID shared.UserID, sessionID shared.SessionID) (string, error)
	IssueRefreshToken() (token string, hash string, expiresAt time.Time, err error)
	HashRefreshToken(refreshToken string) (string, error)
	VerifyAccessTokenIdentity(accessToken string) (shared.UserID, shared.SessionID, error)
	AccessTokenTTL() time.Duration
}

type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

type RefreshInput struct {
	RefreshToken string
}

type LogoutInput struct {
	RefreshToken string
}

type LogoutAllInput struct {
	AccessToken string
}

type LogoutCurrentInput struct {
	AccessToken string
}

type ListSessionsInput struct {
	UserID shared.UserID
}

type RevokeSessionInput struct {
	UserID    shared.UserID
	SessionID shared.SessionID
}

type UserSessionView struct {
	ID         shared.SessionID
	UserAgent  *string
	IP         *string
	DeviceName *string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  time.Time
}

type AuthFlowService struct {
	auth         *AuthService
	sessions     SessionRepository
	sessionIDGen SessionIDGenerator
	tokens       TokenIssuer
	clock        Clock
}

func NewAuthFlowService(
	auth *AuthService,
	sessions SessionRepository,
	sessionIDGen SessionIDGenerator,
	tokens TokenIssuer,
	clock Clock,
) *AuthFlowService {
	return &AuthFlowService{
		auth:         auth,
		sessions:     sessions,
		sessionIDGen: sessionIDGen,
		tokens:       tokens,
		clock:        clock,
	}
}

func (s *AuthFlowService) Register(ctx context.Context, input RegisterInput) (AuthTokens, error) {
	user, err := s.auth.Register(ctx, input)
	if err != nil {
		return AuthTokens{}, err
	}

	return s.createSessionAndIssueTokens(ctx, user)
}

func (s *AuthFlowService) Login(ctx context.Context, input LoginInput) (AuthTokens, error) {
	user, err := s.auth.Login(ctx, input)
	if err != nil {
		return AuthTokens{}, err
	}

	return s.createSessionAndIssueTokens(ctx, user)
}

func (s *AuthFlowService) Refresh(ctx context.Context, input RefreshInput) (AuthTokens, error) {
	return s.refreshByToken(ctx, input.RefreshToken)
}

func (s *AuthFlowService) Logout(ctx context.Context, input LogoutInput) error {
	return s.logoutByRefreshToken(ctx, input.RefreshToken)
}

func (s *AuthFlowService) LogoutAll(ctx context.Context, input LogoutAllInput) error {
	userID, _, err := s.tokens.VerifyAccessTokenIdentity(input.AccessToken)
	if err != nil {
		return ErrInvalidAccessToken
	}

	if err := s.sessions.RevokeAllByUserID(ctx, userID, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("revoke all sessions by user id: %w", err)
	}

	return nil
}

func (s *AuthFlowService) LogoutCurrent(ctx context.Context, input LogoutCurrentInput) error {
	_, sessionID, err := s.tokens.VerifyAccessTokenIdentity(input.AccessToken)
	if err != nil {
		return ErrInvalidAccessToken
	}

	if err := s.sessions.RevokeByID(ctx, sessionID, s.clock.Now().UTC()); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return fmt.Errorf("revoke current session by access token: %w", err)
	}

	return nil
}

func (s *AuthFlowService) ListActiveSessions(ctx context.Context, input ListSessionsInput) ([]UserSessionView, error) {
	now := s.clock.Now().UTC()
	sessions, err := s.sessions.ListActiveByUserID(ctx, input.UserID, now)
	if err != nil {
		return nil, fmt.Errorf("list active sessions by user id: %w", err)
	}

	result := make([]UserSessionView, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, UserSessionView{
			ID:         session.ID,
			UserAgent:  session.UserAgent,
			IP:         session.IP,
			DeviceName: session.DeviceName,
			CreatedAt:  session.CreatedAt,
			LastUsedAt: session.LastUsedAt,
			ExpiresAt:  session.ExpiresAt,
		})
	}

	return result, nil
}

func (s *AuthFlowService) RevokeSession(ctx context.Context, input RevokeSessionInput) error {
	session, err := s.sessions.FindByID(ctx, input.SessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ErrSessionNotFound
		}

		return fmt.Errorf("find session by id: %w", err)
	}

	if session.UserID != input.UserID {
		return ErrSessionNotFound
	}

	now := s.clock.Now().UTC()
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return ErrSessionNotFound
	}

	if err := s.sessions.RevokeByID(ctx, input.SessionID, now); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("revoke session by id: %w", err)
	}

	return nil
}

func (s *AuthFlowService) createSessionAndIssueTokens(ctx context.Context, user domainidentity.User) (AuthTokens, error) {
	refreshToken, refreshTokenHash, refreshTokenExpiresAt, err := s.tokens.IssueRefreshToken()
	if err != nil {
		return AuthTokens{}, fmt.Errorf("issue refresh token: %w", err)
	}

	now := s.clock.Now().UTC()
	session, err := domainidentity.NewSession(
		s.sessionIDGen.NewSessionID(),
		user.ID,
		refreshTokenHash,
		now,
		refreshTokenExpiresAt,
	)
	if err != nil {
		return AuthTokens{}, fmt.Errorf("build session: %w", err)
	}

	if err := s.sessions.Create(ctx, session); err != nil {
		if errors.Is(err, ErrDuplicateSessionRefreshToken) {
			return AuthTokens{}, ErrDuplicateSessionRefreshToken
		}

		return AuthTokens{}, fmt.Errorf("store session: %w", err)
	}

	accessToken, err := s.tokens.IssueAccessToken(user.ID, session.ID)
	if err != nil {
		return AuthTokens{}, fmt.Errorf("issue access token: %w", err)
	}

	expiresIn := int64(s.tokens.AccessTokenTTL() / time.Second)

	return AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

func (s *AuthFlowService) refreshByToken(ctx context.Context, refreshToken string) (AuthTokens, error) {
	session, err := s.findActiveSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return AuthTokens{}, err
	}

	now := s.clock.Now().UTC()
	if err := s.sessions.TouchLastUsedAt(ctx, session.ID, now); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return AuthTokens{}, ErrInvalidRefreshToken
		}
		return AuthTokens{}, fmt.Errorf("touch session last_used_at: %w", err)
	}

	accessToken, err := s.tokens.IssueAccessToken(session.UserID, session.ID)
	if err != nil {
		return AuthTokens{}, fmt.Errorf("issue access token: %w", err)
	}

	return AuthTokens{
		AccessToken: accessToken,
		ExpiresIn:   int64(s.tokens.AccessTokenTTL() / time.Second),
	}, nil
}

func (s *AuthFlowService) logoutByRefreshToken(ctx context.Context, refreshToken string) error {
	session, err := s.findActiveSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return err
	}

	if err := s.sessions.RevokeByID(ctx, session.ID, s.clock.Now().UTC()); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ErrInvalidRefreshToken
		}
		return fmt.Errorf("revoke session by id: %w", err)
	}

	return nil
}

func (s *AuthFlowService) findActiveSessionByRefreshToken(ctx context.Context, refreshToken string) (domainidentity.Session, error) {
	refreshTokenHash, err := s.tokens.HashRefreshToken(refreshToken)
	if err != nil {
		return domainidentity.Session{}, ErrInvalidRefreshToken
	}

	session, err := s.sessions.FindByRefreshTokenHash(ctx, refreshTokenHash)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return domainidentity.Session{}, ErrInvalidRefreshToken
		}
		return domainidentity.Session{}, fmt.Errorf("find session by refresh token hash: %w", err)
	}

	now := s.clock.Now().UTC()
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return domainidentity.Session{}, ErrInvalidRefreshToken
	}

	return session, nil
}
