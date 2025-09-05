package server

import (
	"product-service/pkg/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func CreProFailResponse(message string, err error, code codes.Code) (*productpb.CreateProductResponse, error) {
	return &productpb.CreateProductResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func UpdProFailResponse(message string, err error, code codes.Code) (*productpb.UpdateProductResponse, error) {
	return &productpb.UpdateProductResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func GetProByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetProductByIDResponse, error) {
	return &productpb.GetProductByIDResponse{
		Message: message,
		Success: false,
		Product: nil,
	}, status.Error(code, err.Error())
}

func GetProsByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetProductsByIDResponse, error) {
	return &productpb.GetProductsByIDResponse{
		Message: message,
		Success: false,
		Product: nil,
	}, status.Error(code, err.Error())
}

func GetProsBySelIDFailResponse(message string, err error, code codes.Code) (*productpb.GetProductsBySellerIDResponse, error) {
	return &productpb.GetProductsBySellerIDResponse{
		Message:  message,
		Success:  false,
		Products: nil,
	}, status.Error(code, err.Error())
}

func GetInvByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetInventoryByIDResponse, error) {
	return &productpb.GetInventoryByIDResponse{
		Message:   message,
		Success:   false,
		Inventory: 0,
	}, status.Error(code, err.Error())
}

func GetAndDecInvByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetAndDecreaseInventoryByIDResponse, error) {
	return &productpb.GetAndDecreaseInventoryByIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}
