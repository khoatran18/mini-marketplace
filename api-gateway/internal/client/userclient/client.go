package userclient

import (
	"api-gateway/internal/client"
	userpb "api-gateway/pkg/pb/userservice"

	"go.uber.org/zap"
)

// UserClient is responsible for interacting with UserClient
type UserClient struct {
	Client        userpb.UserServiceClient
	ClientManager *client.ClientManager
	Logger        *zap.Logger
}

// NewUserClient create UserClient
func NewUserClient(client userpb.UserServiceClient, clientManager *client.ClientManager, logger *zap.Logger) *UserClient {
	return &UserClient{
		Client:        client,
		ClientManager: clientManager,
		Logger:        logger,
	}
}
