package adapter

import (
	"auth-service/pkg/dto"
	"auth-service/pkg/pb"
)

func LoginRequestToInput(req *authpb.LoginRequest) (*dto.LoginInput, error) {
	return &dto.LoginInput{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}, nil
}

func LoginOutputToResponse(output *dto.LoginOutput) (*authpb.LoginResponse, error) {
	return &authpb.LoginResponse{
		Message:      output.Message,
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		Success:      output.Success,
	}, nil
}

func RegisterRequestToInput(req *authpb.RegisterRequest) (*dto.RegisterInput, error) {
	return &dto.RegisterInput{
		Username:        req.GetUsername(),
		Password:        req.GetPassword(),
		Role:            req.GetRole(),
		RoleNotRegister: "seller_employee",
	}, nil
}

func RegisterOutputToResponse(output *dto.RegisterOutput) (*authpb.RegisterResponse, error) {
	return &authpb.RegisterResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func ChangePasswordRequestToInput(req *authpb.ChangePasswordRequest) (*dto.ChangePasswordInput, error) {
	return &dto.ChangePasswordInput{
		Username:    req.GetUsername(),
		OldPassword: req.GetOldPassword(),
		NewPassword: req.GetNewPassword(),
		Role:        req.GetRole(),
	}, nil
}

func ChangePasswordOutputToResponse(output *dto.ChangePasswordOutput) (*authpb.ChangePasswordResponse, error) {
	return &authpb.ChangePasswordResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func RefreshTokenRequestToInput(req *authpb.RefreshTokenRequest) (*dto.RefreshTokenInput, error) {
	return &dto.RefreshTokenInput{
		RefreshToken: req.GetRefreshToken(),
	}, nil
}

func RefreshTokenOutputToResponse(output *dto.RefreshTokenOutput) (*authpb.RefreshTokenResponse, error) {
	return &authpb.RefreshTokenResponse{
		Message:      output.Message,
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		Success:      output.Success,
	}, nil
}

func RegisterSellerRolesRequestToInput(req *authpb.RegisterSellerRolesRequest) (*dto.RegisterSellerRolesInput, error) {
	return &dto.RegisterSellerRolesInput{
		SellerAdminID: req.GetSellerAdminId(),
		Username:      req.GetUsername(),
		Password:      req.GetPassword(),
		Role:          req.GetRole(),
	}, nil
}
func RegisterSellerRolesOutputToResponse(output *dto.RegisterSellerRolesOutput) (*authpb.RegisterSellerRolesResponse, error) {
	return &authpb.RegisterSellerRolesResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func GetStoreIDRoleByIdRequestToInput(req *authpb.GetStoreIDRoleByIDRequest) (*dto.GetStoreIDRoleByIdInput, error) {
	return &dto.GetStoreIDRoleByIdInput{
		ID: req.GetID(),
	}, nil
}
func GetStoreIDRoleByIdOutputToResponse(output *dto.GetStoreIDRoleByIdOutput) (*authpb.GetStoreIDRoleByIDResponse, error) {
	return &authpb.GetStoreIDRoleByIDResponse{
		Message: output.Message,
		Success: output.Success,
		Role:    output.Role,
		StoreId: output.StoreID,
	}, nil
}
