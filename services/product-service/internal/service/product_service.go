package service

import (
	"context"
	"errors"
	"fmt"
	"product-service/internal/config/messagequeue"
	"product-service/internal/config/messagequeue/kafkaimpl"
	"product-service/internal/repository"
	"product-service/internal/service/adapter"
	"product-service/pkg/dto"
	"product-service/pkg/model"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func validateProductFields(name string, price float64, inventory int64) error {
	if strings.TrimSpace(name) == "" {
		return status.Error(codes.InvalidArgument, "name is required")
	}
	if price <= 0 {
		return status.Error(codes.InvalidArgument, "price must be positive")
	}
	if inventory < 0 {
		return status.Error(codes.InvalidArgument, "inventory must not be negative")
	}
	return nil
}

type ProductService struct {
	ProductRepo *repository.ProductRepository
	MQProducer  messagequeue.Producer
	MQConsumer  messagequeue.Consumer
	KafkaClient *kafkaimpl.KafkaClient
	ZapLogger   *zap.Logger
}

// NewProductService create new ProductService
func NewProductService(productRepo *repository.ProductRepository, logger *zap.Logger,
	producer messagequeue.Producer, consumer messagequeue.Consumer, kafkaClient *kafkaimpl.KafkaClient) *ProductService {
	return &ProductService{
		ProductRepo: productRepo,
		MQProducer:  producer,
		MQConsumer:  consumer,
		KafkaClient: kafkaClient,
		ZapLogger:   logger,
	}
}

// CreateProduct handle logic for Create Product gRPC request in Service
func (s *ProductService) CreateProduct(ctx context.Context, input *dto.CreateProductInput) (*dto.CreateProductOutput, error) {

	if err := validateProductFields(input.Name, input.Price, input.Inventory); err != nil {
		return nil, err
	}
	if input.SellerID == 0 {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}

	// Create product
	var product = &model.Product{
		Name:       input.Name,
		Price:      input.Price,
		Inventory:  input.Inventory,
		SellerID:   input.SellerID,
		Attributes: input.Attributes,
	}

	// Handle in repository
	if err := s.ProductRepo.CreateProduct(ctx, product); err != nil {
		s.ZapLogger.Warn("ProductService: failed to create product", zap.Error(err))
		return nil, err
	}
	return &dto.CreateProductOutput{
		Message: "Product created successfully",
		Success: true,
	}, nil
}

// UpdateProduct handle logic for Update Product gRPC request in Service
func (s *ProductService) UpdateProduct(ctx context.Context, input *dto.UpdateProductInput) (*dto.UpdateProductOutput, error) {

	if input.Product == nil {
		return nil, status.Error(codes.InvalidArgument, "product is required")
	}
	if err := validateProductFields(input.Product.Name, input.Product.Price, input.Product.Inventory); err != nil {
		return nil, err
	}

	// Check if product not existed
	oldProduct, err := s.ProductRepo.GetProductByID(ctx, input.Product.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		s.ZapLogger.Warn("ProductService: failed to get old product", zap.Error(err))
		return nil, err
	}

	// input.UserID carries the caller's store (seller) ID, resolved by the gateway from the token.
	if input.UserID == 0 || oldProduct.SellerID != input.UserID {
		s.ZapLogger.Warn("ProductService: caller is not owner of product", zap.Uint64("owner", oldProduct.SellerID), zap.Uint64("caller", input.UserID))
		return nil, status.Error(codes.PermissionDenied, "seller is not owner of product")
	}

	// Parse ProductModel to Product DTO
	productModel := adapter.ProductDTOToModel(input.Product)
	if err := s.ProductRepo.UpdateProduct(ctx, productModel); err != nil {
		s.ZapLogger.Warn("ProductService: failed to update product", zap.Error(err))
		return nil, err
	}
	return &dto.UpdateProductOutput{
		Message: "Product updated successfully",
		Success: true,
	}, nil
}

// GetProductByID handle logic for Get Product By ID gRPC request in Service
func (s *ProductService) GetProductByID(ctx context.Context, input *dto.GetProductByIDInput) (*dto.GetProductByIDOutput, error) {

	// Get product
	product, err := s.ProductRepo.GetProductByID(ctx, input.ID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get product", zap.Error(err))
		return nil, err
	}

	// Parse ProductModel to ProductDTO
	productDTO := adapter.ProductModelToDTO(product)
	return &dto.GetProductByIDOutput{
		Message: fmt.Sprintf("Get product with id %v successfully", productDTO.ID),
		Success: true,
		Product: productDTO,
	}, nil
}

// GetProductsByID handle logic for Get Products By ID gRPC request in Service
func (s *ProductService) GetProductsByID(ctx context.Context, input *dto.GetProductsByIDInput) (*dto.GetProductsByIDOutput, error) {

	// Get product
	products, err := s.ProductRepo.GetProductsByID(ctx, input.IDs)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get products", zap.Error(err))
		return nil, err
	}

	// Parse ProductModel to ProductDTO
	productsDTO := adapter.ProductsModelToDTO(products)
	return &dto.GetProductsByIDOutput{
		Message:  "Get products by ID successfully",
		Success:  true,
		Products: productsDTO,
	}, nil
}

