package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appidentity "moneo/internal/app/identity"
	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
	"moneo/internal/infra/security"
	transporthttp "moneo/internal/transport/http"

	"github.com/google/uuid"
)

func TestRegisterEndpointReturnsTokensAndSetsRefreshCookie(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "User@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var response authResponse
	decodeJSONResponse(t, rec, &response)
	if strings.TrimSpace(response.AccessToken) == "" {
		t.Fatal("access_token must not be empty")
	}
	if response.ExpiresIn != 900 {
		t.Fatalf("expected expires_in=900, got %d", response.ExpiresIn)
	}

	refreshCookie := findCookie(t, rec, transporthttp.RefreshCookieName)
	assertRefreshCookieAttributes(t, refreshCookie)

	if len(fixture.sessionRepo.sessions) != 1 {
		t.Fatalf("expected 1 session stored, got %d", len(fixture.sessionRepo.sessions))
	}

	expectedHash, err := fixture.tokenService.HashRefreshToken(refreshCookie.Value)
	if err != nil {
		t.Fatalf("hash refresh token from cookie: %v", err)
	}

	storedSession := fixture.sessionRepo.sessions[0]
	if storedSession.RefreshTokenHash != expectedHash {
		t.Fatalf("expected stored refresh hash %q, got %q", expectedHash, storedSession.RefreshTokenHash)
	}
	if storedSession.RefreshTokenHash == refreshCookie.Value {
		t.Fatal("stored refresh hash must not equal raw refresh token")
	}
}

func TestLoginEndpointReturnsTokensAndSetsRefreshCookie(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response authResponse
	decodeJSONResponse(t, rec, &response)
	if strings.TrimSpace(response.AccessToken) == "" {
		t.Fatal("access_token must not be empty")
	}
	if response.ExpiresIn != 900 {
		t.Fatalf("expected expires_in=900, got %d", response.ExpiresIn)
	}

	refreshCookie := findCookie(t, rec, transporthttp.RefreshCookieName)
	assertRefreshCookieAttributes(t, refreshCookie)

	if len(fixture.sessionRepo.sessions) != 1 {
		t.Fatalf("expected 1 session stored, got %d", len(fixture.sessionRepo.sessions))
	}

	expectedHash, err := fixture.tokenService.HashRefreshToken(refreshCookie.Value)
	if err != nil {
		t.Fatalf("hash refresh token from cookie: %v", err)
	}

	storedSession := fixture.sessionRepo.sessions[0]
	if storedSession.RefreshTokenHash != expectedHash {
		t.Fatalf("expected stored refresh hash %q, got %q", expectedHash, storedSession.RefreshTokenHash)
	}
}

func TestRegisterEndpointRejectsDuplicateEmail(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	first := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first register status 201, got %d", first.Code)
	}

	second := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "USER@example.com",
		"password":         "AnotherStrongPassw0rd!",
		"password_confirm": "AnotherStrongPassw0rd!",
	}, nil)
	if second.Code != http.StatusConflict {
		t.Fatalf("expected duplicate register status 409, got %d", second.Code)
	}

	var errResponse errorResponse
	decodeJSONResponse(t, second, &errResponse)
	if errResponse.Error != "duplicate_email" {
		t.Fatalf("expected duplicate_email error, got %q", errResponse.Error)
	}
}

func TestLoginEndpointRejectsInvalidCredentials(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "WrongPassw0rd!",
	}, nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var errResponse errorResponse
	decodeJSONResponse(t, rec, &errResponse)
	if errResponse.Error != "invalid_credentials" {
		t.Fatalf("expected invalid_credentials error, got %q", errResponse.Error)
	}
}

