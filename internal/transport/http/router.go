package http

import (
	"context"
	"fmt"
	"moneo/internal/transport/http/generated"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

type RouterOptions struct {
	AuthMiddleware     gin.HandlerFunc
	SecurityMiddleware gin.HandlerFunc
	StrictAPIHandler   generated.StrictServerInterface
}

func NewRouter(authMiddleware ...gin.HandlerFunc) *gin.Engine {
	options := RouterOptions{}
	if len(authMiddleware) > 0 {
		options.AuthMiddleware = authMiddleware[0]
	}

	return NewRouterWithOptions(options)
}

func NewRouterWithOptions(options RouterOptions) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	securityMiddleware := options.SecurityMiddleware
	if securityMiddleware == nil {
		securityMiddleware = NewAuthSecurityMiddleware(DefaultAuthSecurityConfig())
	}
	if securityMiddleware != nil {
		router.Use(securityMiddleware)
	}

	if options.StrictAPIHandler == nil {
		panic("strict api handler is required")
	}

	if options.AuthMiddleware != nil {
		protectedAPI := router.Group("/", options.AuthMiddleware)
		registerStrictHandlers(protectedAPI, options.StrictAPIHandler)
	} else {
		publicAPI := router.Group("/")
		registerStrictHandlers(publicAPI, options.StrictAPIHandler)
	}

	return router
}

func registerStrictHandlers(routes gin.IRouter, handler generated.StrictServerInterface) {
	swagger, err := generated.GetSwagger()
	if err != nil {
		panic(fmt.Errorf("load embedded swagger: %w", err))
	}
	// Validator performs host/server checks. We don't want to bind runtime host here.
	swagger.Servers = nil

	routes.Use(ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
		ErrorHandler: writeOpenAPIValidationError,
		Options: openapi3filter.Options{
			AuthenticationFunc: func(_ context.Context, _ *openapi3filter.AuthenticationInput) error {
				// Auth is handled by our gin auth middleware before transport handlers.
				return nil
			},
		},
	}))
	routes.Use(recoverEmptyBadRequestBody())
	routes.Use(captureRawRequestBody())

	strict := generated.NewStrictHandler(handler, nil)
	generated.RegisterHandlersWithOptions(routes, strict, generated.GinServerOptions{
		ErrorHandler: func(c *gin.Context, err error, statusCode int) {
			writeOpenAPIValidationError(c, err.Error(), statusCode)
		},
	})
}
