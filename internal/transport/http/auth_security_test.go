package http_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	appidentity "moneo/internal/app/identity"
	transporthttp "moneo/internal/transport/http"
)

func TestLoginEndpointRateLimitBlocksBruteForceAttempts(t *testing.T) {
	var logBuffer bytes.Buffer

	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		SecurityMiddleware: transporthttp.NewAuthSecurityMiddleware(transporthttp.AuthSecurityConfig{
			RateLimits: map[string]transporthttp.AuthRateLimitRule{
				"/auth/login":           {MaxAttempts: 2, Window: time.Minute},
				"/auth/register":        {MaxAttempts: 5, Window: time.Minute},
				"/auth/refresh":         {MaxAttempts: 5, Window: time.Minute},
				"/auth/forgot-password": {MaxAttempts: 5, Window: time.Minute},
			},
			Logger: log.New(&logBuffer, "", 0),
		}),
	})

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		rec := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
			"email":    "user@example.com",
			"password": "WrongPassw0rd!",
		}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected status 401, got %d", attempt, rec.Code)
		}
	}

	blocked := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "WrongPassw0rd!",
	}, nil)
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", blocked.Code)
	}

	var response errorResponse
	decodeJSONResponse(t, blocked, &response)
	if response.Error != "rate_limited" {
		t.Fatalf("expected rate_limited error, got %q", response.Error)
	}
	if strings.TrimSpace(blocked.Header().Get("Retry-After")) == "" {
		t.Fatal("expected Retry-After header for rate-limited response")
	}

	if !strings.Contains(logBuffer.String(), "security_event=auth_rate_limited") {
		t.Fatal("expected rate-limited security event in logs")
	}
}

func TestLoginEndpointRateLimitIgnoresUntrustedForwardedFor(t *testing.T) {
	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		SecurityMiddleware: transporthttp.NewAuthSecurityMiddleware(transporthttp.AuthSecurityConfig{
			RateLimits: map[string]transporthttp.AuthRateLimitRule{
				"/auth/login": {MaxAttempts: 2, Window: time.Minute},
			},
		}),
	})

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		rec := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
			"email":    "user@example.com",
			"password": "WrongPassw0rd!",
		}, map[string]string{
			"X-Forwarded-For": fmt.Sprintf("203.0.113.%d", attempt),
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected status 401, got %d", attempt, rec.Code)
		}
	}

	blocked := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "WrongPassw0rd!",
	}, map[string]string{
		"X-Forwarded-For": "203.0.113.200",
	})
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", blocked.Code)
	}
}

func TestAuthSecurityLogsDoNotContainSensitiveValues(t *testing.T) {
	var logBuffer bytes.Buffer

	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		SecurityMiddleware: transporthttp.NewAuthSecurityMiddleware(transporthttp.AuthSecurityConfig{
			Logger: log.New(&logBuffer, "", 0),
		}),
	})

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	rawPassword := "WrongPassw0rd!"
	rawRefreshToken := "raw-refresh-token-should-not-appear"
	rawAuthorization := "raw-access-token-should-not-appear"

	login := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": rawPassword,
	}, map[string]string{
		"Authorization": "Bearer " + rawAuthorization,
	})
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("expected login status 401, got %d", login.Code)
	}

	refresh := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": rawRefreshToken,
	}, map[string]string{
		"Authorization": "Bearer " + rawAuthorization,
	})
	if refresh.Code != http.StatusUnauthorized {
		t.Fatalf("expected refresh status 401, got %d", refresh.Code)
	}

	securityLogs := logBuffer.String()
	if strings.TrimSpace(securityLogs) == "" {
		t.Fatal("expected security logs to contain suspicious event entries")
	}
	if strings.Contains(securityLogs, rawPassword) {
		t.Fatal("security logs must not include raw password")
	}
	if strings.Contains(securityLogs, rawRefreshToken) {
		t.Fatal("security logs must not include raw refresh token")
	}
	if strings.Contains(securityLogs, rawAuthorization) {
		t.Fatal("security logs must not include raw Authorization token")
	}
}

func TestAuthSecurityRequiresHTTPSInProduction(t *testing.T) {
	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		SecurityMiddleware: transporthttp.NewAuthSecurityMiddleware(transporthttp.AuthSecurityConfig{
			RequireHTTPSInProduction: true,
		}),
	})

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	insecure := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, nil)
	if insecure.Code != http.StatusBadRequest {
		t.Fatalf("expected insecure status 400, got %d", insecure.Code)
	}

	var insecureResponse errorResponse
	decodeJSONResponse(t, insecure, &insecureResponse)
	if insecureResponse.Error != "https_required" {
		t.Fatalf("expected https_required error, got %q", insecureResponse.Error)
	}

	forgedForwarded := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, map[string]string{
		"X-Forwarded-Proto": "https",
	})
	if forgedForwarded.Code != http.StatusBadRequest {
		t.Fatalf("expected forged forwarded status 400, got %d", forgedForwarded.Code)
	}
}

func TestAuthSecurityAllowsTrustedForwardedProto(t *testing.T) {
	fixture := newAuthEndpointsFixtureWithRouterOptions(t, transporthttp.RouterOptions{
		SecurityMiddleware: transporthttp.NewAuthSecurityMiddleware(transporthttp.AuthSecurityConfig{
			RequireHTTPSInProduction: true,
			TrustForwardedHeaders:    true,
			TrustedProxyCIDRs:        []string{"192.0.2.0/24"},
		}),
	})

	_, err := fixture.authService.Register(context.Background(), appidentity.RegisterInput{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	})
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	secure := performJSONRequest(t, fixture.router, http.MethodPost, "/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "StrongPassw0rd!",
	}, map[string]string{
		"X-Forwarded-Proto": "https",
	})
	if secure.Code != http.StatusOK {
		t.Fatalf("expected secure status 200, got %d", secure.Code)
	}

	refreshCookie := findCookie(t, secure, transporthttp.RefreshCookieName)
	assertRefreshCookieAttributes(t, refreshCookie)
}

func TestDefaultAuthSecurityConfigTreatsProdAsProduction(t *testing.T) {
	t.Setenv("APP_ENV", "prod")

	cfg := transporthttp.DefaultAuthSecurityConfig()
	if !cfg.RequireHTTPSInProduction {
		t.Fatal("expected APP_ENV=prod to enable production HTTPS requirement")
	}
}
