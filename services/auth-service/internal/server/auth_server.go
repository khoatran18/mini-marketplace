package server

import (
	"auth-service/internal/service"
	"auth-service/pkg/pb"
	"context"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// AuthServer is responsible for handle gRPC request
type AuthServer struct {
	authpb.UnimplementedAuthServiceServer
	AuthService *service.AuthService
	ZapLogger   *zap.Logger
}

// Login handle login request
func (s *AuthServer) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid request for Login", zap.Error(err))
		return LoginFailResponse("AuthServer: Invalid request for Login", err, codes.InvalidArgument)
	}
	input, err := LoginProtoToDTO(req)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse Login request to input error", zap.Error(err))
		return LoginFailResponse("Parse Login request error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.AuthService.Login(input)
	if err != nil {
		s.ZapLogger.Error("AuthServer: Login error in AuthService", zap.Error(err))
		return LoginFailResponse("Login error in AuthService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := LoginDTOToProto(output)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse Login output to response error", zap.Error(err))
		return LoginFailResponse("Parse Login output to response error", err, codes.Internal)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid response for Login", zap.Error(err))
		return LoginFailResponse("Invalid response for Login", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}

// Register handle register request
func (s *AuthServer) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid request for Register", zap.Error(err))
		return RegisterFailResponse("Invalid request for Register", err, codes.InvalidArgument)
	}
	input, err := RegisterProtoToDTO(req)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse Register request to input error", zap.Error(err))
		return RegisterFailResponse("Parse Register request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.AuthService.Register(input)
	if err != nil {
		s.ZapLogger.Error("AuthServer: Register error in AuthService", zap.Error(err))
		return RegisterFailResponse("Register error in AuthService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := RegisterDTOToProto(output)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse Register output to response error", zap.Error(err))
		return RegisterFailResponse("Parse Register output to response error", err, codes.Internal)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid response for Register", zap.Error(err))
		return RegisterFailResponse("Invalid response for Register", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// RefreshToken handle refresh token for expired access token
func (s *AuthServer) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid request for RefreshToken", zap.Error(err))
		return RefreshTokenFailResponse("Invalid request for RefreshToken", err, codes.InvalidArgument)
	}
	input, err := RefreshTokenProtoToDTO(req)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse RefreshToken request to input error", zap.Error(err))
		return RefreshTokenFailResponse("Parse RefreshToken request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.AuthService.RefreshToken(input)
	if err != nil {
		s.ZapLogger.Error("AuthServer: RefreshToken error in AuthService", zap.Error(err))
		return RefreshTokenFailResponse("RefreshToken error in AuthService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := RefreshTokenDTOToProto(output)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse RefreshToken output to response error", zap.Error(err))
		return RefreshTokenFailResponse("parse RefreshToken output to response error", err, codes.Internal)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid response for RefreshToken", zap.Error(err))
		return RefreshTokenFailResponse("invalid response for RefreshToken", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// ChangePassword handle change password request
func (s *AuthServer) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid request for ChangePassword", zap.Error(err))
		return ChangePasswordFailResponse("Invalid request for ChangePassword", err, codes.InvalidArgument)
	}
	input, err := ChangePasswordProtoToDTO(req)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse ChangePassword request to input error", zap.Error(err))
		return ChangePasswordFailResponse("Parse ChangePassword request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.AuthService.ChangePassword(input)
	if err != nil {
		s.ZapLogger.Error("AuthServer: ChangePassword error in AuthService", zap.Error(err))
		return ChangePasswordFailResponse("ChangePassword error in AuthService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := ChangePasswordDTOToProto(output)
	if err != nil {
		s.ZapLogger.Warn("AuthServer: parse ChangePassword output to response error", zap.Error(err))
		return ChangePasswordFailResponse("parse ChangePassword output to response error", err, codes.Internal)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("AuthServer: invalid response for ChangePassword", zap.Error(err))
		return ChangePasswordFailResponse("invalid response for ChangePassword", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}