func TestRefreshEndpointUpdatesAccessTokenAndLastUsedAt(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	refreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	if fixture.sessionRepo.sessions[0].LastUsedAt != nil {
		t.Fatal("expected last_used_at to be nil before refresh")
	}

	fixture.clock.Advance(2 * time.Minute)
	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/refresh", nil, nil, refreshCookie)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response authResponse
	decodeJSONResponse(t, rec, &response)
	if strings.TrimSpace(response.AccessToken) == "" {
		t.Fatal("access_token must not be empty")
	}
	if response.ExpiresIn != 900 {
		t.Fatalf("expected expires_in=900, got %d", response.ExpiresIn)
	}

	refreshedCookie := findCookie(t, rec, transporthttp.RefreshCookieName)
	assertRefreshCookieAttributes(t, refreshedCookie)
	if refreshedCookie.Value != refreshCookie.Value {
		t.Fatal("refresh flow via cookie must keep the same refresh token")
	}

	lastUsedAt := fixture.sessionRepo.sessions[0].LastUsedAt
	if lastUsedAt == nil {
		t.Fatal("expected last_used_at to be updated")
	}
	if !lastUsedAt.Equal(fixture.clock.Now()) {
		t.Fatalf("expected last_used_at=%s, got %s", fixture.clock.Now(), *lastUsedAt)
	}
}

func TestRefreshEndpointAcceptsBodyTokenForMacOSFlow(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	refreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": refreshCookie.Value,
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response authResponse
	decodeJSONResponse(t, rec, &response)
	if strings.TrimSpace(response.AccessToken) == "" {
		t.Fatal("access_token must not be empty")
	}

	if hasCookie(rec, transporthttp.RefreshCookieName) {
		t.Fatal("body-based refresh must not set refresh cookie")
	}
}

func TestRefreshEndpointRejectsRevokedSession(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	refreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	now := fixture.clock.Now()
	fixture.sessionRepo.sessions[0].RevokedAt = &now

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/refresh", nil, nil, refreshCookie)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var response errorResponse
	decodeJSONResponse(t, rec, &response)
	if response.Error != "invalid_refresh_token" {
		t.Fatalf("expected invalid_refresh_token, got %q", response.Error)
	}
}

func TestRefreshEndpointRejectsExpiredSession(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	refreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	session := fixture.sessionRepo.sessions[0]
	fixture.clock.Set(session.ExpiresAt.Add(time.Second))

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/refresh", nil, nil, refreshCookie)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var response errorResponse
	decodeJSONResponse(t, rec, &response)
	if response.Error != "invalid_refresh_token" {
		t.Fatalf("expected invalid_refresh_token, got %q", response.Error)
	}
}

func TestLogoutEndpointRevokesSessionAndClearsCookie(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	refreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	fixture.clock.Advance(5 * time.Minute)
	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/logout", nil, nil, refreshCookie)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	clearedCookie := findCookie(t, rec, transporthttp.RefreshCookieName)
	assertClearedRefreshCookie(t, clearedCookie)

	session := fixture.sessionRepo.sessions[0]
	if session.RevokedAt == nil {
		t.Fatal("expected session to be revoked")
	}
	if !session.RevokedAt.Equal(fixture.clock.Now()) {
		t.Fatalf("expected revoked_at=%s, got %s", fixture.clock.Now(), *session.RevokedAt)
	}
}

func TestLogoutEndpointRevokesCurrentSessionFromAccessTokenWithoutCookie(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d", register.Code)
	}

	var registerResponse authResponse
	decodeJSONResponse(t, register, &registerResponse)

	logout := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{
		"Authorization": "Bearer " + registerResponse.AccessToken,
	})
	if logout.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d", logout.Code)
	}

	if len(fixture.sessionRepo.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(fixture.sessionRepo.sessions))
	}
	if fixture.sessionRepo.sessions[0].RevokedAt == nil {
		t.Fatal("expected current session to be revoked via access token")
	}
}

