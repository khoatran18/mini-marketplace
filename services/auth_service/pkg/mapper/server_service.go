package mapper

import (
	"auth-service/pkg/dto"
	"auth-service/pkg/pb"
)

func LoginProtoToDTO(req *pb.LoginRequest) (*dto.LoginInput, error) {
	return &dto.LoginInput{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}, nil
}

func LoginDTOToProto(output *dto.LoginOutput) (*pb.LoginResponse, error) {
	return &pb.LoginResponse{
		Message:      output.Message,
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		Success:      output.Success,
	}, nil
}

func RegisterProtoToDTO(req *pb.RegisterRequest) (*dto.RegisterInput, error) {
	return &dto.RegisterInput{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}, nil
}

func RegisterDTOToProto(output *dto.RegisterOutput) (*pb.RegisterResponse, error) {
	return &pb.RegisterResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func ChangePasswordProtoToDTO(req *pb.ChangePasswordRequest) (*dto.ChangePasswordInput, error) {
	return &dto.ChangePasswordInput{
		Username:    req.GetUsername(),
		OldPassword: req.GetOldPassword(),
		NewPassword: req.GetNewPassword(),
		Role:        req.GetRole(),
	}, nil
}

func ChangePasswordDTOToProto(output *dto.ChangePasswordOutput) (*pb.ChangePasswordResponse, error) {
	return &pb.ChangePasswordResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func RefreshTokenProtoToDTO(req *pb.RefreshTokenRequest) (*dto.RefreshTokenInput, error) {
	return &dto.RefreshTokenInput{
		RefreshToken: req.GetRefreshToken(),
	}, nil
}

func RefreshTokenDTOToProto(output *dto.RefreshTokenOutput) (*pb.RefreshTokenResponse, error) {
	return &pb.RefreshTokenResponse{
		Message:      output.Message,
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		Success:      output.Success,
	}, nil
}
