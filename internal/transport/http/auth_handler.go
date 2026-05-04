package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	appidentity "moneo/internal/app/identity"
	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	RefreshCookieName          = "refresh_token"
	refreshCookiePath          = "/api/v1/auth/refresh"
	refreshCookieMaxAgeSeconds = 2_592_000
	maxAuthJSONBodyBytes       = 8 * 1024
	maxEmailLength             = 320
	maxPasswordLength          = 1024
	maxRefreshTokenLength      = 4096
	maxOneTimeTokenLength      = 4096
)

type AuthUseCase interface {
	Register(ctx context.Context, input appidentity.RegisterInput) (appidentity.AuthTokens, error)
	Login(ctx context.Context, input appidentity.LoginInput) (appidentity.AuthTokens, error)
	Refresh(ctx context.Context, input appidentity.RefreshInput) (appidentity.AuthTokens, error)
	Logout(ctx context.Context, input appidentity.LogoutInput) error
	LogoutAll(ctx context.Context, input appidentity.LogoutAllInput) error
	LogoutCurrent(ctx context.Context, input appidentity.LogoutCurrentInput) error
	ListActiveSessions(ctx context.Context, input appidentity.ListSessionsInput) ([]appidentity.UserSessionView, error)
	RevokeSession(ctx context.Context, input appidentity.RevokeSessionInput) error
}

type AuthPostMVPUseCase interface {
	ForgotPassword(ctx context.Context, input appidentity.ForgotPasswordInput) error
	ResetPassword(ctx context.Context, input appidentity.ResetPasswordInput) error
	SendVerificationEmail(ctx context.Context, input appidentity.SendVerificationEmailInput) error
	VerifyEmail(ctx context.Context, input appidentity.VerifyEmailInput) error
}

type authServiceAdapter struct {
	service *appidentity.AuthFlowService
}

