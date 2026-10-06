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
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Idempotency-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	if envConfig.MaxBodyBytes > 0 {
		router.Use(middleware.BodyLimitMiddleware(envConfig.MaxBodyBytes))
	}
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

	// Public catalog: anonymous visitors see active products; a valid token adds owner/admin visibility
	optionalAuth := middleware.OptionalAuth(authenticated)
	sellerRoles := middleware.AuthorizationMiddleware([]string{"seller_admin", "seller_employee"}, serviceConfig.ZapLogger)
	router.GET("/search", optionalAuth, h.CatalogHandler.Search)
	router.GET("/categories", h.CatalogHandler.Categories)

	productRoute := router.Group("/products")
	{
		productRoute.POST("", authenticated, sellerRoles, h.ProductHandler.CreateProduct)
		productRoute.PUT("/:id", authenticated, sellerRoles, h.ProductHandler.UpdateProduct)
		productRoute.PATCH("/:id/status", authenticated, sellerRoles, h.CatalogHandler.SetStatus)
		productRoute.GET("/:id", optionalAuth, h.ProductHandler.GetProductByID)
		productRoute.GET("/:id/stock", h.CatalogHandler.Stock)
		productRoute.GET("", optionalAuth, h.ProductHandler.GetProducts)
		productRoute.GET("/seller/:seller_id", optionalAuth, h.ProductHandler.GetProductsBySellerID)
	}

	// Seller console
	sellerRoute := router.Group("/seller", authenticated, sellerRoles)
	{
		sellerRoute.GET("/products", h.CatalogHandler.SellerProducts)
		sellerRoute.POST("/products/:id/inventory/adjust", h.CatalogHandler.AdjustInventory)
		sellerRoute.GET("/products/:id/inventory/ledger", h.CatalogHandler.InventoryLedger)
		sellerRoute.GET("/inventory/low-stock", h.CatalogHandler.LowStock)
	}

	// Platform administrators only
	var systemTargets []handler.SystemTarget
	for name, url := range envConfig.SystemTargets {
		systemTargets = append(systemTargets, handler.SystemTarget{Name: name, URL: url})
	}
	systemHandler := handler.NewSystemHandler(systemTargets, serviceConfig.ZapLogger)
	adminRoute := router.Group("/admin", authenticated, middleware.AuthorizationMiddleware([]string{"admin"}, serviceConfig.ZapLogger))
	{
		adminRoute.GET("/system/health", systemHandler.Health)
		adminRoute.GET("/categories", h.CatalogHandler.AdminCategories)
		adminRoute.POST("/categories", h.CatalogHandler.CreateCategory)
		adminRoute.PUT("/categories/:id", h.CatalogHandler.UpdateCategory)
		adminRoute.PATCH("/products/:id/status", h.CatalogHandler.AdminSetStatus)
		adminRoute.GET("/payments", h.PaymentHandler.AdminList)
		adminRoute.GET("/payments/:id", h.PaymentHandler.AdminGet)
		adminRoute.POST("/payments/:id/refund", h.PaymentHandler.AdminRefund)
		adminRoute.POST("/dev/payments/:id/force", h.PaymentHandler.AdminForce)
		adminRoute.GET("/orders", h.OrderHandler.ListAllOrders())
		adminRoute.GET("/orders/:id", h.OrderHandler.GetAnyOrder())
		adminRoute.POST("/orders/:id/cancel", h.OrderHandler.AdminCancel())
		adminRoute.POST("/orders/:id/deliver", h.OrderHandler.AdminDeliver())
		adminRoute.POST("/orders/:id/return/approve", h.OrderHandler.AdminApproveReturn())
		adminRoute.POST("/orders/:id/return/reject", h.OrderHandler.AdminRejectReturn())
	}

	// Buyers: cart, checkout, orders, addresses (every handler scopes its queries to the authenticated buyer)
	buyerRoles := middleware.AuthorizationMiddleware([]string{"buyer"}, serviceConfig.ZapLogger)
	cartRoute := router.Group("/cart", authenticated, buyerRoles)
	{
		cartRoute.GET("", h.OrderHandler.GetCart)
		cartRoute.DELETE("", h.OrderHandler.ClearCart)
		cartRoute.POST("/merge", h.OrderHandler.MergeCart)
		cartRoute.PUT("/items/:product_id", h.OrderHandler.SetCartItem)
		cartRoute.DELETE("/items/:product_id", h.OrderHandler.RemoveCartItem)
	}
	router.POST("/checkout/preview", authenticated, buyerRoles, h.OrderHandler.PreviewCheckout)
	router.GET("/checkouts/:id", authenticated, buyerRoles, h.OrderHandler.GetCheckout)

	orderRoute := router.Group("/orders", authenticated, buyerRoles)
	{
		orderRoute.POST("", h.OrderHandler.Checkout)
		orderRoute.GET("", h.OrderHandler.ListMyOrders()) // ?status=&page=&page_size=
		orderRoute.GET("/summary", h.OrderHandler.CountMyOrders())
		orderRoute.GET("/:id", h.OrderHandler.GetMyOrder())
		orderRoute.POST("/:id/cancel", h.OrderHandler.BuyerCancel())
		orderRoute.DELETE("/:id", h.OrderHandler.BuyerCancel()) // kept for old clients: same as POST /:id/cancel
		orderRoute.POST("/:id/confirm-received", h.OrderHandler.BuyerConfirmReceived())
		orderRoute.POST("/:id/return", h.OrderHandler.BuyerRequestReturn())
	}
	addressRoute := router.Group("/users/me/addresses", authenticated, buyerRoles)
	{
		addressRoute.GET("", h.OrderHandler.ListAddresses)
		addressRoute.POST("", h.OrderHandler.CreateAddress)
		addressRoute.PUT("/:id", h.OrderHandler.UpdateAddress)
		addressRoute.DELETE("/:id", h.OrderHandler.DeleteAddress)
	}

	// Payments (SIMULATED provider): a buyer pays only their own payments
	router.GET("/payments/methods", h.PaymentHandler.Methods)
	paymentRoute := router.Group("/payments", authenticated, buyerRoles)
	{
		paymentRoute.GET("/:id", h.PaymentHandler.Get)
		paymentRoute.POST("/:id/confirm", h.PaymentHandler.Confirm)
		paymentRoute.POST("/:id/cancel", h.PaymentHandler.Cancel)
	}

	// Sellers: the order inbox of their own store
	sellerOrders := router.Group("/seller/orders", authenticated, sellerRoles)
	{
		sellerOrders.GET("", h.OrderHandler.ListStoreOrders())
		sellerOrders.GET("/summary", h.OrderHandler.CountStoreOrders())
		sellerOrders.GET("/:id", h.OrderHandler.GetStoreOrder())
		sellerOrders.POST("/:id/ship", h.OrderHandler.SellerShip())
		sellerOrders.POST("/:id/deliver", h.OrderHandler.SellerDeliver())
		sellerOrders.POST("/:id/cancel", h.OrderHandler.SellerCancel())
		sellerOrders.POST("/:id/return/approve", h.OrderHandler.SellerApproveReturn())
		sellerOrders.POST("/:id/return/reject", h.OrderHandler.SellerRejectReturn())
	}
}
