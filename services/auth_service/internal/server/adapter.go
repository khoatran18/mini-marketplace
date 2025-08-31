package server

import (
	"auth-service/pkg/dto"
	"auth-service/pkg/pb"
)

func LoginProtoToDTO(req *authpb.LoginRequest) (*dto.LoginInput, error) {
	return &dto.LoginInput{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}, nil
}

func LoginDTOToProto(output *dto.LoginOutput) (*authpb.LoginResponse, error) {
	return &authpb.LoginResponse{
		Message:      output.Message,
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		Success:      output.Success,
	}, nil
}

func RegisterProtoToDTO(req *authpb.RegisterRequest) (*dto.RegisterInput, error) {
	return &dto.RegisterInput{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}, nil
}

func RegisterDTOToProto(output *dto.RegisterOutput) (*authpb.RegisterResponse, error) {
	return &authpb.RegisterResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func ChangePasswordProtoToDTO(req *authpb.ChangePasswordRequest) (*dto.ChangePasswordInput, error) {
	return &dto.ChangePasswordInput{
		Username:    req.GetUsername(),
		OldPassword: req.GetOldPassword(),
		NewPassword: req.GetNewPassword(),
		Role:        req.GetRole(),
	}, nil
}

func ChangePasswordDTOToProto(output *dto.ChangePasswordOutput) (*authpb.ChangePasswordResponse, error) {
	return &authpb.ChangePasswordResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func RefreshTokenProtoToDTO(req *authpb.RefreshTokenRequest) (*dto.RefreshTokenInput, error) {
	return &dto.RefreshTokenInput{
		RefreshToken: req.GetRefreshToken(),
	}, nil
}

func RefreshTokenDTOToProto(output *dto.RefreshTokenOutput) (*authpb.RefreshTokenResponse, error) {
	return &authpb.RefreshTokenResponse{
		Message:      output.Message,
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		Success:      output.Success,
	}, nil
}
