package server

import (
	"auth-service/pkg/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func LoginFailResponse(message string, err error, code codes.Code) (*pb.LoginResponse, error) {
	return &pb.LoginResponse{
		Message:      message,
		AccessToken:  "",
		RefreshToken: "",
		Success:      false,
	}, status.Error(code, err.Error())
}

func RegisterFailResponse(message string, err error, code codes.Code) (*pb.RegisterResponse, error) {
	return &pb.RegisterResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func ChangePasswordFailResponse(message string, err error, code codes.Code) (*pb.ChangePasswordResponse, error) {
	return &pb.ChangePasswordResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func RefreshTokenFailResponse(message string, err error, code codes.Code) (*pb.RefreshTokenResponse, error) {
	return &pb.RefreshTokenResponse{
		Message:      message,
		AccessToken:  "",
		RefreshToken: "",
		Success:      false,
	}, status.Error(code, err.Error())
}
