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
	}, statusError(code, err)
}

func RegisterFailResponse(message string, err error, code codes.Code) (*authpb.RegisterResponse, error) {
	return &authpb.RegisterResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func ChangePasswordFailResponse(message string, err error, code codes.Code) (*authpb.ChangePasswordResponse, error) {
	return &authpb.ChangePasswordResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func RefreshTokenFailResponse(message string, err error, code codes.Code) (*authpb.RefreshTokenResponse, error) {
	return &authpb.RefreshTokenResponse{
		Message:      message,
		AccessToken:  "",
		RefreshToken: "",
		Success:      false,
	}, statusError(code, err)
}

func RegisterSellerRolesFailResponse(message string, err error, code codes.Code) (*authpb.RegisterSellerRolesResponse, error) {
	return &authpb.RegisterSellerRolesResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func GetStoreIDRoleByIdFailResponse(message string, err error, code codes.Code) (*authpb.GetStoreIDRoleByIDResponse, error) {
	return &authpb.GetStoreIDRoleByIDResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

// statusError keeps the gRPC code chosen by the service layer (e.g. NotFound) and
// falls back to code for plain errors.
func statusError(code codes.Code, err error) error {
	if st, ok := status.FromError(err); ok {
		return st.Err()
	}
	return status.Error(code, err.Error())
}