// GetProductsBySellerID handle logic for Get Products By Seller ID gRPC request in Service
func (s *ProductService) GetProductsBySellerID(ctx context.Context, input *dto.GetProductsBySellerIDInput) (*dto.GetProductsBySellerIDOutput, error) {

	// Get products
	products, err := s.ProductRepo.GetProductsBySellerID(ctx, input.SellerID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get products", zap.Error(err))
		return nil, err
	}

	// Parse ProductsModel to ProductsDTO
	productsDTO := adapter.ProductsModelToDTO(products)
	return &dto.GetProductsBySellerIDOutput{
		Message:  fmt.Sprintf("Get products by sellerID %v", input.SellerID),
		Success:  true,
		Products: productsDTO,
	}, nil
}

// GetInventoryByID handle logic for Get Inventory By ID gRPC request in Service
func (s *ProductService) GetInventoryByID(ctx context.Context, input *dto.GetInventoryByIDInput) (*dto.GetInventoryByIDOutput, error) {

	// Get inventory
	product, err := s.ProductRepo.GetProductByID(ctx, input.ID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get product inventory", zap.Error(err))
		return nil, err
	}
	return &dto.GetInventoryByIDOutput{
		Message:   fmt.Sprintf("Get product with id %v successfully", product.ID),
		Success:   true,
		Inventory: product.Inventory,
	}, nil
}

// GetAndDecreaseInventoryByID handle logic for Get And Decrease Inventory By ID gRPC request in Service
func (s *ProductService) GetAndDecreaseInventoryByID(ctx context.Context, input *dto.GetAndDecreaseInventoryByIDInput) (*dto.GetAndDecreaseInventoryByIDOutput, error) {

	// Get and decrease inventory
	if err := s.ProductRepo.GetAndDecreaseInventoryByID(ctx, input.ID, input.Quantity); err != nil {
		s.ZapLogger.Warn("ProductService: failed to decrease inventory", zap.Error(err))
		return nil, err
	}
	return &dto.GetAndDecreaseInventoryByIDOutput{
		Message: "Product decreased successfully",
		Success: true,
	}, nil
}

// GetProducts handle logic for Get Products gRPC request in Service
func (s *ProductService) GetProducts(ctx context.Context, input *dto.GetProductsInput) (*dto.GetProductsOutput, error) {

	// Get Products
	products, err := s.ProductRepo.GetProducts(ctx, input.Page, input.PageSize)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get products", zap.Error(err))
		return nil, err
	}
	productsDTO := adapter.ProductsModelToDTO(products)
	return &dto.GetProductsOutput{
		Message:  "Get products successfully",
		Success:  true,
		Products: productsDTO,
	}, nil
}
