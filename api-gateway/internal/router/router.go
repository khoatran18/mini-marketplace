package router

import (
	"ProjectPhase2/api-gateway/internal/config"
	"ProjectPhase2/api-gateway/internal/handler"
	"ProjectPhase2/api-gateway/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
)

func SetupRouter(router *gin.Engine, serviceConfig *config.ServiceConfig) {

	router.Use(middleware.RequestLoggingMiddleware(serviceConfig.ZapLogger))
	router.Use(middleware.RateLimitingMiddleware(100, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))
	router.POST("/login", handler.Login)

	protectedAPI := router.Group("/protected")
	protectedAPI.Use(
		middleware.AuthMiddleware(serviceConfig.ZapLogger),
		middleware.AuthorizationMiddleware([]string{"user"}, serviceConfig.ZapLogger),
	)
	{
		protectedAPI.GET("/profile", handler.ProtectedEndpoint)
	}
}
