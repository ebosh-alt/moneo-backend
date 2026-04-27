package http

import "github.com/gin-gonic/gin"

type RouterOptions struct {
	AuthMiddleware     gin.HandlerFunc
	SecurityMiddleware gin.HandlerFunc
	CatalogHandler     *CatalogHandler
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

		if options.CatalogHandler != nil {
			protectedCatalog := router.Group("/", options.AuthMiddleware)
			protectedCatalog.POST("/accounts", options.CatalogHandler.CreateAccount)
			protectedCatalog.GET("/accounts", options.CatalogHandler.ListAccounts)
			protectedCatalog.GET("/accounts/:accountId", options.CatalogHandler.GetAccount)
			protectedCatalog.GET("/categories", options.CatalogHandler.ListCategories)
			protectedCatalog.GET("/categories/:categoryId", options.CatalogHandler.GetCategory)
			protectedCatalog.GET("/subcategories", options.CatalogHandler.ListSubcategories)
			protectedCatalog.GET("/subcategories/:subcategoryId", options.CatalogHandler.GetSubcategory)
		}
	} else {
		router.GET("/auth/me", authHandler.Me)
		router.GET("/auth/sessions", authHandler.Sessions)
		router.DELETE("/auth/sessions/:sessionId", authHandler.RevokeSession)
		router.POST("/auth/send-verification-email", authHandler.SendVerificationEmail)

		if options.CatalogHandler != nil {
			router.POST("/accounts", options.CatalogHandler.CreateAccount)
			router.GET("/accounts", options.CatalogHandler.ListAccounts)
			router.GET("/accounts/:accountId", options.CatalogHandler.GetAccount)
			router.GET("/categories", options.CatalogHandler.ListCategories)
			router.GET("/categories/:categoryId", options.CatalogHandler.GetCategory)
			router.GET("/subcategories", options.CatalogHandler.ListSubcategories)
			router.GET("/subcategories/:subcategoryId", options.CatalogHandler.GetSubcategory)
		}
	}

	return router
}
