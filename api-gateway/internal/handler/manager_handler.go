package handler

import (
	"api-gateway/internal/grpc_client"
	"api-gateway/internal/service"
	pb "api-gateway/pkg/pb/auth_service"
	"log"

	"go.uber.org/zap"
)

type ManagerHandler struct {
	AuthHandler *AuthHandler
}

func NewHandlerManager(cm *grpc_client.ClientManager, logger *zap.Logger) *ManagerHandler {
	authClient := cm.Clients["AuthClient"]
	var authService *service.AuthService
	authServiceClient, ok := authClient.Client.(pb.AuthServiceClient)
	if !ok {
		log.Printf("AuthService client is not ok")
		authService = &service.AuthService{
			Client: nil,
			Logger: logger,
		}
	} else {
		authService = service.NewAuthService(authServiceClient, logger)
	}
	authHandler := NewAuthHandler(authService, logger)

	return &ManagerHandler{
		AuthHandler: authHandler,
	}
}
