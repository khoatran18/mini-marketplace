package server

import (
	"auth-service/pkg/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func LoginFailResponse(message string, err error, code codes.Code) (*authpb.LoginResponse, error) {
	return &authpb.LoginResponse{
		Message:      message,
		AccessToken:  "",
		RefreshToken: "",
		Success:      false,
	}, status.Error(code, err.Error())
}

func RegisterFailResponse(message string, err error, code codes.Code) (*authpb.RegisterResponse, error) {
	return &authpb.RegisterResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func ChangePasswordFailResponse(message string, err error, code codes.Code) (*authpb.ChangePasswordResponse, error) {
	return &authpb.ChangePasswordResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func RefreshTokenFailResponse(message string, err error, code codes.Code) (*authpb.RefreshTokenResponse, error) {
	return &authpb.RefreshTokenResponse{
		Message:      message,
		AccessToken:  "",
		RefreshToken: "",
		Success:      false,
	}, status.Error(code, err.Error())
}

func RegisterSellerRolesFailResponse(message string, err error, code codes.Code) (*authpb.RegisterSellerRolesResponse, error) {
	return &authpb.RegisterSellerRolesResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func GetStoreIDRoleByIdFailResponse(message string, err error, code codes.Code) (*authpb.GetStoreIDRoleByIDResponse, error) {
	return &authpb.GetStoreIDRoleByIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}
