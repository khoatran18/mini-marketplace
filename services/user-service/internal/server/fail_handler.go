package server

import (
	userpb "user-service/pkg/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func CreBuyFailResponse(message string, err error, code codes.Code) (*userpb.CreateBuyerResponse, error) {
	return &userpb.CreateBuyerResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func UpdBuyByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.UpdateBuyerByUserIDResponse, error) {
	return &userpb.UpdateBuyerByUserIDResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func GetBuyByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.GetBuyerByUserIDResponse, error) {
	return &userpb.GetBuyerByUserIDResponse{
		Message: message,
		Success: false,
		Buyer:   nil,
	}, statusError(code, err)
}

func CreSelFailResponse(message string, err error, code codes.Code) (*userpb.CreateSellerResponse, error) {
	return &userpb.CreateSellerResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func UpdSelByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.UpdateSellerByIDResponse, error) {
	return &userpb.UpdateSellerByIDResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func GetSelByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.GetSellerByIDResponse, error) {
	return &userpb.GetSellerByIDResponse{
		Message: message,
		Success: false,
		Seller:  nil,
	}, statusError(code, err)
}

func DelBuyByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.DelBuyerByUserIDResponse, error) {
	return &userpb.DelBuyerByUserIDResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func DelSelByIDFailResponse(message string, err error, code codes.Code) (*userpb.DelSellerByIDResponse, error) {
	return &userpb.DelSellerByIDResponse{
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
