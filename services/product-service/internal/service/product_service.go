package service

import (
	"fmt"
	"product-service/internal/repository"
	"product-service/pkg/dto"
	"product-service/pkg/mapper"
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
func (s *ProductService) CreateProduct(req *dto.CreateProductInput) *dto.CreateProductOutput {

	// Create product
	var product = &model.Product{
		Name:       req.Name,
		Price:      req.Price,
		Inventory:  req.Inventory,
		SellerID:   req.SellerID,
		Attributes: req.Attributes,
	}

	// Handle in repository
	if err := s.ProductRepo.CreateProduct(product); err != nil {
		s.ZapLogger.Warn("ProductService: failed to create product", zap.Error(err))
		return &dto.CreateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	return &dto.CreateProductOutput{
		Message: "Product created successfully",
		Success: true,
	}
}

// UpdateProduct handle logic for Update Product gRPC request in Service
func (s *ProductService) UpdateProduct(req *dto.UpdateProductInput) *dto.UpdateProductOutput {

	// Check if product not existed
	oldProduct, err := s.ProductRepo.GetProductByID(req.Product.ID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get old product", zap.Error(err))
		return &dto.UpdateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}

	// Check user is owner of product
	if oldProduct.SellerID != req.UserID {
		s.ZapLogger.Error("ProductService: user is not owner of product", zap.Any("old", oldProduct.SellerID))
		return &dto.UpdateProductOutput{
			Message: fmt.Sprintf("Seller is not owner of product"),
			Success: false,
		}
	}

	// Parse ProductModel to Product DTO
	productModel := mapper.ProductDTOToModel(req.Product)
	if err := s.ProductRepo.UpdateProduct(productModel); err != nil {
		s.ZapLogger.Warn("ProductService: failed to update product", zap.Error(err))
		return &dto.UpdateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	return &dto.UpdateProductOutput{
		Message: "Product updated successfully",
		Success: true,
	}
}

// GetProductByID handle logic for Get Product By ID gRPC request in Service
func (s *ProductService) GetProductByID(req *dto.GetProductByIDInput) *dto.GetProductByIDOutput {

	// Get product
	product, err := s.ProductRepo.GetProductByID(req.ID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get product", zap.Error(err))
		return &dto.GetProductByIDOutput{
			Message: err.Error(),
			Success: false,
			Product: nil,
		}
	}

	// Parse ProductModel to ProductDTO
	productDTO := mapper.ProductModelToDTO(product)
	return &dto.GetProductByIDOutput{
		Message: fmt.Sprintf("Get product with id %v successfully", productDTO.ID),
		Success: true,
		Product: productDTO,
	}
}

// GetProductsBySellerID handle logic for Get Products By Seller ID gRPC request in Service
func (s *ProductService) GetProductsBySellerID(req *dto.GetProductsBySellerIDInput) *dto.GetProductsBySellerIDOutput {

	// Get products
	products, err := s.ProductRepo.GetProductsBySellerID(req.SellerID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get products", zap.Error(err))
		return &dto.GetProductsBySellerIDOutput{
			Message:  err.Error(),
			Success:  false,
			Products: nil,
		}
	}

	// Parse ProductsModel to ProductsDTO
	productsDTO := mapper.ProductsModelToDTO(products)
	return &dto.GetProductsBySellerIDOutput{
		Message:  fmt.Sprintf("Get products by sellerID %v", req.SellerID),
		Success:  true,
		Products: productsDTO,
	}
}

// GetInventoryByID handle logic for Get Inventory By ID gRPC request in Service
func (s *ProductService) GetInventoryByID(req *dto.GetInventoryByIDInput) *dto.GetInventoryByIDOutput {

	// Get inventory
	product, err := s.ProductRepo.GetProductByID(req.ID)
	if err != nil {
		s.ZapLogger.Warn("ProductService: failed to get product inventory", zap.Error(err))
		return &dto.GetInventoryByIDOutput{
			Message:   err.Error(),
			Success:   false,
			Inventory: 0,
		}
	}
	return &dto.GetInventoryByIDOutput{
		Message:   fmt.Sprintf("Get product with id %v successfully", product.ID),
		Success:   true,
		Inventory: product.Inventory,
	}
}

// GetAndDecreaseInventoryByID handle logic for Get And Decrease Inventory By ID gRPC request in Service
func (s *ProductService) GetAndDecreaseInventoryByID(req *dto.GetAndDecreaseInventoryByIDInput) *dto.GetAndDecreaseInventoryByIDOutput {

	// Get and decrease inventory
	if err := s.ProductRepo.GetAndDecreaseInventoryByID(req.ID, req.Quantity); err != nil {
		s.ZapLogger.Warn("ProductService: failed to decrease inventory", zap.Error(err))
		return &dto.GetAndDecreaseInventoryByIDOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	return &dto.GetAndDecreaseInventoryByIDOutput{
		Message: "Product decreased successfully",
		Success: true,
	}
}
