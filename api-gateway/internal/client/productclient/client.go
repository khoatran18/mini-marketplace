package productclient

import (
	"api-gateway/internal/client"
	productpb "api-gateway/pkg/pb/productservice"

	"go.uber.org/zap"
)

// ProductClient is responsible for interacting with ProductClient
type ProductClient struct {
	Client        productpb.ProductServiceClient
	ClientManager *client.ClientManager
	Logger        *zap.Logger
}

// NewProductClient create ProductClient
func NewProductClient(client productpb.ProductServiceClient, clientManager *client.ClientManager, logger *zap.Logger) *ProductClient {
	return &ProductClient{
		Client:        client,
		ClientManager: clientManager,
		Logger:        logger,
	}
}
