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

	authRoute := router.Group("/auth")
	{
		authRoute.POST("/login", h.AuthHandler.Login)
		authRoute.POST("/register", h.AuthHandler.Register)
		authRoute.POST("/change-password", h.AuthHandler.ChangePassword)
		authRoute.POST("/refresh-token", h.AuthHandler.RefreshToken)
		authRoute.POST("/register-seller-roles", h.AuthHandler.RegisterSellerRoles)
	}

	userRoute := router.Group("/users")
	{
		buyerRoute := userRoute.Group("/buyers")
		{
			buyerRoute.POST("", h.UserHandler.CreateBuyer)
			buyerRoute.GET("/:id", h.UserHandler.GetBuyerByUserID)
			buyerRoute.PUT("/:id", h.UserHandler.UpdateBuyerByUserID)
			buyerRoute.DELETE("/:id", h.UserHandler.DelBuyerByUserID)
		}
		sellerRoute := userRoute.Group("/sellers")
		{
			sellerRoute.POST("", h.UserHandler.CreateSeller)
			sellerRoute.GET("/:id", h.UserHandler.GetSellerByID)
			sellerRoute.PUT("/:id", h.UserHandler.UpdateSellerByID)
			sellerRoute.DELETE("/:id", h.UserHandler.DelSellerByID)
		}
	}

	productRoute := router.Group("/products")
	{
		productRoute.POST("", h.ProductHandler.CreateProduct)
		productRoute.PUT("/:id", h.ProductHandler.UpdateProduct)
		productRoute.GET("/:id", h.ProductHandler.GetProductByID)
		productRoute.GET("/seller/:seller_id", h.ProductHandler.GetProductsBySellerID)
	}

	orderRoute := router.Group("/orders")
	{
		orderRoute.POST("", h.OrderHandler.CreateOrder)
		orderRoute.GET("/:id", h.OrderHandler.GetOrderByID)
		orderRoute.PUT("/:id", h.OrderHandler.UpdateOrderByID)
		orderRoute.GET("", h.OrderHandler.GetOrdersByBuyerIDStatus) // ?buyer_id={buyer_id}&status={status}
		orderRoute.DELETE("/:id", h.OrderHandler.CancelOrderByID)
	}

}
