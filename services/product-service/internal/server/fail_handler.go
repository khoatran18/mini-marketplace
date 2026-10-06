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
	}, statusError(code, err)
}

func UpdProFailResponse(message string, err error, code codes.Code) (*productpb.UpdateProductResponse, error) {
	return &productpb.UpdateProductResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func GetProByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetProductByIDResponse, error) {
	return &productpb.GetProductByIDResponse{
		Message: message,
		Success: false,
		Product: nil,
	}, statusError(code, err)
}

func GetProsByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetProductsByIDResponse, error) {
	return &productpb.GetProductsByIDResponse{
		Message: message,
		Success: false,
		Product: nil,
	}, statusError(code, err)
}

func GetProsBySelIDFailResponse(message string, err error, code codes.Code) (*productpb.GetProductsBySellerIDResponse, error) {
	return &productpb.GetProductsBySellerIDResponse{
		Message:  message,
		Success:  false,
		Products: nil,
	}, statusError(code, err)
}

func GetInvByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetInventoryByIDResponse, error) {
	return &productpb.GetInventoryByIDResponse{
		Message:   message,
		Success:   false,
		Inventory: 0,
	}, statusError(code, err)
}

func GetAndDecInvByIDFailResponse(message string, err error, code codes.Code) (*productpb.GetAndDecreaseInventoryByIDResponse, error) {
	return &productpb.GetAndDecreaseInventoryByIDResponse{
		Message: message,
		Success: false,
	}, statusError(code, err)
}

func GetProductsFailResponse(message string, err error, code codes.Code) (*productpb.GetProductsResponse, error) {
	return &productpb.GetProductsResponse{
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
