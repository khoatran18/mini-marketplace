package serviceclientmanager

import (
	"order-service/internal/client/clientmanager"
	"order-service/internal/client/productclient"

	"go.uber.org/zap"
)

type ServiceClientManager struct {
	ProductServiceClient *productclient.ProductClient
}

func NewServiceClientManager(cm *clientmanager.ClientManager, logger *zap.Logger) *ServiceClientManager {

	productClient := productclient.NewProductClient(nil, cm, logger)

	return &ServiceClientManager{
		ProductServiceClient: productClient,
	}
}
