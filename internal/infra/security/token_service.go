package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"moneo/internal/domain/shared"
)

const (
	minAccessTokenTTL = 10 * time.Minute
	maxAccessTokenTTL = 15 * time.Minute
)

var (
	ErrJWTSecretRequired     = errors.New("jwt secret is required")
	ErrInvalidAccessTokenTTL = errors.New("invalid access token ttl")
	ErrInvalidRefreshTTL     = errors.New("invalid refresh token ttl")
	ErrInvalidToken          = errors.New("invalid token")
	ErrInvalidTokenSignature = errors.New("invalid token signature")
	ErrTokenExpired          = errors.New("token expired")
	ErrTokenClaimsMissing    = errors.New("token claims missing")
	ErrRefreshTokenEmpty     = errors.New("refresh token is empty")
	ErrRefreshTokenHashEmpty = errors.New("refresh token hash is empty")
)

type TokenServiceConfig struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	JWTSecret       string
}

type AccessTokenClaims struct {
	Subject   string
	SessionID string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type timeProvider interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

type TokenService struct {
	config TokenServiceConfig
	clock  timeProvider
}

func NewTokenService(config TokenServiceConfig, clock timeProvider) (*TokenService, error) {
	if strings.TrimSpace(config.JWTSecret) == "" {
		return nil, ErrJWTSecretRequired
	}
	if config.AccessTokenTTL < minAccessTokenTTL || config.AccessTokenTTL > maxAccessTokenTTL {
		return nil, ErrInvalidAccessTokenTTL
	}
	if config.RefreshTokenTTL <= 0 {
		return nil, ErrInvalidRefreshTTL
	}
	if clock == nil {
		clock = systemClock{}
	}

	return &TokenService{
		config: config,
		clock:  clock,
	}, nil
}

func (s *TokenService) IssueAccessToken(userID shared.UserID, sessionID shared.SessionID) (string, error) {
	now := s.clock.Now().UTC()
	payload := accessTokenPayload{
		Subject:   string(userID),
		SessionID: string(sessionID),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.config.AccessTokenTTL).Unix(),
	}

	headerJSON, err := json.Marshal(jwtHeader{
		Alg: "HS256",
		Typ: "JWT",
	})
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal jwt payload: %w", err)
	}

	headerPart := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerPart + "." + payloadPart

	signature := s.signJWT(signingInput)
	signaturePart := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signaturePart, nil
}

func (s *TokenService) VerifyAccessToken(token string) (AccessTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return AccessTokenClaims{}, ErrInvalidToken
	}

	var header jwtHeader
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		return AccessTokenClaims{}, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSignature := s.signJWT(signingInput)

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if subtle.ConstantTimeCompare(expectedSignature, signature) != 1 {
		return AccessTokenClaims{}, ErrInvalidTokenSignature
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}

	var payload accessTokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if payload.Subject == "" || payload.SessionID == "" || payload.IssuedAt == 0 || payload.ExpiresAt == 0 {
		return AccessTokenClaims{}, ErrTokenClaimsMissing
	}

	now := s.clock.Now().UTC().Unix()
	if now >= payload.ExpiresAt {
		return AccessTokenClaims{}, ErrTokenExpired
	}

	return AccessTokenClaims{
		Subject:   payload.Subject,
		SessionID: payload.SessionID,
		IssuedAt:  time.Unix(payload.IssuedAt, 0).UTC(),
		ExpiresAt: time.Unix(payload.ExpiresAt, 0).UTC(),
	}, nil
}

func (s *TokenService) VerifyAccessTokenIdentity(token string) (shared.UserID, shared.SessionID, error) {
	claims, err := s.VerifyAccessToken(token)
	if err != nil {
		return "", "", err
	}

	return shared.UserID(claims.Subject), shared.SessionID(claims.SessionID), nil
}

func (s *TokenService) IssueRefreshToken() (token string, hash string, expiresAt time.Time, err error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}

	token = base64.RawURLEncoding.EncodeToString(rawToken)
	hash, err = s.HashRefreshToken(token)
	if err != nil {
		return "", "", time.Time{}, err
	}

	return token, hash, s.clock.Now().UTC().Add(s.config.RefreshTokenTTL), nil
}

func (s *TokenService) HashRefreshToken(refreshToken string) (string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", ErrRefreshTokenEmpty
	}

	digest := sha256.Sum256([]byte(refreshToken))
	return hex.EncodeToString(digest[:]), nil
}

func (s *TokenService) VerifyRefreshTokenHash(refreshToken string, expectedHash string) (bool, error) {
	if strings.TrimSpace(expectedHash) == "" {
		return false, ErrRefreshTokenHashEmpty
	}

	computedHash, err := s.HashRefreshToken(refreshToken)
	if err != nil {
		return false, err
	}

	if subtle.ConstantTimeCompare([]byte(computedHash), []byte(expectedHash)) == 1 {
		return true, nil
	}

	return false, nil
}

func (s *TokenService) signJWT(signingInput string) []byte {
	mac := hmac.New(sha256.New, []byte(s.config.JWTSecret))
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

func (s *TokenService) AccessTokenTTL() time.Duration {
	return s.config.AccessTokenTTL
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type accessTokenPayload struct {
	Subject   string `json:"sub"`
	SessionID string `json:"session_id"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}
