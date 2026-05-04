package http

import (
	"context"
	"fmt"
	"net/http"

	"moneo/internal/domain/shared"

	"github.com/gin-gonic/gin"
)

func ginContextFromContext(ctx context.Context) (*gin.Context, error) {
	ginCtx, ok := ctx.(*gin.Context)
	if !ok || ginCtx == nil {
		return nil, fmt.Errorf("gin context is required")
	}
	return ginCtx, nil
}

func userIDFromStrictContext(ctx context.Context) (shared.UserID, bool) {
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return "", false
	}
	user, ok := UserFromContext(ginCtx)
	if !ok {
		return "", false
	}
	return user.ID, true
}

func propagateSetCookies(from http.Header, to http.Header) {
	for _, cookie := range from.Values("Set-Cookie") {
		to.Add("Set-Cookie", cookie)
	}
}

func optionalPtr[T any](v T) *T {
	return &v
}
