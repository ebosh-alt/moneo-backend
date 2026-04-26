package http

import "github.com/gin-gonic/gin"

func NewRouter(authHandler *AuthHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.POST("/auth/logout", authHandler.Logout)
	router.POST("/auth/logout-all", authHandler.LogoutAll)

	return router
}