func TestLogoutAllEndpointRevokesAllSessionsAndKeepsAccessTokenValidUntilExpiry(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	firstRefreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	login := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, nil)
	secondRefreshCookie := findCookie(t, login, transporthttp.RefreshCookieName)

	var loginResponse authResponse
	decodeJSONResponse(t, login, &loginResponse)

	if len(fixture.sessionRepo.sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(fixture.sessionRepo.sessions))
	}

	logoutAll := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/logout-all", nil, map[string]string{
		"Authorization": "Bearer " + loginResponse.AccessToken,
	}, secondRefreshCookie)
	if logoutAll.Code != http.StatusOK {
		t.Fatalf("expected first logout-all status 200, got %d", logoutAll.Code)
	}

	clearedCookie := findCookie(t, logoutAll, transporthttp.RefreshCookieName)
	assertClearedRefreshCookie(t, clearedCookie)

	for i, session := range fixture.sessionRepo.sessions {
		if session.RevokedAt == nil {
			t.Fatalf("session %d must be revoked", i)
		}
	}

	refreshAfterLogoutAll := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/refresh", nil, nil, firstRefreshCookie)
	if refreshAfterLogoutAll.Code != http.StatusUnauthorized {
		t.Fatalf("expected refresh with revoked session status 401, got %d", refreshAfterLogoutAll.Code)
	}

	secondLogoutAll := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/logout-all", nil, map[string]string{
		"Authorization": "Bearer " + loginResponse.AccessToken,
	}, nil)
	if secondLogoutAll.Code != http.StatusOK {
		t.Fatalf("expected second logout-all status 200, got %d", secondLogoutAll.Code)
	}

	fixture.clock.Advance(16 * time.Minute)
	expiredAccessTokenCall := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/logout-all", nil, map[string]string{
		"Authorization": "Bearer " + loginResponse.AccessToken,
	}, nil)
	if expiredAccessTokenCall.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired access token status 401, got %d", expiredAccessTokenCall.Code)
	}

	var expiredResponse errorResponse
	decodeJSONResponse(t, expiredAccessTokenCall, &expiredResponse)
	if expiredResponse.Error != "invalid_access_token" {
		t.Fatalf("expected invalid_access_token, got %q", expiredResponse.Error)
	}
}

func TestAuthMeRejectsMissingToken(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/me", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var response errorResponse
	decodeJSONResponse(t, rec, &response)
	if response.Error != "invalid_access_token" {
		t.Fatalf("expected invalid_access_token, got %q", response.Error)
	}
}

func TestAuthMeRejectsExpiredToken(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)

	var registerResponse authResponse
	decodeJSONResponse(t, register, &registerResponse)

	fixture.clock.Advance(16 * time.Minute)
	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + registerResponse.AccessToken,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var response errorResponse
	decodeJSONResponse(t, rec, &response)
	if response.Error != "invalid_access_token" {
		t.Fatalf("expected invalid_access_token, got %q", response.Error)
	}
}

func TestAuthMeRejectsRevokedSession(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)

	var registerResponse authResponse
	decodeJSONResponse(t, register, &registerResponse)

	now := fixture.clock.Now()
	fixture.sessionRepo.sessions[0].RevokedAt = &now

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + registerResponse.AccessToken,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var response errorResponse
	decodeJSONResponse(t, rec, &response)
	if response.Error != "invalid_access_token" {
		t.Fatalf("expected invalid_access_token, got %q", response.Error)
	}
}

func TestAuthMeReturnsCurrentUserWithoutSensitiveFields(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)

	var registerResponse authResponse
	decodeJSONResponse(t, register, &registerResponse)

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + registerResponse.AccessToken,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]any
	decodeJSONResponse(t, rec, &payload)

	if payload["id"] == "" {
		t.Fatal("id must not be empty")
	}
	if payload["email"] != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %v", payload["email"])
	}
	if _, has := payload["password_hash"]; has {
		t.Fatal("response must not include password_hash")
	}
	if _, has := payload["refresh_token"]; has {
		t.Fatal("response must not include refresh_token")
	}
	if _, has := payload["password"]; has {
		t.Fatal("response must not include password")
	}
}

