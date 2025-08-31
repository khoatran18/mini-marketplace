package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/pb/auth_service"
	"log"

	"go.uber.org/zap"
)

// ManagerHandler save handlers for all client gRPC
type ManagerHandler struct {
	AuthHandler *AuthHandler
}

// NewHandlerManager init handlers for ManagerHandler
func NewHandlerManager(cm *client.ClientManager, logger *zap.Logger) *ManagerHandler {
	authClient := cm.Clients["AuthClient"]

	// AuthClient: client can be nil if no action call to AuthClient
	var authService *authclient.AuthClient
	if authClient == nil {
		log.Printf("AuthClient client is not ok")
		authService = authclient.NewAuthClient(nil, cm, logger)
	} else {
		// Parse AuthServiceClient
		authServiceClient, ok := authClient.Client.(authpb.AuthServiceClient)
		if !ok {
			log.Printf("AuthClient client is not ok")
			authService = authclient.NewAuthClient(nil, cm, logger)
		}

		authService = authclient.NewAuthClient(authServiceClient, cm, logger)
	}
	authHandler := NewAuthHandler(authService, logger)

	// Return ManagerHandler
	return &ManagerHandler{
		AuthHandler: authHandler,
	}
}
