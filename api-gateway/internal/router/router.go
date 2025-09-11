package router

import (
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
)

// SetupRouter setup middleware, router for engine
func SetupRouter(router *gin.Engine, h *handler.ManagerHandler, serviceConfig *config.ServiceConfig, envConfig *config.EnvConfig) {

	router.Use(middleware.RequestLoggingMiddleware(serviceConfig.ZapLogger))
	router.Use(middleware.RateLimitingMiddleware(100, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))
	router.POST("/auth/login", h.AuthHandler.Login)
	router.POST("/auth/register", h.AuthHandler.Register)
	router.POST("/auth/change-password", h.AuthHandler.ChangePassword)
	router.POST("/auth/refresh-token", h.AuthHandler.RefreshToken)
}
