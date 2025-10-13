package router

import (
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter setup middleware, router for engine
func SetupRouter(router *gin.Engine, h *handler.ManagerHandler, serviceConfig *config.ServiceConfig, envConfig *config.EnvConfig) {

	router.Use(cors.New(cors.Config{
		AllowAllOrigins:  true, // hoặc AllowOrigins: []string{"https://frontend.example.com"}
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.Use(middleware.RequestLoggingMiddleware(serviceConfig.ZapLogger))
	// router.Use(middleware.RateLimitingMiddleware(100, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))

	authRoute := router.Group("/auth")
	{
		authRoute.POST("/login", h.AuthHandler.Login)
		authRoute.POST("/register", h.AuthHandler.Register)
		authRoute.POST("/change-password", h.AuthHandler.ChangePassword)
		authRoute.POST("/refresh-token", h.AuthHandler.RefreshToken)
		authRoute.POST("/register-seller-roles", middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret),
			middleware.AuthorizationMiddleware([]string{"seller_admin"}, serviceConfig.ZapLogger),
			h.AuthHandler.RegisterSellerRoles)
	}

	router.Use(middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret))
	userRoute := router.Group("/users")
	{
		//userRoute.Use(middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret))
		buyerRoute := userRoute.Group("/buyers")
		{
			buyerRoute.Use(middleware.AuthorizationMiddleware([]string{"buyer"}, serviceConfig.ZapLogger))
			buyerRoute.POST("", h.UserHandler.CreateBuyer)
			buyerRoute.GET("/:id", h.UserHandler.GetBuyerByUserID)
			buyerRoute.PUT("/:id", h.UserHandler.UpdateBuyerByUserID)
			buyerRoute.DELETE("/:id", h.UserHandler.DelBuyerByUserID)
		}
		sellerRoute := userRoute.Group("/sellers")
		{
			sellerRoute.POST("", middleware.AuthorizationMiddleware([]string{"seller_admin"}, serviceConfig.ZapLogger), h.UserHandler.CreateSeller)
			sellerRoute.GET("/:id", h.UserHandler.GetSellerByID)
			sellerRoute.PUT("/:id", middleware.AuthorizationMiddleware([]string{"seller_admin"}, serviceConfig.ZapLogger), h.UserHandler.UpdateSellerByID)
			sellerRoute.DELETE("/:id", middleware.AuthorizationMiddleware([]string{"seller_admin"}, serviceConfig.ZapLogger), h.UserHandler.DelSellerByID)
		}
	}

	productRoute := router.Group("/products")
	{
		productRoute.POST("", middleware.AuthorizationMiddleware([]string{"seller_admin", "seller_employee"}, serviceConfig.ZapLogger), h.ProductHandler.CreateProduct)
		productRoute.PUT("/:id", middleware.AuthorizationMiddleware([]string{"seller_admin", "seller_employee"}, serviceConfig.ZapLogger), h.ProductHandler.UpdateProduct)
		productRoute.GET("/:id", h.ProductHandler.GetProductByID)
		productRoute.GET("", h.ProductHandler.GetProducts)
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
