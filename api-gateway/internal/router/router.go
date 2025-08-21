package router

import (
	"ProjectPhase2/api-gateway/internal/config"
	"ProjectPhase2/api-gateway/internal/handler"
	"ProjectPhase2/api-gateway/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
)

// SetupRouter setup middleware, router for engine
func SetupRouter(router *gin.Engine, serviceConfig *config.ServiceConfig, envConfig *config.EnvConfig) {

	router.Use(middleware.RequestLoggingMiddleware(serviceConfig.ZapLogger))
	router.Use(middleware.RateLimitingMiddleware(100, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))
	router.POST("/login", handler.Login)

	protectedAPI := router.Group("/protected")
	protectedAPI.Use(
		middleware.AuthMiddleware(serviceConfig.ZapLogger, envConfig.JWTSecret),
		middleware.AuthorizationMiddleware([]string{"user"}, serviceConfig.ZapLogger),
	)
	{
		protectedAPI.GET("/profile", handler.ProtectedEndpoint)
	}
}
