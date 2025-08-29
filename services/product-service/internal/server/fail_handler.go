package server

import (
	"product-service/pkg/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func CreProFailResponse(message string, err error, code codes.Code) (*pb.CreateProductResponse, error) {
	return &pb.CreateProductResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func UpdProFailResponse(message string, err error, code codes.Code) (*pb.UpdateProductResponse, error) {
	return &pb.UpdateProductResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func GetProsBySelIDFailResponse(message string, err error, code codes.Code) (*pb.GetProductsBySellerIDResponse, error) {
	return &pb.GetProductsBySellerIDResponse{
		Message:  message,
		Success:  false,
		Products: nil,
	}, status.Error(code, err.Error())
}

func GetProByIDFailResponse(message string, err error, code codes.Code) (*pb.GetProductByIDResponse, error) {
	return &pb.GetProductByIDResponse{
		Message: message,
		Success: false,
		Product: nil,
	}, status.Error(code, err.Error())
}

func GetInvByIDFailResponse(message string, err error, code codes.Code) (*pb.GetInventoryByIDResponse, error) {
	return &pb.GetInventoryByIDResponse{
		Message:   message,
		Success:   false,
		Inventory: 0,
	}, status.Error(code, err.Error())
}

func GetAndDecInvByIDFailResponse(message string, err error, code codes.Code) (*pb.GetAndDecreaseInventoryByIDResponse, error) {
	return &pb.GetAndDecreaseInventoryByIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}
