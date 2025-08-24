package server

import (
	"auth-service/internal/service"
	"auth-service/pkg/model"
	"auth-service/pkg/pb"
	"context"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthServer is responsible for handle gRPC request
type AuthServer struct {
	pb.UnimplementedAuthServiceServer
	AuthService *service.AuthService
	ZapLogger   *zap.Logger
}

// Login handle login request
func (s *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {

	// Validate proto request
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid argument", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Check account and generate token
	loginRequest := model.LoginRequest{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}
	signedAccessToken, signedRefreshToken, err := s.AuthService.Login(&loginRequest)
	if err != nil {
		s.ZapLogger.Error("AuthServer: login failed", zap.Error(err))
		return &pb.LoginResponse{
			Message:      "Login failed!",
			AccessToken:  "",
			RefreshToken: "",
			Success:      false,
		}, status.Errorf(codes.Internal, err.Error())
	}

	s.ZapLogger.Info("AuthServer: login success", zap.String("Username", req.GetUsername()))
	return &pb.LoginResponse{
		Message:      "Login successfully!",
		AccessToken:  signedAccessToken,
		RefreshToken: signedRefreshToken,
		Success:      true,
	}, nil
}

// Register handle register request
func (s *AuthServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {

	// Validate proto request
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid argument", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	// Check existed account and register
	registerRequest := model.RegisterRequest{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}
	err := s.AuthService.Register(&registerRequest)
	if err != nil {
		s.ZapLogger.Error("AuthServer: register failed", zap.Error(err))
		return &pb.RegisterResponse{
			Message: "Register failed!",
			Success: false,
		}, status.Errorf(codes.Internal, err.Error())
	}

	s.ZapLogger.Info("AuthServer: register success", zap.String("Username", req.GetUsername()))
	return &pb.RegisterResponse{
		Message: "Register successfully!",
		Success: true,
	}, err
}

// RefreshToken handle refresh token for expired access token
func (s *AuthServer) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {

	// Validate proto request
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid argument", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	// Check valid token and refresh
	signedRefreshToken := req.GetRefreshToken()
	signedAccessToken, signedRefreshToken, err := s.AuthService.RefreshToken(signedRefreshToken)
	if err != nil {
		s.ZapLogger.Error("AuthServer: refreshToken failed", zap.Error(err))
		return &pb.RefreshTokenResponse{
			Message:      "Refresh token failed!",
			AccessToken:  "",
			RefreshToken: "",
			Success:      false,
		}, status.Errorf(codes.Internal, err.Error())
	}

	s.ZapLogger.Info("AuthServer: refreshToken success", zap.String("Refresh Token", signedRefreshToken))
	return &pb.RefreshTokenResponse{
		Message:      "Refresh token successfully!",
		AccessToken:  signedAccessToken,
		RefreshToken: signedRefreshToken,
		Success:      true,
	}, nil

}

// ChangePassword handle change password request
func (s *AuthServer) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {

	// Validate proto request
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid argument", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	// Check account, password and update
	changePasswordRequest := &model.ChangePasswordRequest{
		Username:    req.GetUsername(),
		OldPassword: req.GetOldPassword(),
		NewPassword: req.GetNewPassword(),
		Role:        req.GetRole(),
	}
	err := s.AuthService.ChangePassword(changePasswordRequest)
	if err != nil {
		s.ZapLogger.Error("AuthServer: changePassword failed", zap.Error(err))
		return &pb.ChangePasswordResponse{
			Message: "Change password failed!",
			Success: false,
		}, err
	}

	s.ZapLogger.Info("AuthServer: changePassword success", zap.String("Username", req.GetUsername()))
	return &pb.ChangePasswordResponse{
		Message: "Change password successfully!",
		Success: true,
	}, nil
}
