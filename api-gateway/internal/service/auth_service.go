package service

import (
	"api-gateway/internal/grpc_client"
	"api-gateway/pkg/model"
	pb "api-gateway/pkg/pb/auth_service"
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

// AuthService is responsible for interacting with AuthClient
type AuthService struct {
	Client        pb.AuthServiceClient
	ClientManager *grpc_client.ClientManager
	Logger        *zap.Logger
}

// NewAuthService create AuthService
func NewAuthService(client pb.AuthServiceClient, clientManager *grpc_client.ClientManager, logger *zap.Logger) *AuthService {
	return &AuthService{
		Client:        client,
		ClientManager: clientManager,
		Logger:        logger,
	}
}

// Login handle request from AuthHandler to AuthClient
func (service *AuthService) Login(req *model.LoginRequest) (*model.LoginResponse, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthService: AuthClient is nil and create failed", zap.Error(err))
			return &model.LoginResponse{
				Message: "Service error and init failed",
				Success: false,
			}, nil
		}
		service.Logger.Info("AuthService: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	loginResponse, err := service.Client.Login(ctx, &pb.LoginRequest{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthService Login error", zap.Error(err))
		return &model.LoginResponse{
			Success: false,
		}, err
	}
	return &model.LoginResponse{
		Message:      loginResponse.GetMessage(),
		Success:      loginResponse.GetSuccess(),
		AccessToken:  loginResponse.GetAccessToken(),
		RefreshToken: loginResponse.GetRefreshToken(),
	}, nil
}

// Register handle request from AuthHandler to AuthClient
func (service *AuthService) Register(req *model.RegisterRequest) (*model.RegisterResponse, error) {

	// Check if AuthClient is not connected
	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthService: AuthClient is nil and create failed", zap.Error(err))
			return &model.RegisterResponse{
				Message: "Service error and init failed",
				Success: false,
			}, nil
		}
		service.Logger.Info("AuthService: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	registerResponse, err := service.Client.Register(ctx, &pb.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthService Register error", zap.Error(err))
		return &model.RegisterResponse{}, err
	}
	return &model.RegisterResponse{
		Message: registerResponse.GetMessage(),
		Success: registerResponse.GetSuccess(),
	}, nil
}

// ChangePassword handle request from AuthHandler to AuthClient
func (service *AuthService) ChangePassword(req *model.ChangePasswordRequest) (*model.ChangePasswordResponse, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthService: AuthClient is nil and create failed", zap.Error(err))
			return &model.ChangePasswordResponse{
				Message: "Service error and init failed",
				Success: false,
			}, nil
		}
		service.Logger.Info("AuthService: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	changePasswordResponse, err := service.Client.ChangePassword(ctx, &pb.ChangePasswordRequest{
		Username:    req.Username,
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
		Role:        req.Role,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthService Change Password error", zap.Error(err))
		return &model.ChangePasswordResponse{
			Message: "AuthService Change Password error",
			Success: false,
		}, err
	}
	return &model.ChangePasswordResponse{
		Message: changePasswordResponse.GetMessage(),
		Success: changePasswordResponse.GetSuccess(),
	}, nil
}

// RefreshToken handle request from AuthHandler to AuthClient
func (service *AuthService) RefreshToken(req *model.RefreshTokenRequest) (*model.RefreshTokenResponse, error) {

	// Check if AuthClient is not connected
	if service.Client == nil {
		authClient, err := service.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			service.Logger.Error("AuthService: AuthClient is nil and create failed", zap.Error(err))
			return &model.RefreshTokenResponse{
				Message: "Service error and init failed",
				Success: false,
			}, errors.New("AuthService: AuthClient is nil and create failed")
		}
		service.Logger.Info("AuthService: AuthClient is nil and create success")
		service.Client = authClient
	}

	// Send request to AuthClient
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	changePasswordResponse, err := service.Client.RefreshToken(ctx, &pb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})

	// Return response
	if err != nil {
		service.Logger.Warn("AuthService Change Password error", zap.Error(err))
		return &model.RefreshTokenResponse{
			Message: "AuthService Change Password error",
			Success: false,
		}, err
	}
	return &model.RefreshTokenResponse{
		Message:      changePasswordResponse.GetMessage(),
		AccessToken:  changePasswordResponse.GetAccessToken(),
		RefreshToken: changePasswordResponse.GetRefreshToken(),
		Success:      changePasswordResponse.GetSuccess(),
	}, nil
}
