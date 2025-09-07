package productclient

import (
	"context"
	"order-service/internal/client/clientmanager"
	productpb "order-service/pkg/client/productclient"

	"go.uber.org/zap"
)

type ProductClient struct {
	Client        productpb.ProductServiceClient
	ClientManager *clientmanager.ClientManager
	ZapLogger     *zap.Logger
}

func NewProductClient(client productpb.ProductServiceClient, cm *clientmanager.ClientManager, logger *zap.Logger) *ProductClient {
	return &ProductClient{
		Client:        client,
		ClientManager: cm,
		ZapLogger:     logger,
	}
}

func (p *ProductClient) GetProductsByID(ctx context.Context, input *GetProductsByIDInput) (*GetProductsByIDOutput, error) {
	if p.Client == nil {
		productClient, err := p.ClientManager.GetOrCreateProductClient()
		if err != nil {
			p.ZapLogger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
			return nil, err
		}
		p.ZapLogger.Info("AuthClient: AuthClient is nil and create success")
		p.Client = productClient
	}

	res, err := p.Client.GetProductsByID(ctx, &productpb.GetProductsByIDRequest{
		Id: input.IDs,
	})
	if err != nil {
		p.ZapLogger.Error("ProductClient: GetProductsByID error", zap.Error(err))
		return nil, err
	}

	output := ProductsProtoToDTO(res.GetProduct())
	return &GetProductsByIDOutput{
		Message:  "Product successfully",
		Success:  true,
		Products: output,
	}, nil
}
