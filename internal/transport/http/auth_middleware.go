package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	appidentity "moneo/internal/app/identity"
	domainidentity "moneo/internal/domain/identity"

	"github.com/gin-gonic/gin"
)

const (
	authUserContextKey    = "auth.user"
	authSessionContextKey = "auth.session"
)

type AccessAuthenticator interface {
	Authenticate(ctx context.Context, accessToken string) (domainidentity.User, domainidentity.Session, error)
}

func NewAuthMiddleware(authenticator AccessAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticator == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
			return
		}

		accessToken := parseBearerToken(c.GetHeader("Authorization"))
		if strings.TrimSpace(accessToken) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
			return
		}

		user, session, err := authenticator.Authenticate(c.Request.Context(), accessToken)
		if err != nil {
			if errors.Is(err, appidentity.ErrInvalidAccessToken) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse{Error: "invalid_access_token"})
				return
			}

			c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
			return
		}

		c.Set(authUserContextKey, user)
		c.Set(authSessionContextKey, session)
		c.Next()
	}
}

func UserFromContext(c *gin.Context) (domainidentity.User, bool) {
	value, ok := c.Get(authUserContextKey)
	if !ok {
		return domainidentity.User{}, false
	}

	user, ok := value.(domainidentity.User)
	if !ok {
		return domainidentity.User{}, false
	}

	return user, true
}

func SessionFromContext(c *gin.Context) (domainidentity.Session, bool) {
	value, ok := c.Get(authSessionContextKey)
	if !ok {
		return domainidentity.Session{}, false
	}

	session, ok := value.(domainidentity.Session)
	if !ok {
		return domainidentity.Session{}, false
	}

	return session, true
}
