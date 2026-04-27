package bootstrap

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	EnvAuthJWTSecret       = "AUTH_JWT_SECRET"
	EnvAuthAccessTokenTTL  = "AUTH_ACCESS_TOKEN_TTL"
	EnvAuthRefreshTokenTTL = "AUTH_REFRESH_TOKEN_TTL"
)

type AuthTokenConfig struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	JWTSecret       string
}

func LoadAuthTokenConfigFromEnv() (AuthTokenConfig, error) {
	accessTokenTTL, err := durationFromEnv(EnvAuthAccessTokenTTL, 15*time.Minute)
	if err != nil {
		return AuthTokenConfig{}, err
	}

	refreshTokenTTL, err := durationFromEnv(EnvAuthRefreshTokenTTL, 30*24*time.Hour)
	if err != nil {
		return AuthTokenConfig{}, err
	}

	jwtSecret := strings.TrimSpace(os.Getenv(EnvAuthJWTSecret))
	if jwtSecret == "" {
		return AuthTokenConfig{}, fmt.Errorf("%s is required", EnvAuthJWTSecret)
	}

	return AuthTokenConfig{
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
		JWTSecret:       jwtSecret,
	}, nil
}

func durationFromEnv(envKey string, defaultValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(envKey))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s duration %q: %w", envKey, value, err)
	}

	return parsed, nil
}
