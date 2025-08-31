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
		s.ZapLogger.Warn("ProductServer: invalid request for CreateProduct", zap.Error(err))
		return CreProFailResponse("Invalid request for CreateProduct", err, codes.InvalidArgument)
	}
	input, err := mapper.CreProRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse CreateProduct request to input error", zap.Error(err))
		return CreProFailResponse("Parse CreateProduct request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.ProductService.CreateProduct(input)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: CreateProduct error in ProductService", zap.Error(err))
		return CreProFailResponse("CreateProduct error in ProductService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.CreProOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("ProductServer: parse CreateProduct output to response error", zap.Error(err))
		return CreProFailResponse("Parse CreateProduct output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid response for CreateProduct", zap.Error(err))
		return CreProFailResponse("Invalid response for CreateProduct", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// UpdateProduct handle logic for Update Product gRPC request in Server
func (s *ProductServer) UpdateProduct(ctx context.Context, req *pb.UpdateProductRequest) (*pb.UpdateProductResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid request for UpdateProduct", zap.Error(err))
		return UpdProFailResponse("Invalid request for UpdateProduct", err, codes.InvalidArgument)
	}
	input, err := mapper.UpdProRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse UpdateProduct request to input error", zap.Error(err))
		return UpdProFailResponse("Parse UpdateProduct request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.ProductService.UpdateProduct(input)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: UpdateProduct error in ProductService", zap.Error(err))
		return UpdProFailResponse("UpdateProduct error in ProductService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.UpdProOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse UpdateProduct output to response error", zap.Error(err))
		return UpdProFailResponse("Parse UpdateProduct output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid response for UpdateProduct", zap.Error(err))
		return UpdProFailResponse("Invalid response for UpdateProduct", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetProductByID handle logic for Get Product By ID gRPC request in Server
func (s *ProductServer) GetProductByID(ctx context.Context, req *pb.GetProductByIDRequest) (*pb.GetProductByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid request for GetProByID", zap.Error(err))
		return GetProByIDFailResponse("Invalid request for GetProByID", err, codes.InvalidArgument)
	}
	input, err := mapper.GetProByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetProByID request to input error", zap.Error(err))
		return GetProByIDFailResponse("Parse GetProByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.ProductService.GetProductByID(input)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: GetProByID error in ProductService", zap.Error(err))
		return GetProByIDFailResponse("GetProByID error in ProductService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetProByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetProByID output to response error", zap.Error(err))
		return GetProByIDFailResponse("Parse GetProByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid response for GetProByID", zap.Error(err))
		return GetProByIDFailResponse("Invalid response for GetProByID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetProductsBySellerID handle logic for Get Products By Seller ID gRPC request in Server
func (s *ProductServer) GetProductsBySellerID(ctx context.Context, req *pb.GetProductsBySellerIDRequest) (*pb.GetProductsBySellerIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid request for GetProsBySelID", zap.Error(err))
		return GetProsBySelIDFailResponse("Invalid request for GetProsBySelID", err, codes.InvalidArgument)
	}
	input, err := mapper.GetProsBySelIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetProsBySelID request to input error", zap.Error(err))
		return GetProsBySelIDFailResponse("Parse GetProsBySelID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.ProductService.GetProductsBySellerID(input)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: GetProsBySelID error in ProductService", zap.Error(err))
		return GetProsBySelIDFailResponse("GetProsBySelID error in ProductService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetProsBySelIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetProsBySelID output to response error", zap.Error(err))
		return GetProsBySelIDFailResponse("Parse GetProsBySelID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid response for GetProsBySelID", zap.Error(err))
		return GetProsBySelIDFailResponse("Invalid response for GetProsBySelID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetInventoryByID handle logic for Get Inventory By ID gRPC request in Server
func (s *ProductServer) GetInventoryByID(ctx context.Context, req *pb.GetInventoryByIDRequest) (*pb.GetInventoryByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid request for GetInvByID", zap.Error(err))
		return GetInvByIDFailResponse("Invalid request for GetInvByID", err, codes.InvalidArgument)
	}
	input, err := mapper.GetInvByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetInvByID request to input error", zap.Error(err))
		return GetInvByIDFailResponse("Parse GetInvByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.ProductService.GetInventoryByID(input)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: GetInvByID error in ProductService", zap.Error(err))
		return GetInvByIDFailResponse("GetInvByID error in ProductService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetInvByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetInvByID output to response error", zap.Error(err))
		return GetInvByIDFailResponse("Parse GetInvByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid response for GetInvByID", zap.Error(err))
		return GetInvByIDFailResponse("Invalid response for GetInvByID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

// GetAndDecreaseInventoryByID handle logic for Get And Decrease Inventory By ID gRPC request in Server
func (s *ProductServer) GetAndDecreaseInventoryByID(ctx context.Context, req *pb.GetAndDecreaseInventoryByIDRequest) (*pb.GetAndDecreaseInventoryByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid request for GetAndDecInvByID", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Invalid request for GetAndDecInvByID", err, codes.InvalidArgument)
	}
	input, err := mapper.GetAndDecInvByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetAndDecInvByID request to input error", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Parse GetAndDecInvByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.ProductService.GetAndDecreaseInventoryByID(input)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: GetAndDecInvByID error in ProductService", zap.Error(err))
		return GetAndDecInvByIDFailResponse("GetAndDecInvByID error in ProductService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := mapper.GetAndDecInvByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("ProductServer: parse GetAndDecInvByID output to response error", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Parse GetAndDecInvByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("ProductServer: invalid response for GetAndDecInvByID", zap.Error(err))
		return GetAndDecInvByIDFailResponse("Invalid response for", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}