func TestSessionsEndpointReturnsOnlyCurrentUserSessions(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	userOneRegister := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user1@example.com",
		"password":         "StrongPassw0rd1!",
		"password_confirm": "StrongPassw0rd1!",
	}, nil)
	var userOneRegisterResponse authResponse
	decodeJSONResponse(t, userOneRegister, &userOneRegisterResponse)

	userOneLogin := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    "user1@example.com",
		"password": "StrongPassw0rd1!",
	}, nil)
	var userOneLoginResponse authResponse
	decodeJSONResponse(t, userOneLogin, &userOneLoginResponse)

	userTwoRegister := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user2@example.com",
		"password":         "StrongPassw0rd2!",
		"password_confirm": "StrongPassw0rd2!",
	}, nil)
	var userTwoRegisterResponse authResponse
	decodeJSONResponse(t, userTwoRegister, &userTwoRegisterResponse)

	rec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{
		"Authorization": "Bearer " + userOneLoginResponse.AccessToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response sessionsResponse
	decodeJSONResponse(t, rec, &response)
	if len(response.Sessions) != 2 {
		t.Fatalf("expected 2 sessions for user1, got %d", len(response.Sessions))
	}

	userTwoRec := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{
		"Authorization": "Bearer " + userTwoRegisterResponse.AccessToken,
	})
	if userTwoRec.Code != http.StatusOK {
		t.Fatalf("expected user2 status 200, got %d", userTwoRec.Code)
	}

	var userTwoResponse sessionsResponse
	decodeJSONResponse(t, userTwoRec, &userTwoResponse)
	if len(userTwoResponse.Sessions) != 1 {
		t.Fatalf("expected 1 session for user2, got %d", len(userTwoResponse.Sessions))
	}
}

func TestRevokeSessionEndpointDoesNotRevokeForeignSession(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	userOneRegister := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user1@example.com",
		"password":         "StrongPassw0rd1!",
		"password_confirm": "StrongPassw0rd1!",
	}, nil)
	var userOneRegisterResponse authResponse
	decodeJSONResponse(t, userOneRegister, &userOneRegisterResponse)

	userTwoRegister := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user2@example.com",
		"password":         "StrongPassw0rd2!",
		"password_confirm": "StrongPassw0rd2!",
	}, nil)
	var userTwoRegisterResponse authResponse
	decodeJSONResponse(t, userTwoRegister, &userTwoRegisterResponse)

	ownerSessions := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{
		"Authorization": "Bearer " + userOneRegisterResponse.AccessToken,
	})
	var ownerResponse sessionsResponse
	decodeJSONResponse(t, ownerSessions, &ownerResponse)
	targetSessionID := ownerResponse.Sessions[0].ID

	revoke := performJSONRequest(t, fixture.router, http.MethodDelete, "/api/v1/auth/sessions/"+targetSessionID, nil, map[string]string{
		"Authorization": "Bearer " + userTwoRegisterResponse.AccessToken,
	})
	if revoke.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for foreign session delete, got %d", revoke.Code)
	}

	ownerSessionsAfter := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{
		"Authorization": "Bearer " + userOneRegisterResponse.AccessToken,
	})
	var ownerAfterResponse sessionsResponse
	decodeJSONResponse(t, ownerSessionsAfter, &ownerAfterResponse)
	if len(ownerAfterResponse.Sessions) != 1 {
		t.Fatalf("expected owner's session to remain active, got %d active sessions", len(ownerAfterResponse.Sessions))
	}
}

func TestRevokeSessionEndpointRemovesSessionFromActiveList(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	var registerResponse authResponse
	decodeJSONResponse(t, register, &registerResponse)

	login := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, nil)
	var loginResponse authResponse
	decodeJSONResponse(t, login, &loginResponse)

	before := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{
		"Authorization": "Bearer " + loginResponse.AccessToken,
	})
	var beforeResponse sessionsResponse
	decodeJSONResponse(t, before, &beforeResponse)
	if len(beforeResponse.Sessions) != 2 {
		t.Fatalf("expected 2 active sessions before revoke, got %d", len(beforeResponse.Sessions))
	}

	targetSessionID := beforeResponse.Sessions[0].ID

	revoke := performJSONRequest(t, fixture.router, http.MethodDelete, "/api/v1/auth/sessions/"+targetSessionID, nil, map[string]string{
		"Authorization": "Bearer " + loginResponse.AccessToken,
	})
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", revoke.Code)
	}

	after := performJSONRequest(t, fixture.router, http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{
		"Authorization": "Bearer " + loginResponse.AccessToken,
	})
	var afterResponse sessionsResponse
	decodeJSONResponse(t, after, &afterResponse)
	if len(afterResponse.Sessions) != 1 {
		t.Fatalf("expected 1 active session after revoke, got %d", len(afterResponse.Sessions))
	}

	if afterResponse.Sessions[0].ID == targetSessionID {
		t.Fatal("revoked session must not appear in active sessions list")
	}
}

