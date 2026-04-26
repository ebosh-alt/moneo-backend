package http

import "github.com/gin-gonic/gin"

func NewRouter(authHandler *AuthHandler, authMiddleware ...gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.POST("/auth/logout", authHandler.Logout)
	router.POST("/auth/logout-all", authHandler.LogoutAll)

	if len(authMiddleware) > 0 && authMiddleware[0] != nil {
		protectedAuth := router.Group("/auth", authMiddleware[0])
		protectedAuth.GET("/me", authHandler.Me)
		protectedAuth.GET("/sessions", authHandler.Sessions)
		protectedAuth.DELETE("/sessions/:sessionId", authHandler.RevokeSession)
	} else {
		router.GET("/auth/me", authHandler.Me)
		router.GET("/auth/sessions", authHandler.Sessions)
		router.DELETE("/auth/sessions/:sessionId", authHandler.RevokeSession)
	}

	return router
}
