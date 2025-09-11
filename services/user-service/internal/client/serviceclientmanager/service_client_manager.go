package serviceclientmanager

import (
	"user-service/internal/client/authclient"
	"user-service/internal/client/clientmanager"

	"go.uber.org/zap"
)

type ServiceClientManager struct {
	//ProductServiceClient *productclient.ProductClient
	AuthServiceClient *authclient.AuthClient
}

func NewServiceClientManager(cm *clientmanager.ClientManager, logger *zap.Logger) *ServiceClientManager {

	//productClient := productclient.NewProductClient(nil, cm, logger)
	//
	//return &ServiceClientManager{
	//	ProductServiceClient: productClient,
	//}

	authClient := authclient.NewAuthClient(nil, cm, logger)
	return &ServiceClientManager{
		AuthServiceClient: authClient,
	}
}
