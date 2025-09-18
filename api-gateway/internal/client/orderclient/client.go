package orderclient

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/clientname"
	"api-gateway/pkg/dto"
	orderpb "api-gateway/pkg/pb/orderservice"
	"context"
	"errors"
	"time"

	"buf.build/go/protovalidate"
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

func (s *OrderClient) CreateOrder(input *dto.CreateOrderInput) (*dto.CreateOrderOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := CreateOrderInputToRequest(input)
	if err != nil {
		s.Logger.Warn("OrderServer: parse CreateOrder input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("OrderServer: invalid request for CreateOrder", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.CreateOrder(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("OrderServer: CreateOrder error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("OrderServer: invalid response for CreateOrder", zap.Error(err))
		return nil, err
	}
	output, err := CreateOrderResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("OrderServer: invalid response for CreateOrder", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *OrderClient) GetOrderByID(input *dto.GetOrderByIDInput) (*dto.GetOrderByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := GetOrderByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("OrderServer: parse GetOrderByID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("OrderServer: invalid request for GetOrderByID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.GetOrderByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("OrderServer: GetOrderByID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("OrderServer: invalid response for GetOrderByID", zap.Error(err))
		return nil, err
	}
	output, err := GetOrderByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("OrderServer: invalid response for GetOrderByID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *OrderClient) GetOrdersByBuyerIDStatus(input *dto.GetOrdersByBuyerIDStatusInput) (*dto.GetOrdersByBuyerIDStatusOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := GetOrdersByBuyerIDStatusInputToRequest(input)
	if err != nil {
		s.Logger.Warn("OrderServer: parse GetOrdersByBuyerIDStatus input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("OrderServer: invalid request for GetOrdersByBuyerIDStatus", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.GetOrdersByBuyerIDStatus(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("OrderServer: GetOrdersByBuyerIDStatus error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("OrderServer: invalid response for GetOrdersByBuyerIDStatus", zap.Error(err))
		return nil, err
	}
	output, err := GetOrdersByBuyerIDStatusResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("OrderServer: invalid response for GetOrdersByBuyerIDStatus", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *OrderClient) UpdateOrderByID(input *dto.UpdateOrderByIDInput) (*dto.UpdateOrderByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := UpdateOrderByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("OrderServer: parse UpdateOrderByID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("OrderServer: invalid request for UpdateOrderByID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.UpdateOrderByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("OrderServer: UpdateOrderByID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("OrderServer: invalid response for UpdateOrderByID", zap.Error(err))
		return nil, err
	}
	output, err := UpdateOrderByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("OrderServer: invalid response for UpdateOrderByID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *OrderClient) CancelOrderByID(input *dto.CancelOrderByIDInput) (*dto.CancelOrderByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := CancelOrderByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("OrderServer: parse UpdateOrderByID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("OrderServer: invalid request for UpdateOrderByID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.CancelOrderByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("OrderServer: UpdateOrderByID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("OrderServer: invalid response for UpdateOrderByID", zap.Error(err))
		return nil, err
	}
	output, err := CancelOrderByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("OrderServer: invalid response for UpdateOrderByID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// Validate client

func (s *OrderClient) validateClient() error {
	authClient, err := s.ClientManager.GetOrCreateServiceClient(clientname.OrderClientName)
	if err != nil {
		s.Logger.Error("OrderClient: OrderClient is nil and create failed", zap.Error(err))
		return errors.New("OrderClient: OrderClient is nil and create failed")
	}
	client, ok := authClient.(orderpb.OrderServiceClient)
	if !ok {
		s.Logger.Error("OrderClient: OrderClient is nil and create success but is not OrderClient", zap.Error(err))
	}
	s.Logger.Info("OrderClient: OrderClient is nil and create success")
	s.Client = client
	return nil
}
