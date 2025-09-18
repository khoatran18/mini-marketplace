package userclient

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/clientname"
	"api-gateway/pkg/dto"
	userpb "api-gateway/pkg/pb/userservice"
	"context"
	"errors"
	"time"

	"buf.build/go/protovalidate"
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

// For buyer

func (s *UserClient) CreateBuyer(input *dto.CreateBuyerInput) (*dto.CreateBuyerOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := CreBuyInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse CreateBuyer input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for CreateBuyer", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.CreateBuyer(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: CreateBuyer error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for CreateBuyer", zap.Error(err))
		return nil, err
	}
	output, err := CreBuyResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for CreateBuyer", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *UserClient) GetBuyerByUserID(input *dto.GetBuyByUseIDInput) (*dto.GetBuyByUseIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := GetBuyByUseIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse GetBuyerByUserID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for GetBuyerByUserID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.GetBuyerByUserID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: GetBuyerByUserID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for GetBuyerByUserID", zap.Error(err))
		return nil, err
	}
	output, err := GetBuyByUseIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for GetBuyerByUserID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil

}

func (s *UserClient) UpdateBuyerByUserID(input *dto.UpdBuyByUseIDInput) (*dto.UpdBuyByUseIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := UpdBuyByUseIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse UpdateBuyerByUserID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for UpdateBuyerByUserID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.UpdateBuyerByUserID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: UpdateBuyerByUserID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for UpdateBuyerByUserID", zap.Error(err))
		return nil, err
	}
	output, err := UpdBuyByUseIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for UpdateBuyerByUserID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *UserClient) DelBuyerByUserID(input *dto.DelBuyByUseIDInput) (*dto.DelBuyByUseIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := DelBuyByUseIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse DelBuyerByUserID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for DelBuyerByUserID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.DelBuyerByUserID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: DelBuyerByUserID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for DelBuyerByUserID", zap.Error(err))
		return nil, err
	}
	output, err := DelBuyByUseIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for DelBuyerByUserID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// For Seller

func (s *UserClient) CreateSeller(input *dto.CreateSellerInput) (*dto.CreateSellerOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := CreSelInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse CreateSeller input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for CreateSeller", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.CreateSeller(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: CreateSeller error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for CreateSeller", zap.Error(err))
		return nil, err
	}
	output, err := CreSelResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for CreateSeller", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *UserClient) GetSellerByID(input *dto.GetSelByIDInput) (*dto.GetSelByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := GetSelByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse GetSellerByUserID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for GetSellerByUserID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.GetSellerByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: GetSellerByUserID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for GetSellerByUserID", zap.Error(err))
		return nil, err
	}
	output, err := GetSelByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for GetSellerByUserID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil

}

func (s *UserClient) UpdateSellerByID(input *dto.UpdSelByIDInput) (*dto.UpdSelByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := UpdSelByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse UpdateSellerByID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for UpdateSellerByID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.UpdateSellerByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: UpdateSellerByID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for UpdateSellerByID", zap.Error(err))
		return nil, err
	}
	output, err := UpdSelByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for UpdateSellerByID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *UserClient) DelSellerByID(input *dto.DelSelByIDInput) (*dto.DelSelByIDOutput, error) {
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := DelSelByIDInputToRequest(input)
	if err != nil {
		s.Logger.Warn("UserServer: parse DelSellerByID input to request error", zap.Error(err))
		return nil, err
	}
	if err := protovalidate.Validate(req); err != nil {
		s.Logger.Warn("UserServer: invalid request for DelSellerByID", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.DelSellerByID(ctx, req)

	// Get response, validate and parse to output
	if err != nil {
		s.Logger.Warn("UserServer: DelSellerByID error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("UserServer: invalid response for DelSellerByID", zap.Error(err))
		return nil, err
	}
	output, err := DelSelByIDResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("UserServer: invalid response for DelSellerByID", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// Validate client

func (s *UserClient) validateClient() error {
	authClient, err := s.ClientManager.GetOrCreateServiceClient(clientname.UserClientName)
	if err != nil {
		s.Logger.Error("UserClient: UserClient is nil and create failed", zap.Error(err))
		return errors.New("UserClient: UserClient is nil and create failed")
	}
	client, ok := authClient.(userpb.UserServiceClient)
	if !ok {
		s.Logger.Error("UserClient: UserClient is nil and create success but is not UserClient", zap.Error(err))
	}
	s.Logger.Info("UserClient: UserClient is nil and create success")
	s.Client = client
	return nil
}
