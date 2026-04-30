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
			apiV1 := router.Group("/api/v1", options.AuthMiddleware)
			registerCatalogRoutes(apiV1, options.CatalogHandler)

			legacy := router.Group("/", options.AuthMiddleware)
			registerCatalogRoutes(legacy, options.CatalogHandler)
		}
	} else {
		router.GET("/auth/me", authHandler.Me)
		router.GET("/auth/sessions", authHandler.Sessions)
		router.DELETE("/auth/sessions/:sessionId", authHandler.RevokeSession)
		router.POST("/auth/send-verification-email", authHandler.SendVerificationEmail)

		if options.CatalogHandler != nil {
			apiV1 := router.Group("/api/v1")
			registerCatalogRoutes(apiV1, options.CatalogHandler)

			registerCatalogRoutes(router, options.CatalogHandler)
		}
	}

	return router
}

func registerCatalogRoutes(routes gin.IRoutes, handler *CatalogHandler) {
	routes.POST("/accounts", handler.CreateAccount)
	routes.GET("/accounts", handler.ListAccounts)
	routes.GET("/accounts/summary", handler.GetAccountsSummary)
	routes.POST("/accounts/:accountId/archive", handler.ArchiveAccount)
	routes.POST("/accounts/:accountId/restore", handler.RestoreAccount)
	routes.GET("/accounts/:accountId", handler.GetAccount)
	routes.PATCH("/accounts/:accountId", handler.PatchAccount)
	routes.POST("/categories", handler.CreateCategory)
	routes.GET("/categories", handler.ListCategories)
	routes.POST("/categories/:categoryId/restore", handler.RestoreCategory)
	routes.PATCH("/categories/:categoryId", handler.PatchCategory)
	routes.DELETE("/categories/:categoryId", handler.DeleteCategory)
	routes.POST("/categories/:categoryId/subcategories", handler.CreateSubcategory)
	routes.GET("/categories/:categoryId/subcategories", handler.ListCategorySubcategories)
	routes.GET("/categories/:categoryId", handler.GetCategory)
	routes.POST("/subcategories/:subcategoryId/restore", handler.RestoreSubcategory)
	routes.PATCH("/subcategories/:subcategoryId", handler.PatchSubcategory)
	routes.DELETE("/subcategories/:subcategoryId", handler.DeleteSubcategory)
	routes.GET("/subcategories", handler.ListSubcategories)
	routes.GET("/subcategories/:subcategoryId", handler.GetSubcategory)
	routes.POST("/transactions", handler.CreateTransaction)
	routes.GET("/transactions", handler.ListTransactions)
	routes.GET("/transactions/:transactionId", handler.GetTransaction)
	routes.PATCH("/transactions/:transactionId", handler.PatchTransaction)
	routes.DELETE("/transactions/:transactionId", handler.DeleteTransaction)
	routes.POST("/transactions/:transactionId/post", handler.PostTransaction)
	routes.POST("/transactions/:transactionId/cancel", handler.CancelTransaction)
	routes.POST("/transactions/:transactionId/duplicate", handler.DuplicateTransaction)
}
