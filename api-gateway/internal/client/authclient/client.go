package authclient

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/clientname"
	"api-gateway/pkg/dto"
	"api-gateway/pkg/pb/authservice"
	"context"
	"errors"
	"time"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
)

// AuthClient is responsible for interacting with AuthClient
type AuthClient struct {
	Client        authpb.AuthServiceClient
	ClientManager *client.ClientManager
	Logger        *zap.Logger
}

// NewAuthClient create AuthClient
func NewAuthClient(client authpb.AuthServiceClient, clientManager *client.ClientManager, logger *zap.Logger) *AuthClient {
	return &AuthClient{
		Client:        client,
		ClientManager: clientManager,
		Logger:        logger,
	}
}

// Login handle request from AuthHandler to AuthClient
func (s *AuthClient) Login(input *dto.LoginInput) (*dto.LoginOutput, error) {

	// Check if AuthClient is not connected
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := LoginInputToRequest(input)
	if err != nil {
		s.Logger.Warn("AuthServer: parse Login input to request error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(req); err != nil {
		s.Logger.Warn("AuthServer: invalid request for Login", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.Login(ctx, req)

	// Get response, validate and parse to input
	if err != nil {
		s.Logger.Warn("AuthClient: Login error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("AuthClient: Invalid response for Login", zap.Error(err))
		return nil, err
	}
	output, err := LoginResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("AuthClient: parse Login response to output error", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// Register handle request from AuthHandler to AuthClient
func (s *AuthClient) Register(input *dto.RegisterInput) (*dto.RegisterOutput, error) {

	// Check if AuthClient is not connected
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := RegisterInputToRequest(input)
	if err != nil {
		s.Logger.Warn("AuthServer: parse Register input to request error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(req); err != nil {
		s.Logger.Warn("AuthServer: invalid request for Register", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.Register(ctx, req)

	// Get response, validate and parse to input
	if err != nil {
		s.Logger.Warn("AuthClient: Register error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("AuthClient: Invalid response for Register", zap.Error(err))
		return nil, err
	}
	output, err := RegisterResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("AuthClient: parse Register response to output error", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// ChangePassword handle request from AuthHandler to AuthClient
func (s *AuthClient) ChangePassword(input *dto.ChangePasswordInput) (*dto.ChangePasswordOutput, error) {

	// Check if AuthClient is not connected
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return nil, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := ChangPasswordInputToRequest(input)
	if err != nil {
		s.Logger.Warn("AuthServer: parse ChangePassword input to request error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(req); err != nil {
		s.Logger.Warn("AuthServer: invalid request for ChangePassword", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.ChangePassword(ctx, req)

	// Get response, validate and parse to input
	if err != nil {
		s.Logger.Warn("AuthClient: ChangePassword error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("AuthClient: Invalid response for ChangePassword", zap.Error(err))
		return nil, err
	}
	output, err := ChangePasswordResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("AuthClient: parse ChangePassword response to output error", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

// RefreshToken handle request from AuthHandler to AuthClient
func (s *AuthClient) RefreshToken(input *dto.RefreshTokenInput) (*dto.RefreshTokenOutput, error) {

	// Check if AuthClient is not connected
	if s.Client == nil {
		if err := s.validateClient(); err != nil {
			return &dto.RefreshTokenOutput{}, err
		}
	}

	// Parse to ServerRequest and validate
	req, err := RefreshTokenInputToRequest(input)
	if err != nil {
		s.Logger.Warn("AuthServer: parse RefreshToken input to request error", zap.Error(err))
		return &dto.RefreshTokenOutput{}, err
	}
	if err = protovalidate.Validate(req); err != nil {
		s.Logger.Warn("AuthServer: invalid request for Register", zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Client.RefreshToken(ctx, req)

	// Get response, validate and parse to input
	if err != nil {
		s.Logger.Warn("AuthClient: Register error", zap.Error(err))
		return nil, err
	}
	if err = protovalidate.Validate(res); err != nil {
		s.Logger.Warn("AuthClient: Invalid response for Register", zap.Error(err))
		return nil, err
	}
	output, err := RefreshTokenResponseToOutput(res)
	if err != nil {
		s.Logger.Warn("AuthClient: parse RefreshToken response to output error", zap.Error(err))
		return nil, err
	}

	// Return valid output
	return output, nil
}

func (s *AuthClient) validateClient() error {
	authClient, err := s.ClientManager.GetOrCreateServiceClient(clientname.AuthClientName)
	if err != nil {
		s.Logger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
		return errors.New("AuthClient: AuthClient is nil and create failed")
	}
	client, ok := authClient.(authpb.AuthServiceClient)
	if !ok {
		s.Logger.Error("AuthClient: AuthClient is nil and create success but is not AuthClient", zap.Error(err))
	}
	s.Logger.Info("AuthClient: AuthClient is nil and create success")
	s.Client = client
	return nil
}
