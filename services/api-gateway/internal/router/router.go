package router

import (
	"api-gateway/internal/agent"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"go.uber.org/zap"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter setup middleware, router for engine
func SetupRouter(router *gin.Engine, h *handler.ManagerHandler, serviceConfig *config.ServiceConfig, envConfig *config.EnvConfig) {

	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.TracingMiddleware())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     envConfig.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Idempotency-Key", "X-Request-ID", middleware.ProposalHeader},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
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

	pwdOpt := middleware.WithPwdVersionFallback(h.AuthHandler.Service, envConfig.JWTExpireTime+time.Minute)
	authenticated := middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret, pwdOpt)

	// ADR-42: the agent's own token, accepted ONLY on the read routes listed in agent.DefaultScopes (and only for the role
	// in the token). On those routes agentOr(...) replaces the user-auth middleware; every other route still rejects it.
	keyRing, _ := agent.ParseKeyRing(envConfig.AgentKeys) // already validated in NewEnvConfig
	signer := &agent.Signer{Keys: keyRing}
	pwdVerifier := middleware.NewPwdVerifier(serviceConfig.ZapLogger, serviceConfig.RedisClient, pwdOpt)
	// confirm sits after auth + role on every proposable write; without X-Agent-Proposal it does nothing (direct action).
	confirm := middleware.ConfirmProposal(signer, agent.RedisOnce{RDB: serviceConfig.RedisClient}, serviceConfig.ZapLogger)
	agentOr := func(userAuth gin.HandlerFunc) gin.HandlerFunc {
		return middleware.AgentOrUser(signer, agent.DefaultScopes, pwdVerifier, userAuth, serviceConfig.ZapLogger)
	}

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
		authRoute.POST("/register-seller-roles", middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret, middleware.WithPwdVersionFallback(h.AuthHandler.Service, envConfig.JWTExpireTime+time.Minute)),
			middleware.AuthorizationMiddleware([]string{"seller_admin"}, serviceConfig.ZapLogger),
			h.AuthHandler.RegisterSellerRoles)
	}

	// NOTE: authentication is attached per group/route (never with router.Use) so the public catalog,
	// /payments/methods and /events stay reachable without a token.
	userRoute := router.Group("/users", authenticated)
	{
		//userRoute.Use(middleware.AuthMiddleware(serviceConfig.ZapLogger, serviceConfig.RedisClient, envConfig.JWTSecret, middleware.WithPwdVersionFallback(h.AuthHandler.Service, envConfig.JWTExpireTime+time.Minute)))
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
	router.GET("/search", agentOr(optionalAuth), h.CatalogHandler.Search)
	router.GET("/categories", h.CatalogHandler.Categories)

	// Clickstream (public, own rate limit; /events/identify needs a token)
	eventsRoute := router.Group("/events")
	if envConfig.EventsRateLimit > 0 {
		eventsRoute.Use(middleware.RateLimitingMiddleware("events", envConfig.EventsRateLimit, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient))
	}
	eventsRoute.POST("", optionalAuth, h.TrackingHandler.Ingest)
	eventsRoute.POST("/identify", authenticated, h.TrackingHandler.Identify)

	productRoute := router.Group("/products")
	{
		productRoute.POST("", authenticated, sellerRoles, h.ProductHandler.CreateProduct)
		productRoute.PUT("/:id", authenticated, sellerRoles, h.ProductHandler.UpdateProduct)
		productRoute.PATCH("/:id/status", authenticated, sellerRoles, h.CatalogHandler.SetStatus)
		productRoute.GET("/:id", agentOr(optionalAuth), h.ProductHandler.GetProductByID)
		productRoute.GET("/:id/stock", h.CatalogHandler.Stock)
		productRoute.GET("", optionalAuth, h.ProductHandler.GetProducts)
		productRoute.GET("/seller/:seller_id", optionalAuth, h.ProductHandler.GetProductsBySellerID)
	}

	// Seller console
	// Reads the seller agent may call (agent.DefaultScopes) sit in their own group: see cartRead.
	sellerRead := router.Group("/seller", agentOr(authenticated), sellerRoles)
	{
		sellerRead.GET("/products", h.CatalogHandler.SellerProducts)
		sellerRead.GET("/products/:id/inventory/ledger", h.CatalogHandler.InventoryLedger)
		sellerRead.GET("/inventory/low-stock", h.CatalogHandler.LowStock)

		// Store analytics (scope = the caller's store, from the token)
		for _, r := range []string{"summary", "timeseries", "top-products", "funnel", "low-stock"} {
			sellerRead.GET("/analytics/"+r, h.AnalyticsHandler.Seller(r))
		}
	}
	sellerRoute := router.Group("/seller", authenticated, sellerRoles)
	{
		sellerRoute.POST("/products/:id/inventory/adjust", confirm, h.CatalogHandler.AdjustInventory)
	}

	// Feature flags (public) and the AI chat stream (ADR-40). The gateway streams to the agent service; it has no model logic.
	router.GET("/config/features", handler.FeaturesHandler{AIChat: envConfig.FeatureAIChat && envConfig.AIServiceURL != "", AIOps: envConfig.FeatureAIOps && envConfig.AIServiceURL != ""}.Get)
	aiHandler := handler.NewAIHandler(envConfig.FeatureAIChat, envConfig.AIServiceURL, envConfig.AIStreamTimeout, serviceConfig.ZapLogger)
	// ADR-42: agent tokens and signed action proposals, keys only here (never JWT_SECRET). No keys = both off.
	aiHandler.Signer = signer
	if serviceConfig.RedisClient != nil {
		aiHandler.Slots = handler.RedisSlots{RDB: serviceConfig.RedisClient} // one running chat stream per user, across replicas
	}
	if keyRing.Enabled() {
		serviceConfig.ZapLogger.Info("agent token keys loaded", zap.Strings("kids", keyRing.IDs())) // ids only, never secrets
	}
	// Agent-token routes: limited per user (the token's subject), refusing when Redis is down (every call can cost money).
	agentRoute := router.Group("/ai", middleware.AgentAuth(signer, serviceConfig.ZapLogger))
	if envConfig.AIProposalRateLimit > 0 {
		agentRoute.Use(middleware.RateLimitByKey("ai_agent", envConfig.AIProposalRateLimit, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient, middleware.UserKey, true))
	}
	agentRoute.POST("/proposals", handler.ProposalHandler{Signer: signer}.Create)
	agentRoute.GET("/tools", handler.ToolsHandler{Scopes: agent.DefaultScopes}.List)
	aiRoute := router.Group("/ai", authenticated, middleware.AuthorizationMiddleware([]string{"buyer", "seller_admin", "seller_employee", "admin", "operator"}, serviceConfig.ZapLogger))
	if envConfig.AIRateLimit > 0 {
		// per IP (coarse, shared offices stay possible) and refusing when Redis is down: every chat can cost money
		aiRoute.Use(middleware.RateLimitByKey("ai", envConfig.AIRateLimit, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient, func(c *gin.Context) string { return c.ClientIP() }, true))
	}
	if envConfig.AIUserRateLimit > 0 {
		aiRoute.Use(middleware.RateLimitByKey("ai_user", envConfig.AIUserRateLimit, time.Minute, serviceConfig.ZapLogger, serviceConfig.RedisClient, middleware.UserKey, true))
	}
	aiRoute.POST("/chat", aiHandler.Chat)
	aiRoute.GET("/conversations", aiHandler.History("/conversations", false, "limit"))
	aiRoute.GET("/conversations/:id", aiHandler.History("/conversations", true))

	// Platform administrators only
	var systemTargets []handler.SystemTarget
	for name, url := range envConfig.SystemTargets {
		systemTargets = append(systemTargets, handler.SystemTarget{Name: name, URL: url})
	}
	if envConfig.AIServiceURL != "" { // the agent serves /ready on its single HTTP port, not on :8081
		if _, ok := envConfig.SystemTargets["ai-agent"]; !ok {
			systemTargets = append(systemTargets, handler.SystemTarget{Name: "ai-agent", URL: envConfig.AIServiceURL})
		}
	}
	systemHandler := handler.NewSystemHandler(systemTargets, serviceConfig.ZapLogger)
	adminRoute := router.Group("/admin", authenticated, middleware.AuthorizationMiddleware([]string{"admin"}, serviceConfig.ZapLogger))
	{
		opsHandler := handler.NewOpsHandler(envConfig.OpsServiceURL, envConfig.OpsAPIToken, serviceConfig.ZapLogger)
		// The whole /ops workflow (read, propose, review, approve, execute) and system health: admin or operator (ADR-40).
		// Reports, revenue and every /admin route stay admin only (except pipeline data-health below). The creator of a proposal still cannot approve or run it.
		adminOrOperator := middleware.AuthorizationMiddleware([]string{"admin", "operator"}, serviceConfig.ZapLogger)
		opsRead := router.Group("/ops", authenticated, adminOrOperator)
		opsRead.POST("/incidents/:id/investigate", opsHandler.Proxy("/incidents/:id/investigate"))
		opsRead.GET("/queries", opsHandler.Proxy("/queries"))
		opsRead.POST("/queries/:name", opsHandler.Proxy("/queries/:name"))
		opsRead.POST("/proposals", confirm, opsHandler.Proxy("/proposals"))
		opsRead.POST("/proposals/:id/review", opsHandler.Proxy("/proposals/:id/review"))
		opsRead.POST("/proposals/:id/decision", opsHandler.Proxy("/proposals/:id/decision"))
		opsRead.POST("/proposals/:id/execute", opsHandler.Proxy("/proposals/:id/execute"))
		opsRead.POST("/proposals/:id/executed", opsHandler.Proxy("/proposals/:id/executed"))
		// pipeline health (lag, reconcile), not business figures: stays under /admin/analytics, open to the operator as well
		// Reads the admin/operator agent may call (agent.DefaultScopes) sit in groups with agentOr: a group-level `authenticated`
		// would reject the agent token before it is checked. Writes stay on adminRoute/opsRead with the user's own JWT.
		opsAgentRead := router.Group("/ops", agentOr(authenticated), adminOrOperator)
		opsAgentRead.GET("/incidents", opsHandler.Proxy("/incidents"))
		opsAgentRead.GET("/incidents/:id", opsHandler.Proxy("/incidents/:id"))
		opsAgentRead.GET("/actions", opsHandler.Proxy("/actions"))
		opsAgentRead.GET("/audit", opsHandler.Proxy("/audit"))
		opsAgentRead.GET("/outbox/:service/dead", opsHandler.Proxy("/outbox/:service/dead"))
		opsAgentRead.GET("/system/health", systemHandler.Health)
		opsAgentRead.GET("/playbooks", opsHandler.Proxy("/playbooks"))
		opsAgentRead.GET("/playbooks/:id", opsHandler.Proxy("/playbooks/:id"))
		opsAgentRead.GET("/proposals", opsHandler.Proxy("/proposals"))
		opsAgentRead.GET("/proposals/:id", opsHandler.Proxy("/proposals/:id"))
		opsAgentRead.GET("/queries/:name", opsHandler.Query())
		router.Group("/admin", agentOr(authenticated), adminOrOperator).GET("/analytics/data-health", h.AnalyticsHandler.Admin("data-health"))
		adminAgentRead := router.Group("/admin", agentOr(authenticated), middleware.AuthorizationMiddleware([]string{"admin"}, serviceConfig.ZapLogger))
		for _, r := range []string{"summary", "timeseries", "top-products", "funnel", "low-stock", "traffic", "payments", "search-terms"} {
			adminAgentRead.GET("/analytics/"+r, h.AnalyticsHandler.Admin(r))
		}
		adminAgentRead.GET("/categories", h.CatalogHandler.AdminCategories)
		adminAgentRead.GET("/payments", h.PaymentHandler.AdminList)
		adminAgentRead.GET("/payments/:id", h.PaymentHandler.AdminGet)
		adminAgentRead.GET("/orders", h.OrderHandler.ListAllOrders())
		adminAgentRead.GET("/orders/:id", h.OrderHandler.GetAnyOrder())
		adminRoute.GET("/ai/conversations", aiHandler.History("/admin/conversations", false, "limit", "role", "user_id"))
		adminRoute.GET("/ai/conversations/:id", aiHandler.History("/admin/conversations", true))
		adminRoute.GET("/accounts", h.AuthHandler.ListAccounts)
		adminRoute.POST("/accounts", h.AuthHandler.CreateStaffAccount)
		adminRoute.DELETE("/accounts/:id", h.AuthHandler.DeleteStaffAccount)
		adminRoute.POST("/accounts/:id/reset-password", h.AuthHandler.ResetStaffPassword)
		adminRoute.POST("/categories", h.CatalogHandler.CreateCategory)
		adminRoute.PUT("/categories/:id", h.CatalogHandler.UpdateCategory)
		adminRoute.PATCH("/products/:id/status", h.CatalogHandler.AdminSetStatus)
		adminRoute.POST("/payments/:id/refund", confirm, h.PaymentHandler.AdminRefund)
		adminRoute.POST("/dev/payments/:id/force", h.PaymentHandler.AdminForce)
		adminRoute.POST("/orders/:id/cancel", confirm, h.OrderHandler.AdminCancel())
		adminRoute.POST("/orders/:id/deliver", h.OrderHandler.AdminDeliver())
		adminRoute.POST("/orders/:id/return/approve", h.OrderHandler.AdminApproveReturn())
		adminRoute.POST("/orders/:id/return/reject", h.OrderHandler.AdminRejectReturn())
	}

	// Buyers: cart, checkout, orders, addresses (every handler scopes its queries to the authenticated buyer)
	buyerRoles := middleware.AuthorizationMiddleware([]string{"buyer"}, serviceConfig.ZapLogger)
	cartRoute := router.Group("/cart", authenticated, buyerRoles)
	// Read routes the buyer agent may call with its token (agent.DefaultScopes) are in separate groups: a group-level
	// `authenticated` would reject the agent token before agentOr runs. Writes always need the user's own JWT.
	cartRead := router.Group("/cart", agentOr(authenticated), buyerRoles)
	cartRead.GET("", h.OrderHandler.GetCart)
	{
		cartRoute.DELETE("", h.OrderHandler.ClearCart)
		cartRoute.POST("/merge", h.OrderHandler.MergeCart)
		cartRoute.PUT("/items/:product_id", h.OrderHandler.SetCartItem)
		cartRoute.DELETE("/items/:product_id", h.OrderHandler.RemoveCartItem)
	}
	router.POST("/checkout/preview", authenticated, buyerRoles, h.OrderHandler.PreviewCheckout)
	router.GET("/checkouts/:id", authenticated, buyerRoles, h.OrderHandler.GetCheckout)

	orderRead := router.Group("/orders", agentOr(authenticated), buyerRoles)
	{
		orderRead.GET("", h.OrderHandler.ListMyOrders()) // ?status=&page=&page_size=
		orderRead.GET("/summary", h.OrderHandler.CountMyOrders())
		orderRead.GET("/:id", h.OrderHandler.GetMyOrder())
	}
	orderRoute := router.Group("/orders", authenticated, buyerRoles)
	{
		orderRoute.POST("", h.OrderHandler.Checkout)
		orderRoute.POST("/:id/cancel", confirm, h.OrderHandler.BuyerCancel())
		orderRoute.DELETE("/:id", h.OrderHandler.BuyerCancel()) // kept for old clients: same as POST /:id/cancel
		orderRoute.POST("/:id/confirm-received", h.OrderHandler.BuyerConfirmReceived())
		orderRoute.POST("/:id/return", confirm, h.OrderHandler.BuyerRequestReturn())
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
	router.GET("/shop/rules", h.OrderHandler.ShopRules) // public: order rules in force (also read by the assistant)
	paymentRoute := router.Group("/payments", authenticated, buyerRoles)
	{
		paymentRoute.GET("/:id", h.PaymentHandler.Get)
		paymentRoute.POST("/:id/confirm", h.PaymentHandler.Confirm)
		paymentRoute.POST("/:id/cancel", h.PaymentHandler.Cancel)
	}

	// Sellers: the order inbox of their own store
	sellerOrdersRead := router.Group("/seller/orders", agentOr(authenticated), sellerRoles)
	{
		sellerOrdersRead.GET("", h.OrderHandler.ListStoreOrders())
		sellerOrdersRead.GET("/summary", h.OrderHandler.CountStoreOrders())
		sellerOrdersRead.GET("/:id", h.OrderHandler.GetStoreOrder())
	}
	sellerOrders := router.Group("/seller/orders", authenticated, sellerRoles)
	{
		sellerOrders.POST("/:id/ship", h.OrderHandler.SellerShip())
		sellerOrders.POST("/:id/deliver", h.OrderHandler.SellerDeliver())
		sellerOrders.POST("/:id/cancel", confirm, h.OrderHandler.SellerCancel())
		sellerOrders.POST("/:id/return/approve", confirm, h.OrderHandler.SellerApproveReturn())
		sellerOrders.POST("/:id/return/reject", confirm, h.OrderHandler.SellerRejectReturn())
	}
}
