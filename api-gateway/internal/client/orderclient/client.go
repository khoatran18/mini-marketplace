package orderclient

import (
	"api-gateway/internal/client"
	orderpb "api-gateway/pkg/pb/orderservice"

	"go.uber.org/zap"
)

// OrderClient is responsible for interacting with OrderClient
type OrderClient struct {
	Client        orderpb.OrderServiceClient
	ClientManager *client.ClientManager
	Logger        *zap.Logger
}

// NewOrderClient create OrderClient
func NewOrderClient(client orderpb.OrderServiceClient, clientManager *client.ClientManager, logger *zap.Logger) *OrderClient {
	return &OrderClient{
		Client:        client,
		ClientManager: clientManager,
		Logger:        logger,
	}
}
