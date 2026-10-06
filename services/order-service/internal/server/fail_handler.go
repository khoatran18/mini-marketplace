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
	}, statusError(code, err)
}

func GetOrdByIDFailResponse(message string, err error, code codes.Code) (*orderpb.GetOrderByIDResponse, error) {
	return &orderpb.GetOrderByIDResponse{
		Message: message,
		Success: false,
		Order:   nil,
	}, statusError(code, err)
}

func GetOrdsByBuyIDStaFailResponse(message string, err error, code codes.Code) (*orderpb.GetOrdersByBuyerIDStatusResponse, error) {
	return &orderpb.GetOrdersByBuyerIDStatusResponse{
		Message: message,
		Success: false,
		Order:   nil,
	}, statusError(code, err)
}

func GetOrdItesByOrdIDFailResponse(message string, err error, code codes.Code) (*orderpb.GetOrderItemsByOrderIDResponse, error) {
	return &orderpb.GetOrderItemsByOrderIDResponse{
		Message:   message,
		Success:   false,
		OrderItem: nil,
	}, statusError(code, err)
}

func UpdOrdByIDFailResponse(message string, err error, code codes.Code) (*orderpb.UpdateOrderByIDResponse, error) {
	return &orderpb.UpdateOrderByIDResponse{
		Massage: message,
		Success: false,
	}, statusError(code, err)
}

func CanOrdByIDFailResponse(message string, err error, code codes.Code) (*orderpb.CancelOrderByIDResponse, error) {
	return &orderpb.CancelOrderByIDResponse{
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
