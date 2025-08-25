package handler

import (
	"api-gateway/internal/grpc_client"
	"api-gateway/internal/service"
	pb "api-gateway/pkg/pb/auth_service"
	"log"

	"go.uber.org/zap"
)

// ManagerHandler save handlers for all service gRPC
type ManagerHandler struct {
	AuthHandler *AuthHandler
}

// NewHandlerManager init handlers for ManagerHandler
func NewHandlerManager(cm *grpc_client.ClientManager, logger *zap.Logger) *ManagerHandler {
	authClient := cm.Clients["AuthClient"]

	// AuthService: client can be nil if no action call to AuthClient
	var authService *service.AuthService
	if authClient == nil {
		log.Printf("AuthService client is not ok")
		authService = service.NewAuthService(nil, cm, logger)
	} else {
		// Parse AuthServiceClient
		authServiceClient, ok := authClient.Client.(pb.AuthServiceClient)
		if !ok {
			log.Printf("AuthService client is not ok")
			authService = service.NewAuthService(nil, cm, logger)
		}

		authService = service.NewAuthService(authServiceClient, cm, logger)
	}
	authHandler := NewAuthHandler(authService, logger)

	// Return ManagerHandler
	return &ManagerHandler{
		AuthHandler: authHandler,
	}
}
