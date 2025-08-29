package server

import (
	"context"
	"log"
	"product-service/internal/service"
	"product-service/pkg/mapper"
	"product-service/pkg/pb"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

type ProductServer struct {
	pb.UnimplementedProductServiceServer
	ProductService *service.ProductService
	ZapLogger      *zap.Logger
}

func (s *ProductServer) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.CreateProductResponse, error) {

	// Validate request
	if err := protovalidate.Validate(req); err != nil {
		return CreProFailResponse("Invalid arguments", err, codes.InvalidArgument)
	}

	// Parse and get response
	input, err := mapper.CreProRequestToInput(req)
	if err != nil {
		return CreProFailResponse("Invalid arguments or parse to Service Input error", err, codes.InvalidArgument)
	}
	output := s.ProductService.CreateProduct(input)
	res, err := mapper.CreProOutputToResponse(output)
	if err != nil {
		return CreProFailResponse("Parse output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return CreProFailResponse("Response not valid", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *ProductServer) UpdateProduct(ctx context.Context, req *pb.UpdateProductRequest) (*pb.UpdateProductResponse, error) {

	// Validate request
	if err := protovalidate.Validate(req); err != nil {
		return UpdProFailResponse("Invalid arguments", err, codes.InvalidArgument)
	}

	// Parse and get response
	input, err := mapper.UpdProRequestToInput(req)
	if err != nil {
		return UpdProFailResponse("Invalid arguments or parse to Service Input error", err, codes.InvalidArgument)
	}
	output := s.ProductService.UpdateProduct(input)
	res, err := mapper.UpdProOutputToResponse(output)
	if err != nil {
		return UpdProFailResponse("Parse output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return UpdProFailResponse("Response not valid", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *ProductServer) GetProductByID(ctx context.Context, req *pb.GetProductByIDRequest) (*pb.GetProductByIDResponse, error) {

	// Validate
	if err := protovalidate.Validate(req); err != nil {
		return GetProByIDFailResponse("Invalid arguments", err, codes.InvalidArgument)
	}

	// Parse and get response
	input, err := mapper.GetProByIDRequestToInput(req)
	if err != nil {
		return GetProByIDFailResponse("Parse input to request error", err, codes.InvalidArgument)
	}
	output := s.ProductService.GetProductByID(input)
	res, err := mapper.GetProByIDOutputToResponse(output)
	if err != nil {
		return GetProByIDFailResponse("Parse output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return GetProByIDFailResponse("Response not valid", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *ProductServer) GetProductsBySellerID(ctx context.Context, req *pb.GetProductsBySellerIDRequest) (*pb.GetProductsBySellerIDResponse, error) {

	// Validate
	if err := protovalidate.Validate(req); err != nil {
		log.Printf("Invalid arguments: %v", err)
		return GetProsBySelIDFailResponse("Invalid arguments", err, codes.InvalidArgument)
	}

	// Parse and get response
	input, err := mapper.GetProsBySelIDRequestToInput(req)
	if err != nil {
		log.Printf("Parse input to request error: %v", err)
		return GetProsBySelIDFailResponse("Parse input to request error", err, codes.InvalidArgument)
	}
	output := s.ProductService.GetProductsBySellerID(input)
	res, err := mapper.GetProsBySelIDOutputToResponse(output)
	if err != nil {
		log.Printf("Parse output to response error: %v", err)
		return GetProsBySelIDFailResponse("Parse output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		log.Printf("Response not valid")
		return GetProsBySelIDFailResponse("Response not valid", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *ProductServer) GetInventoryByID(ctx context.Context, req *pb.GetInventoryByIDRequest) (*pb.GetInventoryByIDResponse, error) {

	// Validate
	if err := protovalidate.Validate(req); err != nil {
		return GetInvByIDFailResponse("Invalid arguments", err, codes.InvalidArgument)
	}

	// Parse and get response
	input, err := mapper.GetInvByIDRequestToInput(req)
	if err != nil {
		return GetInvByIDFailResponse("Parse input to request error", err, codes.InvalidArgument)
	}
	output := s.ProductService.GetInventoryByID(input)
	res, err := mapper.GetInvByIDOutputToResponse(output)
	if err != nil {
		return GetInvByIDFailResponse("Parse output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return GetInvByIDFailResponse("Response not valid", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *ProductServer) GetAndDecreaseInventoryByID(ctx context.Context, req *pb.GetAndDecreaseInventoryByIDRequest) (*pb.GetAndDecreaseInventoryByIDResponse, error) {

	// Validate
	if err := protovalidate.Validate(req); err != nil {
		return GetAndDecInvByIDFailResponse("Invalid arguments", err, codes.InvalidArgument)
	}

	// Parse and get response
	input, err := mapper.GetAndDecInvByIDRequestToInput(req)
	if err != nil {
		return GetAndDecInvByIDFailResponse("Parse input to request error", err, codes.InvalidArgument)
	}
	output := s.ProductService.GetAndDecreaseInventoryByID(input)
	res, err := mapper.GetAndDecInvByIDOutputToResponse(output)
	if err != nil {
		return GetAndDecInvByIDFailResponse("Parse output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return GetAndDecInvByIDFailResponse("Response not valid", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}
