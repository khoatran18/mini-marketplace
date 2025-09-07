package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"

	"go.uber.org/zap"
)

// ManagerHandler save handlers for all client gRPC
type ManagerHandler struct {
	AuthHandler *AuthHandler
}

// NewHandlerManager init handlers for ManagerHandler
func NewHandlerManager(cm *client.ClientManager, logger *zap.Logger) *ManagerHandler {

	// Create AuthService (wrap AuthClient)
	authService := authclient.NewAuthClient(nil, cm, logger) // AuthClient is nil until it is called
	authHandler := NewAuthHandler(authService, logger)

	// Return ManagerHandler
	return &ManagerHandler{
		AuthHandler: authHandler,
	}
}
