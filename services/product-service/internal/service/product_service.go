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

// Limits for the catalog fields added with the product-catalog design.
const (
	maxNameLen        = 200
	maxDescriptionLen = 5000
	maxBrandLen       = 100
	maxSKULen         = 64
	maxTags           = 20
	maxTagLen         = 40
	maxImages         = 8
	maxImageURLLen    = 500
)

var validStatuses = map[string]bool{model.StatusDraft: true, model.StatusActive: true, model.StatusHidden: true, model.StatusBanned: true}

// validateCatalogFields checks the optional catalog fields. status may be empty (default applied by the caller).
func validateCatalogFields(name, description, brand, sku, statusValue string, tags, images []string, weight, threshold int64) error {
	switch {
	case len([]rune(name)) > maxNameLen:
		return status.Errorf(codes.InvalidArgument, "name must be at most %d characters", maxNameLen)
	case len([]rune(description)) > maxDescriptionLen:
		return status.Errorf(codes.InvalidArgument, "description must be at most %d characters", maxDescriptionLen)
	case len([]rune(brand)) > maxBrandLen:
		return status.Errorf(codes.InvalidArgument, "brand must be at most %d characters", maxBrandLen)
	case len(sku) > maxSKULen:
		return status.Errorf(codes.InvalidArgument, "sku must be at most %d characters", maxSKULen)
	case len(tags) > maxTags:
		return status.Errorf(codes.InvalidArgument, "at most %d tags", maxTags)
	case len(images) > maxImages:
		return status.Errorf(codes.InvalidArgument, "at most %d images", maxImages)
	case weight < 0 || threshold < 0:
		return status.Error(codes.InvalidArgument, "weight and low stock threshold must not be negative")
	case statusValue != "" && (!validStatuses[statusValue] || statusValue == model.StatusBanned):
		return status.Errorf(codes.InvalidArgument, "status must be one of draft, active, hidden")
	}
	for _, t := range tags {
		if strings.TrimSpace(t) == "" || len([]rune(t)) > maxTagLen {
			return status.Errorf(codes.InvalidArgument, "each tag must be 1-%d characters", maxTagLen)
		}
	}
	for _, u := range images {
		if len(u) == 0 || len(u) > maxImageURLLen || !(strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "/")) {
			return status.Error(codes.InvalidArgument, "image urls must be http(s) URLs or absolute paths")
		}
	}
	return nil
}

// checkCategory verifies that categoryID (when set) refers to an active category.
func (s *ProductService) checkCategory(ctx context.Context, categoryID uint64) error {
	if categoryID == 0 {
		return nil
	}
	ok, err := s.ProductRepo.CategoryExists(ctx, categoryID)
	if err != nil {
		return err
	}
	if !ok {
		return status.Error(codes.InvalidArgument, "category does not exist")
	}
	return nil
}

// mapRepoError converts repository sentinel errors to gRPC status errors (other errors pass through).
func mapRepoError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, repository.ErrCategoryNotFound):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, repository.ErrNotOwner), errors.Is(err, repository.ErrBannedByAdmin):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, repository.ErrInsufficientInventory):
		return status.Error(codes.FailedPrecondition, "inventory can not become negative")
	case errors.Is(err, repository.ErrSlugTaken), errors.Is(err, repository.ErrCategoryCycle):
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if strings.Contains(err.Error(), "idx_seller_sku") {
		return status.Error(codes.AlreadyExists, "sku already used by another product of this store")
	}
	return err
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

	if err := validateCatalogFields(input.Name, input.Description, input.Brand, input.SKU, input.Status, input.Tags, input.ImageURLs, input.WeightG, input.LowStockThreshold); err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, input.CategoryID); err != nil {
		return nil, err
	}
	productStatus := input.Status
	if productStatus == "" {
		productStatus = model.StatusActive
	}

	// Create product
	if len(input.Attributes) == 0 {
		input.Attributes = []byte("{}")
	}
	var product = &model.Product{
		Name:              input.Name,
		Price:             input.Price,
		Inventory:         input.Inventory,
		SellerID:          input.SellerID,
		Attributes:        input.Attributes,
		Description:       input.Description,
		CategoryID:        input.CategoryID,
		Brand:             input.Brand,
		Tags:              adapter.ProductDTOToModel(&dto.Product{Tags: input.Tags}).Tags,
		ImageURLs:         adapter.ProductDTOToModel(&dto.Product{ImageURLs: input.ImageURLs}).ImageURLs,
		Status:            productStatus,
		SKU:               strings.TrimSpace(input.SKU),
		LowStockThreshold: input.LowStockThreshold,
		WeightG:           input.WeightG,
	}

	// Handle in repository
	if err := s.ProductRepo.CreateProduct(ctx, product); err != nil {
		s.ZapLogger.Warn("ProductService: failed to create product", zap.Error(err))
		return nil, mapRepoError(err)
	}
	return &dto.CreateProductOutput{
		Message: "Product created successfully",
		Success: true,
		ID:      product.ID,
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

	if err := validateCatalogFields(input.Product.Name, input.Product.Description, input.Product.Brand, input.Product.SKU, input.Product.Status, input.Product.Tags, input.Product.ImageURLs, input.Product.WeightG, input.Product.LowStockThreshold); err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, input.Product.CategoryID); err != nil {
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
	productModel.SKU = strings.TrimSpace(productModel.SKU)
	if err := s.ProductRepo.UpdateProduct(ctx, productModel); err != nil {
		s.ZapLogger.Warn("ProductService: failed to update product", zap.Error(err))
		return nil, mapRepoError(err)
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
	products, err := s.ProductRepo.GetProductsBySellerID(ctx, input.SellerID, input.OnlyActive)
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
	products, err := s.ProductRepo.GetProducts(ctx, input.Page, input.PageSize, input.OnlyActive)
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
