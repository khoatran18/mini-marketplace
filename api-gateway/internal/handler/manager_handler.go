package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/orderclient"
	"api-gateway/internal/client/productclient"
	"api-gateway/internal/client/userclient"

	"go.uber.org/zap"
)

// ManagerHandler save handlers for all client gRPC
type ManagerHandler struct {
	AuthHandler    *AuthHandler
	OrderHandler   *OrderHandler
	ProductHandler *ProductHandler
	UserHandler    *UserHandler
}

// NewHandlerManager init handlers for ManagerHandler
func NewHandlerManager(cm *client.ClientManager, logger *zap.Logger) *ManagerHandler {

	// Create AuthService (wrap AuthClient)
	authService := authclient.NewAuthClient(nil, cm, logger) // AuthClient is nil until it is called
	authHandler := NewAuthHandler(authService, logger)

	// Create OrderService (wrap OrderClient)
	orderService := orderclient.NewOrderClient(nil, cm, logger)
	orderHandler := NewOrderHandler(orderService, logger)

	// Create ProductService (wrap ProductClient)
	productService := productclient.NewProductClient(nil, cm, logger)
	productHandler := NewProductHandler(productService, logger)

	// Create UserService (wrap UserClient)
	userService := userclient.NewUserClient(nil, cm, logger)
	userHandler := NewUserHandler(userService, logger)

	// Return ManagerHandler
	return &ManagerHandler{
		AuthHandler:    authHandler,
		OrderHandler:   orderHandler,
		ProductHandler: productHandler,
		UserHandler:    userHandler,
	}
}
