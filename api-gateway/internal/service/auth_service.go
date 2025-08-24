package service

import (
	"api-gateway/pkg/model"
	pb "api-gateway/pkg/pb/auth_service"
	"context"
	"time"

	"go.uber.org/zap"
)

type AuthService struct {
	Client pb.AuthServiceClient
	Logger *zap.Logger
}

func NewAuthService(client pb.AuthServiceClient, logger *zap.Logger) *AuthService {
	return &AuthService{
		Client: client,
		Logger: logger,
	}
}

func (service *AuthService) Login(req *model.LoginRequest) (*model.LoginResponse, error) {
	if service.Client == nil {
		return &model.LoginResponse{
			Message: "Service error",
			Success: false,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	loginResponse, err := service.Client.Login(ctx, &pb.LoginRequest{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
	})

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

func (service *AuthService) Register(req *model.RegisterRequest) (*model.RegisterResponse, error) {
	if service.Client == nil {
		service.Logger.Error("Service error")
		return &model.RegisterResponse{
			Message: "Service error",
			Success: false,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	registerResponse, err := service.Client.Register(ctx, &pb.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		service.Logger.Warn("AuthService Register error", zap.Error(err))
		return &model.RegisterResponse{}, err
	}

	return &model.RegisterResponse{
		Message: registerResponse.GetMessage(),
		Success: registerResponse.GetSuccess(),
	}, nil
}
