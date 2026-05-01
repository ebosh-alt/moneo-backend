package http

import (
	"moneo/internal/transport/http/generated"

	"github.com/gin-gonic/gin"
)

type RouterOptions struct {
	AuthMiddleware     gin.HandlerFunc
	SecurityMiddleware gin.HandlerFunc
	StrictAPIHandler   generated.StrictServerInterface
}

func NewRouter(authHandler *AuthHandler, authMiddleware ...gin.HandlerFunc) *gin.Engine {
	options := RouterOptions{}
	if len(authMiddleware) > 0 {
		options.AuthMiddleware = authMiddleware[0]
	}

	return NewRouterWithOptions(authHandler, options)
}

func NewRouterWithOptions(authHandler *AuthHandler, options RouterOptions) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	securityMiddleware := options.SecurityMiddleware
	if securityMiddleware == nil {
		securityMiddleware = NewAuthSecurityMiddleware(DefaultAuthSecurityConfig())
	}
	if securityMiddleware != nil {
		router.Use(securityMiddleware)
	}

	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/forgot-password", authHandler.ForgotPassword)
	router.POST("/auth/reset-password", authHandler.ResetPassword)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.POST("/auth/logout", authHandler.Logout)
	router.POST("/auth/logout-all", authHandler.LogoutAll)
	router.POST("/auth/verify-email", authHandler.VerifyEmail)

	if options.AuthMiddleware != nil {
		protectedAuth := router.Group("/auth", options.AuthMiddleware)
		protectedAuth.GET("/me", authHandler.Me)
		protectedAuth.GET("/sessions", authHandler.Sessions)
		protectedAuth.DELETE("/sessions/:sessionId", authHandler.RevokeSession)
		protectedAuth.POST("/send-verification-email", authHandler.SendVerificationEmail)

		if options.StrictAPIHandler != nil {
			protectedAPI := router.Group("/", options.AuthMiddleware)
			generated.RegisterHandlers(protectedAPI, generated.NewStrictHandler(options.StrictAPIHandler, nil))
		}
	} else {
		router.GET("/auth/me", authHandler.Me)
		router.GET("/auth/sessions", authHandler.Sessions)
		router.DELETE("/auth/sessions/:sessionId", authHandler.RevokeSession)
		router.POST("/auth/send-verification-email", authHandler.SendVerificationEmail)

		if options.StrictAPIHandler != nil {
			generated.RegisterHandlers(router, generated.NewStrictHandler(options.StrictAPIHandler, nil))
		}
	}

	return router
}
