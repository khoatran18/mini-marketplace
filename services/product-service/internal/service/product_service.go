package service

import (
	"fmt"
	"product-service/internal/repository"
	"product-service/internal/service/adapter"
	"product-service/pkg/dto"
	"product-service/pkg/model"

	"go.uber.org/zap"
)

type ProductService struct {
	ProductRepo *repository.ProductRepository
	ZapLogger   *zap.Logger
}

// NewProductService create new ProductService
func NewProductService(productRepo *repository.ProductRepository, logger *zap.Logger) *ProductService {
	return &ProductService{
		ProductRepo: productRepo,
		ZapLogger:   logger,
	}
}

// CreateProduct handle logic for Create Product gRPC request in Service
func (s *ProductService) CreateProduct(input *dto.CreateProductInput) (*dto.CreateProductOutput, error) {

	// Create product
	var product = &model.Product{
		Name:       input.Name,
		Price:      input.Price,
		Inventory:  input.Inventory,
		SellerID:   input.SellerID,
		Attributes: input.Attributes,
	}

	// Handle in repository
	if err := s.ProductRepo.CreateProduct(product); err != nil {
		s.ZapLogger.Warn("ProductService: failed to create product", zap.Error(err))
		return nil, err
	}
	return &dto.CreateProductOutput{
		Message: "Product created successfully",
		Success: true,
	}, nil
}

// UpdateProduct handle logic for Update Product gRPC request in Service
func (s *ProductService) UpdateProduct(input *dto.UpdateProductInput) (*dto.UpdateProductOutput, error) {

	// Check if product not existed
	oldProduct, err := s.ProductRepo.GetProductByID(input.Product.ID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get old product", zap.Error(err))
		return nil, err
	}

	// Check user is owner of product
	if oldProduct.SellerID != input.UserID {
		s.ZapLogger.Error("ProductService: user is not owner of product", zap.Any("old", oldProduct.SellerID))
		return &dto.UpdateProductOutput{
			Message: fmt.Sprintf("Seller is not owner of product"),
			Success: false,
		}, nil
	}

	// Parse ProductModel to Product DTO
	productModel := adapter.ProductDTOToModel(input.Product)
	if err := s.ProductRepo.UpdateProduct(productModel); err != nil {
		s.ZapLogger.Warn("ProductService: failed to update product", zap.Error(err))
		return nil, err
	}
	return &dto.UpdateProductOutput{
		Message: "Product updated successfully",
		Success: true,
	}, nil
}

// GetProductByID handle logic for Get Product By ID gRPC request in Service
func (s *ProductService) GetProductByID(input *dto.GetProductByIDInput) (*dto.GetProductByIDOutput, error) {

	// Get product
	product, err := s.ProductRepo.GetProductByID(input.ID)
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

// GetProductsBySellerID handle logic for Get Products By Seller ID gRPC request in Service
func (s *ProductService) GetProductsBySellerID(input *dto.GetProductsBySellerIDInput) (*dto.GetProductsBySellerIDOutput, error) {

	// Get products
	products, err := s.ProductRepo.GetProductsBySellerID(input.SellerID)
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
func (s *ProductService) GetInventoryByID(input *dto.GetInventoryByIDInput) (*dto.GetInventoryByIDOutput, error) {

	// Get inventory
	product, err := s.ProductRepo.GetProductByID(input.ID)
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
func (s *ProductService) GetAndDecreaseInventoryByID(input *dto.GetAndDecreaseInventoryByIDInput) (*dto.GetAndDecreaseInventoryByIDOutput, error) {

	// Get and decrease inventory
	if err := s.ProductRepo.GetAndDecreaseInventoryByID(input.ID, input.Quantity); err != nil {
		s.ZapLogger.Warn("ProductService: failed to decrease inventory", zap.Error(err))
		return nil, err
	}
	return &dto.GetAndDecreaseInventoryByIDOutput{
		Message: "Product decreased successfully",
		Success: true,
	}, nil
}
