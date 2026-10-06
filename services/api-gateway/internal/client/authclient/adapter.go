package authclient

import (
	"api-gateway/pkg/dto"
	authpb "api-gateway/pkg/pb/authservice"
)

func LoginInputToRequest(input *dto.LoginInput) (*authpb.LoginRequest, error) {
	return &authpb.LoginRequest{
		Username: input.Username,
		Password: input.Password,
		Role:     input.Role,
	}, nil
}
func LoginResponseToOutput(response *authpb.LoginResponse) (*dto.LoginOutput, error) {
	return &dto.LoginOutput{
		Message:      response.GetMessage(),
		AccessToken:  response.GetAccessToken(),
		RefreshToken: response.GetRefreshToken(),
		Success:      response.GetSuccess(),
	}, nil
}

func RegisterInputToRequest(input *dto.RegisterInput) (*authpb.RegisterRequest, error) {
	return &authpb.RegisterRequest{
		Username: input.Username,
		Password: input.Password,
		Role:     input.Role,
	}, nil
}
func RegisterResponseToOutput(response *authpb.RegisterResponse) (*dto.RegisterOutput, error) {
	return &dto.RegisterOutput{
		Message: response.GetMessage(),
		Success: response.GetSuccess(),
	}, nil
}

func ChangPasswordInputToRequest(input *dto.ChangePasswordInput) (*authpb.ChangePasswordRequest, error) {
	return &authpb.ChangePasswordRequest{
		Username:    input.Username,
		OldPassword: input.OldPassword,
		NewPassword: input.NewPassword,
		Role:        input.Role,
	}, nil
}
func ChangePasswordResponseToOutput(response *authpb.ChangePasswordResponse) (*dto.ChangePasswordOutput, error) {
	return &dto.ChangePasswordOutput{
		Message: response.GetMessage(),
		Success: response.GetSuccess(),
	}, nil
}

func RefreshTokenInputToRequest(input *dto.RefreshTokenInput) (*authpb.RefreshTokenRequest, error) {
	return &authpb.RefreshTokenRequest{
		RefreshToken: input.RefreshToken,
	}, nil
}
func RefreshTokenResponseToOutput(response *authpb.RefreshTokenResponse) (*dto.RefreshTokenOutput, error) {
	return &dto.RefreshTokenOutput{
		Message:      response.GetMessage(),
		AccessToken:  response.GetAccessToken(),
		RefreshToken: response.GetRefreshToken(),
		Success:      response.GetSuccess(),
	}, nil
}

func RegisterSellerRolesInputToRequest(input *dto.RegisterSellerRolesInput) (*authpb.RegisterSellerRolesRequest, error) {
	return &authpb.RegisterSellerRolesRequest{
		SellerAdminId: input.SellerAdminID,
		Username:      input.Username,
		Password:      input.Password,
		Role:          input.Role,
	}, nil
}
func RegisterSellerRolesResponseToOutput(response *authpb.RegisterSellerRolesResponse) (*dto.RegisterSellerRolesOutput, error) {
	return &dto.RegisterSellerRolesOutput{
		Message: response.GetMessage(),
		Success: response.GetSuccess(),
	}, nil
}