func TestRevokeSessionEndpointRejectsMalformedSessionID(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	var registerResponse authResponse
	decodeJSONResponse(t, register, &registerResponse)

	rec := performJSONRequest(t, fixture.router, http.MethodDelete, "/api/v1/auth/sessions/not-a-uuid", nil, map[string]string{
		"Authorization": "Bearer " + registerResponse.AccessToken,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed session id status 400, got %d", rec.Code)
	}
}

func TestLegacyRoutesReturnNotFoundAfterBigBangMigration(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/auth/register"},
		{method: http.MethodGet, path: "/auth/me"},
		{method: http.MethodGet, path: "/accounts"},
		{method: http.MethodGet, path: "/categories"},
		{method: http.MethodGet, path: "/subcategories"},
		{method: http.MethodGet, path: "/transactions"},
	}

	for _, tc := range cases {
		rec := performJSONRequest(t, fixture.router, tc.method, tc.path, nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s expected 404, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

type authEndpointsFixture struct {
	router              http.Handler
	authService         *appidentity.AuthService
	sessionRepo         *inMemorySessionRepo
	tokenRepo           *inMemoryOneTimeTokenRepo
	notificationService *captureAuthNotifications
	tokenService        *security.TokenService
	clock               *mutableClock
}

func newAuthEndpointsFixture(t *testing.T) authEndpointsFixture {
	return newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{})
}

func newAuthEndpointsFixtureWithRouterOptions(t *testing.T, routerOptions transporthttp.RouterOptions) authEndpointsFixture {
	t.Helper()

	clock := &mutableClock{now: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)}
	userRepo := newInMemoryUserRepo()
	sessionRepo := newInMemorySessionRepo()
	hasher := security.NewArgon2IDHasher(security.DefaultArgon2IDConfig())
	authService := appidentity.NewAuthService(
		userRepo,
		hasher,
		&sequenceUserIDGenerator{},
		clock,
	)

	tokenService, err := security.NewTokenService(security.TokenServiceConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		JWTSecret:       "test-jwt-secret",
	}, clock)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	authFlowService := appidentity.NewAuthFlowService(
		authService,
		sessionRepo,
		&sequenceSessionIDGenerator{},
		tokenService,
		clock,
	)

	tokenRepo := newInMemoryOneTimeTokenRepo()
	notificationService := &captureAuthNotifications{}
	postMVPService, err := appidentity.NewAuthPostMVPService(
		userRepo,
		sessionRepo,
		tokenRepo,
		tokenService,
		&sequenceOneTimeTokenIDGenerator{},
		hasher,
		nil,
		clock,
		notificationService,
		notificationService,
		appidentity.DefaultAuthPostMVPConfig(),
	)
	if err != nil {
		t.Fatalf("new post-mvp auth service: %v", err)
	}

	accessAuthService := appidentity.NewAccessAuthService(tokenService, userRepo, sessionRepo)
	authMiddleware := transporthttp.NewAuthMiddleware(accessAuthService)
	if routerOptions.AuthMiddleware == nil {
		routerOptions.AuthMiddleware = authMiddleware
	}
	if routerOptions.SecurityMiddleware == nil {
		cfg := transporthttp.DefaultAuthSecurityConfig()
		cfg.RequireHTTPSInProduction = false
		routerOptions.SecurityMiddleware = transporthttp.NewAuthSecurityMiddleware(cfg)
	}

	handler := transporthttp.NewAuthHandler(authFlowService, postMVPService)
	authOnlyHandler := transporthttp.NewAPIHandler(handler, nil)
	if routerOptions.StrictAPIHandler == nil {
		routerOptions.StrictAPIHandler = transporthttp.NewStrictAPIHandler(transporthttp.StrictAPIHandlerDeps{
			Accounts:      authOnlyHandler,
			Auth:          authOnlyHandler,
			Categories:    authOnlyHandler,
			Subcategories: authOnlyHandler,
			Transactions:  authOnlyHandler,
		})
	} else {
		routerOptions.StrictAPIHandler = transporthttp.NewStrictAPIHandler(transporthttp.StrictAPIHandlerDeps{
			Accounts:      routerOptions.StrictAPIHandler.(transporthttp.AccountsStrictHandler),
			Auth:          authOnlyHandler,
			Categories:    routerOptions.StrictAPIHandler.(transporthttp.CategoriesStrictHandler),
			Subcategories: routerOptions.StrictAPIHandler.(transporthttp.SubcategoriesStrictHandler),
			Transactions:  routerOptions.StrictAPIHandler.(transporthttp.TransactionsStrictHandler),
		})
	}
	router := transporthttp.NewRouterWithOptions(routerOptions)

	return authEndpointsFixture{
		router:              router,
		authService:         authService,
		sessionRepo:         sessionRepo,
		tokenRepo:           tokenRepo,
		notificationService: notificationService,
		tokenService:        tokenService,
		clock:               clock,
	}
}

func performJSONRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body any,
	headers map[string]string,
	cookies ...*http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody bytes.Buffer
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		requestBody.Write(payload)
	}

	req := httptest.NewRequest(method, path, &requestBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		req.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSONResponse(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
}

func findCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()

	resp := rec.Result()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}

	t.Fatalf("cookie %q not found in response", name)
	return nil
}

func hasCookie(rec *httptest.ResponseRecorder, name string) bool {
	resp := rec.Result()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == name {
			return true
		}
	}

	return false
}

func assertRefreshCookieAttributes(t *testing.T, cookie *http.Cookie) {
	t.Helper()

	if cookie.Value == "" {
		t.Fatal("refresh cookie value must not be empty")
	}
	if !cookie.HttpOnly {
		t.Fatal("refresh cookie must be HttpOnly")
	}
	if !cookie.Secure {
		t.Fatal("refresh cookie must be Secure")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
	if cookie.Path != "/api/v1/auth/refresh" {
		t.Fatalf("expected Path=/auth/refresh, got %q", cookie.Path)
	}
	if cookie.MaxAge != 2_592_000 {
		t.Fatalf("expected Max-Age=2592000, got %d", cookie.MaxAge)
	}
}

func assertClearedRefreshCookie(t *testing.T, cookie *http.Cookie) {
	t.Helper()

	if cookie.Name != transporthttp.RefreshCookieName {
		t.Fatalf("expected cookie name %q, got %q", transporthttp.RefreshCookieName, cookie.Name)
	}
	if cookie.Value != "" {
		t.Fatalf("expected empty cookie value on clear, got %q", cookie.Value)
	}
	if cookie.MaxAge >= 0 {
		t.Fatalf("expected Max-Age < 0 for clear, got %d", cookie.MaxAge)
	}
	if cookie.Path != "/api/v1/auth/refresh" {
		t.Fatalf("expected Path=/auth/refresh, got %q", cookie.Path)
	}
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type sessionsResponse struct {
	Sessions []sessionResponse `json:"sessions"`
}

type sessionResponse struct {
	ID         string     `json:"id"`
	UserAgent  *string    `json:"user_agent"`
	IP         *string    `json:"ip"`
	DeviceName *string    `json:"device_name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type inMemoryUserRepo struct {
	byNormalizedEmail map[string]domainidentity.User
}

func newInMemoryUserRepo() *inMemoryUserRepo {
	return &inMemoryUserRepo{byNormalizedEmail: map[string]domainidentity.User{}}
}

func (r *inMemoryUserRepo) Create(_ context.Context, user domainidentity.User) error {
	if _, exists := r.byNormalizedEmail[user.NormalizedEmail]; exists {
		return appidentity.ErrDuplicateEmail
	}

	r.byNormalizedEmail[user.NormalizedEmail] = user
	return nil
}

func (r *inMemoryUserRepo) FindByNormalizedEmail(_ context.Context, normalizedEmail string) (domainidentity.User, error) {
	user, ok := r.byNormalizedEmail[normalizedEmail]
	if !ok {
		return domainidentity.User{}, appidentity.ErrUserNotFound
	}

	return user, nil
}

func (r *inMemoryUserRepo) FindByID(_ context.Context, userID shared.UserID) (domainidentity.User, error) {
	for _, user := range r.byNormalizedEmail {
		if user.ID == userID {
			return user, nil
		}
	}

	return domainidentity.User{}, appidentity.ErrUserNotFound
}

func (r *inMemoryUserRepo) UpdatePassword(_ context.Context, userID shared.UserID, passwordHash string, updatedAt time.Time) error {
	for email, user := range r.byNormalizedEmail {
		if user.ID != userID {
			continue
		}

		user.PasswordHash = passwordHash
		user.UpdatedAt = updatedAt
		r.byNormalizedEmail[email] = user
		return nil
	}

	return appidentity.ErrUserNotFound
}

func (r *inMemoryUserRepo) MarkEmailVerified(_ context.Context, userID shared.UserID, updatedAt time.Time) error {
	for email, user := range r.byNormalizedEmail {
		if user.ID != userID {
			continue
		}

		user.EmailVerified = true
		user.UpdatedAt = updatedAt
		r.byNormalizedEmail[email] = user
		return nil
	}

	return appidentity.ErrUserNotFound
}

type inMemorySessionRepo struct {
	sessions []domainidentity.Session
}

func newInMemorySessionRepo() *inMemorySessionRepo {
	return &inMemorySessionRepo{
		sessions: make([]domainidentity.Session, 0, 8),
	}
}

func (r *inMemorySessionRepo) Create(_ context.Context, session domainidentity.Session) error {
	for _, existing := range r.sessions {
		if existing.RefreshTokenHash == session.RefreshTokenHash && existing.RevokedAt == nil {
			return appidentity.ErrDuplicateSessionRefreshToken
		}
	}

	r.sessions = append(r.sessions, session)
	return nil
}

func (r *inMemorySessionRepo) FindByRefreshTokenHash(_ context.Context, refreshTokenHash string) (domainidentity.Session, error) {
	for _, session := range r.sessions {
		if session.RefreshTokenHash == refreshTokenHash {
			return session, nil
		}
	}

	return domainidentity.Session{}, appidentity.ErrSessionNotFound
}

func (r *inMemorySessionRepo) FindByID(_ context.Context, sessionID shared.SessionID) (domainidentity.Session, error) {
	for _, session := range r.sessions {
		if session.ID == sessionID {
			return session, nil
		}
	}

	return domainidentity.Session{}, appidentity.ErrSessionNotFound
}

func (r *inMemorySessionRepo) ListActiveByUserID(_ context.Context, userID shared.UserID, now time.Time) ([]domainidentity.Session, error) {
	result := make([]domainidentity.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		if session.UserID != userID {
			continue
		}
		if session.RevokedAt != nil {
			continue
		}
		if !session.ExpiresAt.After(now) {
			continue
		}

		result = append(result, session)
	}

	return result, nil
}

func (r *inMemorySessionRepo) TouchLastUsedAt(_ context.Context, sessionID shared.SessionID, lastUsedAt time.Time) error {
	for i := range r.sessions {
		if r.sessions[i].ID == sessionID {
			if r.sessions[i].RevokedAt != nil || !r.sessions[i].ExpiresAt.After(lastUsedAt) {
				return appidentity.ErrSessionNotFound
			}
			lastUsedAtCopy := lastUsedAt
			r.sessions[i].LastUsedAt = &lastUsedAtCopy
			return nil
		}
	}

	return appidentity.ErrSessionNotFound
}

func (r *inMemorySessionRepo) RevokeByID(_ context.Context, sessionID shared.SessionID, revokedAt time.Time) error {
	for i := range r.sessions {
		if r.sessions[i].ID == sessionID {
			revokedAtCopy := revokedAt
			r.sessions[i].RevokedAt = &revokedAtCopy
			return nil
		}
	}

	return appidentity.ErrSessionNotFound
}

func (r *inMemorySessionRepo) RevokeAllByUserID(_ context.Context, userID shared.UserID, revokedAt time.Time) error {
	for i := range r.sessions {
		if r.sessions[i].UserID == userID && r.sessions[i].RevokedAt == nil && r.sessions[i].ExpiresAt.After(revokedAt) {
			revokedAtCopy := revokedAt
			r.sessions[i].RevokedAt = &revokedAtCopy
		}
	}

	return nil
}

type inMemoryOneTimeTokenRepo struct {
	tokens []domainidentity.OneTimeToken
}

func newInMemoryOneTimeTokenRepo() *inMemoryOneTimeTokenRepo {
	return &inMemoryOneTimeTokenRepo{
		tokens: make([]domainidentity.OneTimeToken, 0, 8),
	}
}

func (r *inMemoryOneTimeTokenRepo) Create(_ context.Context, token domainidentity.OneTimeToken) error {
	for _, existing := range r.tokens {
		if existing.TokenHash == token.TokenHash {
			return fmt.Errorf("duplicate one-time token hash")
		}
	}

	r.tokens = append(r.tokens, token)
	return nil
}

func (r *inMemoryOneTimeTokenRepo) FindActiveByHash(
	_ context.Context,
	purpose domainidentity.OneTimeTokenPurpose,
	tokenHash string,
	now time.Time,
) (domainidentity.OneTimeToken, error) {
	for _, token := range r.tokens {
		if token.Purpose != purpose || token.TokenHash != tokenHash {
			continue
		}
		if token.UsedAt != nil || !token.ExpiresAt.After(now) {
			continue
		}
		return token, nil
	}

	return domainidentity.OneTimeToken{}, appidentity.ErrOneTimeTokenNotFound
}

func (r *inMemoryOneTimeTokenRepo) MarkUsed(_ context.Context, tokenID shared.OneTimeTokenID, usedAt time.Time) error {
	for i := range r.tokens {
		if r.tokens[i].ID != tokenID {
			continue
		}
		if r.tokens[i].UsedAt != nil || !r.tokens[i].ExpiresAt.After(usedAt) {
			return appidentity.ErrOneTimeTokenNotFound
		}

		usedAtCopy := usedAt
		r.tokens[i].UsedAt = &usedAtCopy
		return nil
	}

	return appidentity.ErrOneTimeTokenNotFound
}

type captureAuthNotifications struct {
	passwordResetTokens     []string
	emailVerificationTokens []string
}

func (s *captureAuthNotifications) SendPasswordReset(_ context.Context, _ domainidentity.User, token string) error {
	s.passwordResetTokens = append(s.passwordResetTokens, token)
	return nil
}

func (s *captureAuthNotifications) SendEmailVerification(_ context.Context, _ domainidentity.User, token string) error {
	s.emailVerificationTokens = append(s.emailVerificationTokens, token)
	return nil
}

type sequenceUserIDGenerator struct {
	next int
}

func (g *sequenceUserIDGenerator) NewUserID() shared.UserID {
	g.next++
	return shared.UserID(fmt.Sprintf("user-%d", g.next))
}

type sequenceSessionIDGenerator struct {
	next int
}

func (g *sequenceSessionIDGenerator) NewSessionID() shared.SessionID {
	return shared.SessionID(uuid.NewString())
}

type sequenceOneTimeTokenIDGenerator struct {
	next int
}

func (g *sequenceOneTimeTokenIDGenerator) NewOneTimeTokenID() shared.OneTimeTokenID {
	g.next++
	return shared.OneTimeTokenID(fmt.Sprintf("otk-%d", g.next))
}

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	return c.now
}

func (c *mutableClock) Advance(duration time.Duration) {
	c.now = c.now.Add(duration)
}

func (c *mutableClock) Set(now time.Time) {
	c.now = now
}
