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
	}, status.Error(code, err.Error())
}

func UpdBuyByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.UpdateBuyerByUserIDResponse, error) {
	return &userpb.UpdateBuyerByUserIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func GetBuyByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.GetBuyerByUserIDResponse, error) {
	return &userpb.GetBuyerByUserIDResponse{
		Message: message,
		Success: false,
		Buyer:   nil,
	}, status.Error(code, err.Error())
}

func CreSelFailResponse(message string, err error, code codes.Code) (*userpb.CreateSellerResponse, error) {
	return &userpb.CreateSellerResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func UpdSelByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.UpdateSellerByUserIDResponse, error) {
	return &userpb.UpdateSellerByUserIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func GetSelByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.GetSellerByUserIDResponse, error) {
	return &userpb.GetSellerByUserIDResponse{
		Message: message,
		Success: false,
		Seller:  nil,
	}, status.Error(code, err.Error())
}

func DelBuyByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.DelBuyerByUserIDResponse, error) {
	return &userpb.DelBuyerByUserIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func DelSelByUseIDFailResponse(message string, err error, code codes.Code) (*userpb.DelSellerByUserIDResponse, error) {
	return &userpb.DelSellerByUserIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}
