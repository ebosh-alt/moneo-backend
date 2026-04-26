package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	appidentity "moneo/internal/app/identity"
	domainidentity "moneo/internal/domain/identity"

	"github.com/gin-gonic/gin"
)

const (
	RefreshCookieName          = "refresh_token"
	refreshCookiePath          = "/auth/refresh"
	refreshCookieMaxAgeSeconds = 2_592_000
)

type AuthUseCase interface {
	Register(ctx context.Context, input appidentity.RegisterInput) (appidentity.AuthTokens, error)
	Login(ctx context.Context, input appidentity.LoginInput) (appidentity.AuthTokens, error)
	Refresh(ctx context.Context, input appidentity.RefreshInput) (appidentity.AuthTokens, error)
	Logout(ctx context.Context, input appidentity.LogoutInput) error
	LogoutAll(ctx context.Context, input appidentity.LogoutAllInput) error
}

type authServiceAdapter struct {
	service *appidentity.AuthFlowService
}

func (a authServiceAdapter) Register(ctx context.Context, input appidentity.RegisterInput) (appidentity.AuthTokens, error) {
	return a.service.Register(ctx, input)
}

func (a authServiceAdapter) Login(ctx context.Context, input appidentity.LoginInput) (appidentity.AuthTokens, error) {
	return a.service.Login(ctx, input)
}

func (a authServiceAdapter) Refresh(ctx context.Context, input appidentity.RefreshInput) (appidentity.AuthTokens, error) {
	return a.service.Refresh(ctx, input)
}

func (a authServiceAdapter) Logout(ctx context.Context, input appidentity.LogoutInput) error {
	return a.service.Logout(ctx, input)
}

func (a authServiceAdapter) LogoutAll(ctx context.Context, input appidentity.LogoutAllInput) error {
	return a.service.LogoutAll(ctx, input)
}

type AuthHandler struct {
	auth AuthUseCase
}

func NewAuthHandler(authService *appidentity.AuthFlowService) *AuthHandler {
	return &AuthHandler{
		auth: authServiceAdapter{service: authService},
	}
}

type registerRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	tokens, err := h.auth.Register(c.Request.Context(), appidentity.RegisterInput{
		Email:           request.Email,
		Password:        request.Password,
		PasswordConfirm: request.PasswordConfirm,
	})
	if err != nil {
		h.writeAuthError(c, err)
		return
	}

	setRefreshCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusCreated, authResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   tokens.ExpiresIn,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	tokens, err := h.auth.Login(c.Request.Context(), appidentity.LoginInput{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		h.writeAuthError(c, err)
		return
	}

	setRefreshCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusOK, authResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   tokens.ExpiresIn,
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, fromCookie, err := extractRefreshToken(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}
	if strings.TrimSpace(refreshToken) == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_refresh_token"})
		return
	}

	tokens, err := h.auth.Refresh(c.Request.Context(), appidentity.RefreshInput{
		RefreshToken: refreshToken,
	})
	if err != nil {
		h.writeAuthError(c, err)
		return
	}

	if fromCookie {
		setRefreshCookie(c, refreshToken)
	}

	c.JSON(http.StatusOK, authResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   tokens.ExpiresIn,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, _, err := extractRefreshToken(c)
	if err != nil {
		clearRefreshCookie(c)
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	if strings.TrimSpace(refreshToken) != "" {
		if err := h.auth.Logout(c.Request.Context(), appidentity.LogoutInput{
			RefreshToken: refreshToken,
		}); err != nil && !errors.Is(err, appidentity.ErrInvalidRefreshToken) {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
			return
		}
	}

	clearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) LogoutAll(c *gin.Context) {
	accessToken := parseBearerToken(c.GetHeader("Authorization"))
	if strings.TrimSpace(accessToken) == "" {
		clearRefreshCookie(c)
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
		return
	}

	if err := h.auth.LogoutAll(c.Request.Context(), appidentity.LogoutAllInput{
		AccessToken: accessToken,
	}); err != nil {
		h.writeAuthError(c, err)
		return
	}

	clearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appidentity.ErrEmailAlreadyRegistered):
		c.JSON(http.StatusConflict, errorResponse{Error: "duplicate_email"})
	case errors.Is(err, appidentity.ErrInvalidRefreshToken):
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_refresh_token"})
	case errors.Is(err, appidentity.ErrInvalidAccessToken):
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
	case errors.Is(err, appidentity.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_credentials"})
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
	default:
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
	}
}

func setRefreshCookie(c *gin.Context, refreshToken string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		RefreshCookieName,
		refreshToken,
		refreshCookieMaxAgeSeconds,
		refreshCookiePath,
		"",
		true,
		true,
	)
}

func clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		RefreshCookieName,
		"",
		-1,
		refreshCookiePath,
		"",
		true,
		true,
	)
}

func parseBearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func extractRefreshToken(c *gin.Context) (refreshToken string, fromCookie bool, err error) {
	if value, cookieErr := c.Cookie(RefreshCookieName); cookieErr == nil && strings.TrimSpace(value) != "" {
		return value, true, nil
	}

	payload, readErr := io.ReadAll(c.Request.Body)
	if readErr != nil {
		return "", false, readErr
	}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return "", false, nil
	}

	var request struct {
		RefreshToken string `json:"refresh_token"`
	}
	if unmarshalErr := json.Unmarshal(payload, &request); unmarshalErr != nil {
		return "", false, unmarshalErr
	}

	return request.RefreshToken, false, nil
}
