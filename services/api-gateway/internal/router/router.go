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
		AllowOrigins:     envConfig.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.Use(middleware.RequestLoggingMiddleware(serviceConfig.ZapLogger))
	if envConfig.RateLimit > 0 {
		router.Use(middleware.RateLimitingMiddleware("global", envConfig.RateLimit, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))
	}

	liveness := func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) }
	router.GET("/health", liveness)
	router.GET("/healthz", liveness)

	authenticated := middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret)

	authRoute := router.Group("/auth")
	if envConfig.AuthRateLimit > 0 {
		// Stricter limit on credential endpoints to slow down brute force
		authRoute.Use(middleware.RateLimitingMiddleware("auth", envConfig.AuthRateLimit, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))
	}
	{
		authRoute.POST("/login", h.AuthHandler.Login)
		authRoute.POST("/register", h.AuthHandler.Register)
		authRoute.POST("/change-password", authenticated, h.AuthHandler.ChangePassword)
		authRoute.POST("/refresh-token", h.AuthHandler.RefreshToken)
		authRoute.POST("/register-seller-roles", middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret),
			middleware.AuthorizationMiddleware([]string{"seller_admin"}, serviceConfig.ZapLogger),
			h.AuthHandler.RegisterSellerRoles)
	}

	router.Use(authenticated)
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
		// Buyers only; handlers scope every query to the authenticated buyer
		orderRoute.Use(middleware.AuthorizationMiddleware([]string{"buyer"}, serviceConfig.ZapLogger))
		orderRoute.POST("", h.OrderHandler.CreateOrder)
		orderRoute.GET("/:id", h.OrderHandler.GetOrderByID)
		orderRoute.GET("", h.OrderHandler.GetOrdersByBuyerIDStatus) // ?status={status}
		orderRoute.DELETE("/:id", h.OrderHandler.CancelOrderByID)
	}

}
