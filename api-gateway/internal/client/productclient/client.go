package productclient

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/clientname"
	"api-gateway/pkg/dto"
	productpb "api-gateway/pkg/pb/productservice"
	"context"
	"errors"
	"time"

	"buf.build/go/protovalidate"
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

func (s *ProductClient) CreateProduct(input *dto.CreateProductInput) (*dto.CreateProductOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := CreateProductInputToRequest(input)
	if err != nil {
		s.Logger.Warn("ProductClient: parse CreateProduct input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("ProductClient: invalid request for CreateProduct", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.CreateProduct(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("ProductClient: CreateProduct error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("ProductClient: invalid response for CreateProduct", zap.Error(err))
		return nil, err
	}
	output, err := CreateProductResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("ProductClient: invalid response for CreateProduct", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *ProductClient) UpdateProduct(input *dto.UpdateProductInput) (*dto.UpdateProductOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := UpdateProductInputToRequest(input)
	if err != nil {
		s.Logger.Warn("ProductClient: parse UpdateProduct input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("ProductClient: invalid request for UpdateProduct", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.UpdateProduct(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("ProductClient: UpdateProduct error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("ProductClient: invalid response for UpdateProduct", zap.Error(err))
		return nil, err
	}
	output, err := UpdateProductResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("ProductClient: invalid response for UpdateProduct", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *ProductClient) GetProductByID(input *dto.GetProductByIDInput) (*dto.GetProductByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := GetProductByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("ProductClient: parse GetProductByID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("ProductClient: invalid request for GetProductByID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.GetProductByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("ProductClient: GetProductByID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("ProductClient: invalid response for GetProductByID", zap.Error(err))
		return nil, err
	}
	output, err := GetProductByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("ProductClient: invalid response for GetProductByID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *ProductClient) GetProductsBySellerID(input *dto.GetProductsBySellerIDInput) (*dto.GetProductsBySellerIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := GetProductsBySellerIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("ProductClient: parse GetProductsBySellerID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("ProductClient: invalid request for GetProductsBySellerID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.GetProductsBySellerID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("ProductClient: GetProductsBySellerID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("ProductClient: invalid response for GetProductsBySellerID", zap.Error(err))
		return nil, err
	}
	output, err := GetProductsBySellerIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("ProductClient: invalid response for GetProductsBySellerID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// Validate client

func (s *ProductClient) validateClient() error {
	authClient, err := s.ClientManager.GetOrCreateServiceClient(clientname.ProductClientName)
	if err != nil {
		s.Logger.Error("ProductClient: ProductClient is nil and create failed", zap.Error(err))
		return errors.New("ProductClient: ProductClient is nil and create failed")
	}
	client, ok := authClient.(productpb.ProductServiceClient)
	if !ok {
		s.Logger.Error("ProductClient: ProductClient is nil and create success but is not ProductClient", zap.Error(err))
	}
	s.Logger.Info("ProductClient: ProductClient is nil and create success")
	s.Client = client
	return nil
}
