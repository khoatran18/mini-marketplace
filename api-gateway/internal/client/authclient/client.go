package authclient

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/dto"
	"api-gateway/pkg/pb/authservice"
	"context"
	"errors"
	"time"

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
func (service *AuthClient) Login(input *dto.LoginInput) (*dto.LoginOutput, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
			return &dto.LoginOutput{
				Message: "Service error and init failed",
				Success: false,
			}, nil
		}
		service.Logger.Info("AuthClient: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	loginResponse, err := service.Client.Login(ctx, &authpb.LoginRequest{
		Username: input.Username,
		Password: input.Password,
		Role:     input.Role,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthClient Login error", zap.Error(err))
		return &dto.LoginOutput{
			Success: false,
		}, err
	}
	return &dto.LoginOutput{
		Message:      loginResponse.GetMessage(),
		Success:      loginResponse.GetSuccess(),
		AccessToken:  loginResponse.GetAccessToken(),
		RefreshToken: loginResponse.GetRefreshToken(),
	}, nil
}

// Register handle request from AuthHandler to AuthClient
func (service *AuthClient) Register(input *dto.RegisterInput) (*dto.RegisterOutput, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
			return &dto.RegisterOutput{
				Message: "Service error and init failed",
				Success: false,
			}, nil
		}
		service.Logger.Info("AuthClient: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	registerResponse, err := service.Client.Register(ctx, &authpb.RegisterRequest{
		Username: input.Username,
		Password: input.Password,
		Role:     input.Role,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthClient Register error", zap.Error(err))
		return &dto.RegisterOutput{}, err
	}
	return &dto.RegisterOutput{
		Message: registerResponse.GetMessage(),
		Success: registerResponse.GetSuccess(),
	}, nil
}

// ChangePassword handle request from AuthHandler to AuthClient
func (service *AuthClient) ChangePassword(input *dto.ChangePasswordInput) (*dto.ChangePasswordOutput, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
			return &dto.ChangePasswordOutput{
				Message: "Service error and init failed",
				Success: false,
			}, nil
		}
		service.Logger.Info("AuthClient: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	changePasswordResponse, err := service.Client.ChangePassword(ctx, &authpb.ChangePasswordRequest{
		Username:    input.Username,
		OldPassword: input.OldPassword,
		NewPassword: input.NewPassword,
		Role:        input.Role,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthClient Change Password error", zap.Error(err))
		return &dto.ChangePasswordOutput{
			Message: "AuthClient Change Password error",
			Success: false,
		}, err
	}
	return &dto.ChangePasswordOutput{
		Message: changePasswordResponse.GetMessage(),
		Success: changePasswordResponse.GetSuccess(),
	}, nil
}

// RefreshToken handle request from AuthHandler to AuthClient
func (service *AuthClient) RefreshToken(input *dto.RefreshTokenInput) (*dto.RefreshTokenOutput, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
			return &dto.RefreshTokenOutput{
				Message: "Service error and init failed",
				Success: false,
			}, errors.New("AuthClient: AuthClient is nil and create failed")
		}
		service.Logger.Info("AuthClient: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	changePasswordResponse, err := service.Client.RefreshToken(ctx, &authpb.RefreshTokenRequest{
		RefreshToken: input.RefreshToken,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthClient Change Password error", zap.Error(err))
		return &dto.RefreshTokenOutput{
			Message: "AuthClient Change Password error",
			Success: false,
		}, err
	}
	return &dto.RefreshTokenOutput{
		Message:      changePasswordResponse.GetMessage(),
		AccessToken:  changePasswordResponse.GetAccessToken(),
		RefreshToken: changePasswordResponse.GetRefreshToken(),
		Success:      changePasswordResponse.GetSuccess(),
	}, nil
}
