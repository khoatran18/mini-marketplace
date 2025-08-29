package server

import (
	"context"
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

// CreateProduct handle logic for Create Product gRPC request in Server
func (s *ProductServer) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.CreateProductResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductService: Invalid request for CreateProduct", zap.Error(err))
		return CreProFailResponse("Invalid CreateProduct request", err, codes.InvalidArgument)
	}
	input, err := mapper.CreProRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse CreateProduct request to input error", zap.Error(err))
		return CreProFailResponse("Parse CreateProduct request error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output := s.ProductService.CreateProduct(input)

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.CreProOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse CreateProduct output to response error", zap.Error(err))
		return CreProFailResponse("Parse CreateProduct output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return CreProFailResponse("Invalid CreateProduct response", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// UpdateProduct handle logic for Update Product gRPC request in Server
func (s *ProductServer) UpdateProduct(ctx context.Context, req *pb.UpdateProductRequest) (*pb.UpdateProductResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductService: Invalid request for UpdateProduct", zap.Error(err))
		return UpdProFailResponse("Invalid UpdateProduct request", err, codes.InvalidArgument)
	}
	input, err := mapper.UpdProRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse UpdateProduct request to input error", zap.Error(err))
		return UpdProFailResponse("Parse UpdateProduct request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output := s.ProductService.UpdateProduct(input)

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.UpdProOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse UpdateProduct output to response error", zap.Error(err))
		return UpdProFailResponse("Parse UpdateProduct output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		return UpdProFailResponse("Invalid UpdateProduct response", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetProductByID handle logic for Get Product By ID gRPC request in Server
func (s *ProductServer) GetProductByID(ctx context.Context, req *pb.GetProductByIDRequest) (*pb.GetProductByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductService: Invalid request for GetProByID", zap.Error(err))
		return GetProByIDFailResponse("Invalid GetProByID request", err, codes.InvalidArgument)
	}
	input, err := mapper.GetProByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetProByID request to input error", zap.Error(err))
		return GetProByIDFailResponse("Parse GetProByID request input to request error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output := s.ProductService.GetProductByID(input)

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetProByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetProByID output to response error", zap.Error(err))
		return GetProByIDFailResponse("Parse GetProByID output output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductService: invalid GetProByID response", zap.Error(err))
		return GetProByIDFailResponse("Invalid GetProByID response", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetProductsBySellerID handle logic for Get Products By Seller ID gRPC request in Server
func (s *ProductServer) GetProductsBySellerID(ctx context.Context, req *pb.GetProductsBySellerIDRequest) (*pb.GetProductsBySellerIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductService: Invalid request for GetProsBySelID", zap.Error(err))
		return GetProsBySelIDFailResponse("Invalid GetProsBySelID request", err, codes.InvalidArgument)
	}
	input, err := mapper.GetProsBySelIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetProsBySelID to input error", zap.Error(err))
		return GetProsBySelIDFailResponse("Parse GetProsBySelID input to request error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output := s.ProductService.GetProductsBySellerID(input)

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetProsBySelIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetProsBySelID output to response error", zap.Error(err))
		return GetProsBySelIDFailResponse("Parse GetProsBySelID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductService: invalid GetProsBySelID response", zap.Error(err))
		return GetProsBySelIDFailResponse("Invalid GetProsBySelID response", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetInventoryByID handle logic for Get Inventory By ID gRPC request in Server
func (s *ProductServer) GetInventoryByID(ctx context.Context, req *pb.GetInventoryByIDRequest) (*pb.GetInventoryByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductService: Invalid request for GetInvByID", zap.Error(err))
		return GetInvByIDFailResponse("Invalid GetInvByID request", err, codes.InvalidArgument)
	}
	input, err := mapper.GetInvByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetInvByID to input error", zap.Error(err))
		return GetInvByIDFailResponse("Parse GetInvByID input to request error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output := s.ProductService.GetInventoryByID(input)

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetInvByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetInvByID to output error", zap.Error(err))
		return GetInvByIDFailResponse("Parse GetInvByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductService: invalid GetInvByID response", zap.Error(err))
		return GetInvByIDFailResponse("Invalid GetInvByID response", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetAndDecreaseInventoryByID handle logic for Get And Decrease Inventory By ID gRPC request in Server
func (s *ProductServer) GetAndDecreaseInventoryByID(ctx context.Context, req *pb.GetAndDecreaseInventoryByIDRequest) (*pb.GetAndDecreaseInventoryByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductService: Invalid request for GetAndDecInvByID", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Invalid GetAndDecInvByID request", err, codes.InvalidArgument)
	}
	input, err := mapper.GetAndDecInvByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetAndDecInvByID to input error", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Parse GetAndDecInvByID input to request error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output := s.ProductService.GetAndDecreaseInventoryByID(input)

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetAndDecInvByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductService: parse GetAndDecInvByID output to response error", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Parse GetAndDecInvByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductService: invalid GetAndDecInvByID response", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Invalid GetAndDecInvByID response", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}
