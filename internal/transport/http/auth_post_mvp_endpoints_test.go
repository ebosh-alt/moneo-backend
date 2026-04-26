package http_test

import (
	"net/http"
	"strings"
	"testing"

	transporthttp "moneo/internal/transport/http"
)

func TestForgotPasswordEndpointReturnsOKWithoutEmailDisclosure(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d", register.Code)
	}

	existing := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "user@example.com",
	}, nil)
	if existing.Code != http.StatusOK {
		t.Fatalf("expected existing-email forgot status 200, got %d", existing.Code)
	}
	if len(fixture.notificationService.passwordResetTokens) != 1 {
		t.Fatalf("expected 1 password reset token sent, got %d", len(fixture.notificationService.passwordResetTokens))
	}
	if len(fixture.tokenRepo.tokens) != 1 {
		t.Fatalf("expected 1 stored one-time token, got %d", len(fixture.tokenRepo.tokens))
	}
	if fixture.tokenRepo.tokens[0].TokenHash == fixture.notificationService.passwordResetTokens[0] {
		t.Fatal("stored password reset token must be hash-only")
	}

	missing := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "missing@example.com",
	}, nil)
	if missing.Code != http.StatusOK {
		t.Fatalf("expected missing-email forgot status 200, got %d", missing.Code)
	}
	if len(fixture.notificationService.passwordResetTokens) != 1 {
		t.Fatalf("expected no additional token for missing email, got %d total", len(fixture.notificationService.passwordResetTokens))
	}
}

func TestResetPasswordEndpointRevokesSessionsAndConsumesToken(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	firstRefreshCookie := findCookie(t, register, transporthttp.RefreshCookieName)

	login := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d", login.Code)
	}

	forgot := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "user@example.com",
	}, nil)
	if forgot.Code != http.StatusOK {
		t.Fatalf("expected forgot-password status 200, got %d", forgot.Code)
	}
	if len(fixture.notificationService.passwordResetTokens) == 0 {
		t.Fatal("expected issued password reset token")
	}

	resetToken := fixture.notificationService.passwordResetTokens[len(fixture.notificationService.passwordResetTokens)-1]
	reset := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/reset-password", map[string]any{
		"token":            resetToken,
		"password":         "NewStrongPassw0rd!",
		"password_confirm": "NewStrongPassw0rd!",
	}, nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("expected reset-password status 200, got %d", reset.Code)
	}

	refreshAfterReset := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/refresh", nil, nil, firstRefreshCookie)
	if refreshAfterReset.Code != http.StatusUnauthorized {
		t.Fatalf("expected refresh after reset status 401, got %d", refreshAfterReset.Code)
	}

	oldPasswordLogin := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, nil)
	if oldPasswordLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password login status 401, got %d", oldPasswordLogin.Code)
	}

	newPasswordLogin := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "NewStrongPassw0rd!",
	}, nil)
	if newPasswordLogin.Code != http.StatusOK {
		t.Fatalf("expected new password login status 200, got %d", newPasswordLogin.Code)
	}

	reuse := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/reset-password", map[string]any{
		"token":            resetToken,
		"password":         "AnotherStrongPassw0rd!",
		"password_confirm": "AnotherStrongPassw0rd!",
	}, nil)
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused reset token status 401, got %d", reuse.Code)
	}

	var errResponse errorResponse
	decodeJSONResponse(t, reuse, &errResponse)
	if errResponse.Error != "invalid_reset_token" {
		t.Fatalf("expected invalid_reset_token error, got %q", errResponse.Error)
	}
}

func TestEmailVerificationFlow(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	register := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/register", map[string]any{
		"email":            "user@example.com",
		"password":         "StrongPassw0rd!",
		"password_confirm": "StrongPassw0rd!",
	}, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d", register.Code)
	}

	var authTokens authResponse
	decodeJSONResponse(t, register, &authTokens)

	sendMissingToken := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/send-verification-email", nil, nil)
	if sendMissingToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected send-verification without token status 401, got %d", sendMissingToken.Code)
	}

	send := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/send-verification-email", nil, map[string]string{
		"Authorization": "Bearer " + authTokens.AccessToken,
	})
	if send.Code != http.StatusOK {
		t.Fatalf("expected send-verification status 200, got %d", send.Code)
	}
	if len(fixture.notificationService.emailVerificationTokens) == 0 {
		t.Fatal("expected issued email verification token")
	}
	if len(fixture.tokenRepo.tokens) == 0 {
		t.Fatal("expected stored one-time token for email verification")
	}
	lastStoredToken := fixture.tokenRepo.tokens[len(fixture.tokenRepo.tokens)-1]
	lastSentToken := fixture.notificationService.emailVerificationTokens[len(fixture.notificationService.emailVerificationTokens)-1]
	if lastStoredToken.TokenHash == lastSentToken {
		t.Fatal("stored email verification token must be hash-only")
	}

	verifyToken := fixture.notificationService.emailVerificationTokens[len(fixture.notificationService.emailVerificationTokens)-1]
	verify := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/verify-email", map[string]any{
		"token": verifyToken,
	}, nil)
	if verify.Code != http.StatusOK {
		t.Fatalf("expected verify-email status 200, got %d", verify.Code)
	}

	me := performJSONRequest(t, fixture.router, http.MethodGet, "/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + authTokens.AccessToken,
	})
	if me.Code != http.StatusOK {
		t.Fatalf("expected /auth/me status 200, got %d", me.Code)
	}

	var payload map[string]any
	decodeJSONResponse(t, me, &payload)
	if verifiedRaw, ok := payload["email_verified"]; !ok || verifiedRaw != true {
		t.Fatalf("expected email_verified=true, got %#v", payload["email_verified"])
	}

	verifyAgain := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/verify-email", map[string]any{
		"token": verifyToken,
	}, nil)
	if verifyAgain.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused verify token status 401, got %d", verifyAgain.Code)
	}

	var errResponse errorResponse
	decodeJSONResponse(t, verifyAgain, &errResponse)
	if errResponse.Error != "invalid_verification_token" {
		t.Fatalf("expected invalid_verification_token error, got %q", errResponse.Error)
	}
}

func TestForgotPasswordEndpointValidatesInput(t *testing.T) {
	fixture := newAuthEndpointsFixture(t)

	rec := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": strings.Repeat("x", 400),
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