type authPostMVPServiceAdapter struct {
	service *appidentity.AuthPostMVPService
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

func (a authServiceAdapter) LogoutCurrent(ctx context.Context, input appidentity.LogoutCurrentInput) error {
	return a.service.LogoutCurrent(ctx, input)
}

func (a authServiceAdapter) ListActiveSessions(ctx context.Context, input appidentity.ListSessionsInput) ([]appidentity.UserSessionView, error) {
	return a.service.ListActiveSessions(ctx, input)
}

func (a authServiceAdapter) RevokeSession(ctx context.Context, input appidentity.RevokeSessionInput) error {
	return a.service.RevokeSession(ctx, input)
}

func (a authPostMVPServiceAdapter) ForgotPassword(ctx context.Context, input appidentity.ForgotPasswordInput) error {
	return a.service.ForgotPassword(ctx, input)
}

func (a authPostMVPServiceAdapter) ResetPassword(ctx context.Context, input appidentity.ResetPasswordInput) error {
	return a.service.ResetPassword(ctx, input)
}

func (a authPostMVPServiceAdapter) SendVerificationEmail(ctx context.Context, input appidentity.SendVerificationEmailInput) error {
	return a.service.SendVerificationEmail(ctx, input)
}

func (a authPostMVPServiceAdapter) VerifyEmail(ctx context.Context, input appidentity.VerifyEmailInput) error {
	return a.service.VerifyEmail(ctx, input)
}

type AuthHandler struct {
	auth    AuthUseCase
	postMVP AuthPostMVPUseCase
}

func NewAuthHandler(authService *appidentity.AuthFlowService, postMVPService ...*appidentity.AuthPostMVPService) *AuthHandler {
	handler := &AuthHandler{
		auth: authServiceAdapter{service: authService},
	}
	if len(postMVPService) > 0 && postMVPService[0] != nil {
		handler.postMVP = authPostMVPServiceAdapter{service: postMVPService[0]}
	}

	return handler
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

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token           string `json:"token"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type meResponse struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
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

func (h *AuthHandler) Register(c *gin.Context) {
	var request registerRequest
	if err := decodeStrictJSONBody(c, &request); err != nil || !validateRegisterRequest(request) {
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
	if err := decodeStrictJSONBody(c, &request); err != nil || !validateLoginRequest(request) {
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

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	if h.postMVP == nil {
		c.JSON(http.StatusNotImplemented, errorResponse{Error: "not_implemented"})
		return
	}

	var request forgotPasswordRequest
	if err := decodeStrictJSONBody(c, &request); err != nil || !validateForgotPasswordRequest(request) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	if err := h.postMVP.ForgotPassword(c.Request.Context(), appidentity.ForgotPasswordInput{
		Email: request.Email,
	}); err != nil {
		h.writePostMVPAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	if h.postMVP == nil {
		c.JSON(http.StatusNotImplemented, errorResponse{Error: "not_implemented"})
		return
	}

	var request resetPasswordRequest
	if err := decodeStrictJSONBody(c, &request); err != nil || !validateResetPasswordRequest(request) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	if err := h.postMVP.ResetPassword(c.Request.Context(), appidentity.ResetPasswordInput{
		Token:           request.Token,
		Password:        request.Password,
		PasswordConfirm: request.PasswordConfirm,
	}); err != nil {
		h.writePostMVPAuthError(c, err)
		return
	}

	clearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
	} else {
		accessToken := parseBearerToken(c.GetHeader("Authorization"))
		if strings.TrimSpace(accessToken) != "" {
			if err := h.auth.LogoutCurrent(c.Request.Context(), appidentity.LogoutCurrentInput{
				AccessToken: accessToken,
			}); err != nil && !errors.Is(err, appidentity.ErrInvalidAccessToken) {
				c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
				return
			}
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

func (h *AuthHandler) Me(c *gin.Context) {
	user, userOK := UserFromContext(c)
	_, sessionOK := SessionFromContext(c)
	if !userOK || !sessionOK {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
		return
	}

	c.JSON(http.StatusOK, meResponse{
		ID:            string(user.ID),
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt,
	})
}

func (h *AuthHandler) Sessions(c *gin.Context) {
	user, userOK := UserFromContext(c)
	_, sessionOK := SessionFromContext(c)
	if !userOK || !sessionOK {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
		return
	}

	sessions, err := h.auth.ListActiveSessions(c.Request.Context(), appidentity.ListSessionsInput{
		UserID: user.ID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
		return
	}

	response := make([]sessionResponse, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, sessionResponse{
			ID:         string(session.ID),
			UserAgent:  session.UserAgent,
			IP:         session.IP,
			DeviceName: session.DeviceName,
			CreatedAt:  session.CreatedAt,
			LastUsedAt: session.LastUsedAt,
			ExpiresAt:  session.ExpiresAt,
		})
	}

	c.JSON(http.StatusOK, sessionsResponse{Sessions: response})
}

func (h *AuthHandler) RevokeSession(c *gin.Context) {
	user, userOK := UserFromContext(c)
	_, sessionOK := SessionFromContext(c)
	if !userOK || !sessionOK {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
		return
	}

	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" || !isUUID(sessionID) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	err := h.auth.RevokeSession(c.Request.Context(), appidentity.RevokeSessionInput{
		UserID:    user.ID,
		SessionID: shared.SessionID(sessionID),
	})
	if err != nil {
		if errors.Is(err, appidentity.ErrSessionNotFound) {
			c.JSON(http.StatusNotFound, errorResponse{Error: "session_not_found"})
			return
		}

		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) SendVerificationEmail(c *gin.Context) {
	if h.postMVP == nil {
		c.JSON(http.StatusNotImplemented, errorResponse{Error: "not_implemented"})
		return
	}

	user, userOK := UserFromContext(c)
	_, sessionOK := SessionFromContext(c)
	if !userOK || !sessionOK {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
		return
	}

	if err := h.postMVP.SendVerificationEmail(c.Request.Context(), appidentity.SendVerificationEmailInput{
		UserID: user.ID,
	}); err != nil {
		h.writePostMVPAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	if h.postMVP == nil {
		c.JSON(http.StatusNotImplemented, errorResponse{Error: "not_implemented"})
		return
	}

	var request verifyEmailRequest
	if err := decodeStrictJSONBody(c, &request); err != nil || !validateVerifyEmailRequest(request) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}

	if err := h.postMVP.VerifyEmail(c.Request.Context(), appidentity.VerifyEmailInput{
		Token: request.Token,
	}); err != nil {
		h.writePostMVPAuthError(c, err)
		return
	}

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

func (h *AuthHandler) writePostMVPAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appidentity.ErrInvalidPasswordResetToken):
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_reset_token"})
	case errors.Is(err, appidentity.ErrInvalidEmailVerificationToken):
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_verification_token"})
	case errors.Is(err, appidentity.ErrInvalidAccessToken):
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
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

	payload, readErr := readRequestPayload(c.Request.Body, maxAuthJSONBodyBytes)
	if readErr != nil {
		return "", false, readErr
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return "", false, nil
	}

	var request struct {
		RefreshToken string `json:"refresh_token"`
	}
	if unmarshalErr := decodeStrictJSONPayload(payload, &request); unmarshalErr != nil {
		return "", false, unmarshalErr
	}
	if len(request.RefreshToken) > maxRefreshTokenLength {
		return "", false, errors.New("refresh token is too long")
	}

	return request.RefreshToken, false, nil
}

func validateRegisterRequest(request registerRequest) bool {
	email := strings.TrimSpace(request.Email)
	if email == "" || len(email) > maxEmailLength {
		return false
	}
	if len(request.Password) == 0 || len(request.Password) > maxPasswordLength {
		return false
	}
	if len(request.PasswordConfirm) == 0 || len(request.PasswordConfirm) > maxPasswordLength {
		return false
	}

	return true
}

func validateLoginRequest(request loginRequest) bool {
	email := strings.TrimSpace(request.Email)
	if email == "" || len(email) > maxEmailLength {
		return false
	}
	if len(request.Password) == 0 || len(request.Password) > maxPasswordLength {
		return false
	}

	return true
}

func validateForgotPasswordRequest(request forgotPasswordRequest) bool {
	email := strings.TrimSpace(request.Email)
	return email != "" && len(email) <= maxEmailLength
}

func validateResetPasswordRequest(request resetPasswordRequest) bool {
	if strings.TrimSpace(request.Token) == "" || len(request.Token) > maxOneTimeTokenLength {
		return false
	}
	if len(request.Password) == 0 || len(request.Password) > maxPasswordLength {
		return false
	}
	if len(request.PasswordConfirm) == 0 || len(request.PasswordConfirm) > maxPasswordLength {
		return false
	}

	return true
}

func validateVerifyEmailRequest(request verifyEmailRequest) bool {
	return strings.TrimSpace(request.Token) != "" && len(request.Token) <= maxOneTimeTokenLength
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func decodeStrictJSONBody(c *gin.Context, target any) error {
	payload, err := readRequestPayload(c.Request.Body, maxAuthJSONBodyBytes)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return io.EOF
	}

	return decodeStrictJSONPayload(payload, target)
}

func readRequestPayload(body io.Reader, maxSize int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	limited := io.LimitReader(body, maxSize+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxSize {
		return nil, errors.New("request payload is too large")
	}

	return payload, nil
}

func decodeStrictJSONPayload(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("invalid json payload")
	}

	return nil
}
