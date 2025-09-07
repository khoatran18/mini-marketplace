package server

import (
	orderpb "order-service/pkg/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func CreOrdFailResponse(message string, err error, code codes.Code) (*orderpb.CreateOrderResponse, error) {
	return &orderpb.CreateOrderResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func GetOrdByIDFailResponse(message string, err error, code codes.Code) (*orderpb.GetOrderByIDResponse, error) {
	return &orderpb.GetOrderByIDResponse{
		Message: message,
		Success: false,
		Order:   nil,
	}, status.Error(code, err.Error())
}

func GetOrdsByBuyIDStaFailResponse(message string, err error, code codes.Code) (*orderpb.GetOrdersByBuyerIDStatusResponse, error) {
	return &orderpb.GetOrdersByBuyerIDStatusResponse{
		Message: message,
		Success: false,
		Order:   nil,
	}, status.Error(code, err.Error())
}

func GetOrdItesByOrdIDFailResponse(message string, err error, code codes.Code) (*orderpb.GetOrderItemsByOrderIDResponse, error) {
	return &orderpb.GetOrderItemsByOrderIDResponse{
		Message:   message,
		Success:   false,
		OrderItem: nil,
	}, status.Error(code, err.Error())
}

func UpdOrdByIDFailResponse(message string, err error, code codes.Code) (*orderpb.UpdateOrderByIDResponse, error) {
	return &orderpb.UpdateOrderByIDResponse{
		Massage: message,
		Success: false,
	}, status.Error(code, err.Error())
}

func CanOrdByIDFailResponse(message string, err error, code codes.Code) (*orderpb.CancelOrderByIDResponse, error) {
	return &orderpb.CancelOrderByIDResponse{
		Message: message,
		Success: false,
	}, status.Error(code, err.Error())
}
