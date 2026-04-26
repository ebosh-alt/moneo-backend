package security

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"moneo/internal/domain/shared"
)

func TestTokenServiceIssueAndVerifyAccessToken(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	service, err := NewTokenService(TokenServiceConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		JWTSecret:       "test-secret",
	}, staticNow{value: now})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := service.IssueAccessToken(shared.UserID("user-1"), shared.SessionID("session-1"))
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	claims, err := service.VerifyAccessToken(token)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}

	if claims.Subject != "user-1" {
		t.Fatalf("expected sub user-1, got %q", claims.Subject)
	}
	if claims.SessionID != "session-1" {
		t.Fatalf("expected session_id session-1, got %q", claims.SessionID)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) != 15*time.Minute {
		t.Fatalf("expected 15m lifetime, got %s", claims.ExpiresAt.Sub(claims.IssuedAt))
	}

	payload := decodeJWTPayload(t, token)
	if len(payload) != 4 {
		t.Fatalf("expected exactly 4 claims, got %d", len(payload))
	}
	if _, ok := payload["sub"]; !ok {
		t.Fatal("sub claim is missing")
	}
	if _, ok := payload["session_id"]; !ok {
		t.Fatal("session_id claim is missing")
	}
	if _, ok := payload["iat"]; !ok {
		t.Fatal("iat claim is missing")
	}
	if _, ok := payload["exp"]; !ok {
		t.Fatal("exp claim is missing")
	}
	if _, hasPassword := payload["password"]; hasPassword {
		t.Fatal("access token must not contain password")
	}
	if _, hasRefreshToken := payload["refresh_token"]; hasRefreshToken {
		t.Fatal("access token must not contain refresh token")
	}
}

func TestTokenServiceVerifyAccessTokenRejectsExpired(t *testing.T) {
	issueTime := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	verifyTime := issueTime.Add(16 * time.Minute)

	issueService, err := NewTokenService(TokenServiceConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		JWTSecret:       "test-secret",
	}, staticNow{value: issueTime})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := issueService.IssueAccessToken(shared.UserID("user-1"), shared.SessionID("session-1"))
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	verifyService, err := NewTokenService(TokenServiceConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		JWTSecret:       "test-secret",
	}, staticNow{value: verifyTime})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	_, err = verifyService.VerifyAccessToken(token)
	if err == nil {
		t.Fatal("expected expired token error")
	}
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenServiceVerifyAccessTokenRejectsTamperedSignature(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	service, err := NewTokenService(TokenServiceConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		JWTSecret:       "test-secret",
	}, staticNow{value: now})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := service.IssueAccessToken(shared.UserID("user-1"), shared.SessionID("session-1"))
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	parts := strings.Split(token, ".")
	parts[1] = "tampered_payload"
	tampered := strings.Join(parts, ".")

	_, err = service.VerifyAccessToken(tampered)
	if err != ErrInvalidTokenSignature {
		t.Fatalf("expected ErrInvalidTokenSignature, got %v", err)
	}
}

func TestTokenServiceIssueRefreshTokenAndHashFlow(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	service, err := NewTokenService(TokenServiceConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		JWTSecret:       "test-secret",
	}, staticNow{value: now})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	issued, err := service.IssueRefreshToken()
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}

	if strings.Count(issued.Token, ".") == 2 {
		t.Fatal("refresh token must be opaque and not JWT")
	}
	if issued.Hash == issued.Token {
		t.Fatal("refresh token hash must differ from raw token")
	}
	if strings.Contains(issued.Hash, issued.Token) {
		t.Fatal("refresh token hash must not include raw token")
	}
	if issued.ExpiresAt.Sub(now) != 30*24*time.Hour {
		t.Fatalf("expected 30d refresh lifetime, got %s", issued.ExpiresAt.Sub(now))
	}

	ok, err := service.VerifyRefreshTokenHash(issued.Token, issued.Hash)
	if err != nil {
		t.Fatalf("verify refresh token hash: %v", err)
	}
	if !ok {
		t.Fatal("expected refresh token hash verification to pass")
	}

	ok, err = service.VerifyRefreshTokenHash("other-token", issued.Hash)
	if err != nil {
		t.Fatalf("verify refresh token hash with wrong token: %v", err)
	}
	if ok {
		t.Fatal("expected wrong refresh token to fail hash verification")
	}
}

func decodeJWTPayload(t *testing.T, token string) map[string]any {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected JWT with 3 parts, got %d", len(parts))
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	return payload
}

type staticNow struct {
	value time.Time
}

func (s staticNow) Now() time.Time {
	return s.value
}
