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

func NewProductService(productRepo *repository.ProductRepository, logger *zap.Logger) *ProductService {
	return &ProductService{
		ProductRepo: productRepo,
		ZapLogger:   logger,
	}
}

func (s *ProductService) CreateProduct(req *dto.CreateProductInput) *dto.CreateProductOutput {
	var product = &model.Product{
		Name:       req.Name,
		Price:      req.Price,
		Inventory:  req.Inventory,
		SellerID:   req.SellerID,
		Attributes: req.Attributes,
	}
	if err := s.ProductRepo.CreateProduct(product); err != nil {
		s.ZapLogger.Warn("failed to create product", zap.Error(err))
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

func (s *ProductService) UpdateProduct(req *dto.UpdateProductInput) *dto.UpdateProductOutput {
	oldProduct, err := s.ProductRepo.GetProductByID(req.Product.ID)
	if err != nil {
		s.ZapLogger.Warn("failed to get old product", zap.Error(err))
		return &dto.UpdateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	if oldProduct.SellerID != req.UserID {
		s.ZapLogger.Error("user is not owner of product", zap.Any("old", oldProduct.SellerID))
		return &dto.UpdateProductOutput{
			Message: fmt.Sprintf("Seller is not owner of product"),
			Success: false,
		}
	}

	productModel := mapper.ProductDTOToModel(req.Product)
	if err := s.ProductRepo.UpdateProduct(productModel); err != nil {
		s.ZapLogger.Warn("failed to update product", zap.Error(err))
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

func (s *ProductService) GetProductByID(req *dto.GetProductByIDInput) *dto.GetProductByIDOutput {
	product, err := s.ProductRepo.GetProductByID(req.ID)
	if err != nil {
		s.ZapLogger.Warn("failed to get product", zap.Error(err))
		return &dto.GetProductByIDOutput{
			Message: err.Error(),
			Success: false,
			Product: nil,
		}
	}

	productDTO := mapper.ProductModelToDTO(product)
	return &dto.GetProductByIDOutput{
		Message: fmt.Sprintf("Get product with id %v successfully", productDTO.ID),
		Success: true,
		Product: productDTO,
	}
}

func (s *ProductService) GetProductsBySellerID(req *dto.GetProductsBySellerIDInput) *dto.GetProductsBySellerIDOutput {
	products, err := s.ProductRepo.GetProductsBySellerID(req.SellerID)
	if err != nil {
		s.ZapLogger.Warn("failed to get products", zap.Error(err))
		return &dto.GetProductsBySellerIDOutput{
			Message:  err.Error(),
			Success:  false,
			Products: nil,
		}
	}

	productsDTO := mapper.ProductsModelToDTO(products)
	return &dto.GetProductsBySellerIDOutput{
		Message:  fmt.Sprintf("Get products by sellerID %v", req.SellerID),
		Success:  true,
		Products: productsDTO,
	}
}

func (s *ProductService) GetInventoryByID(req *dto.GetInventoryByIDInput) *dto.GetInventoryByIDOutput {
	product, err := s.ProductRepo.GetProductByID(req.ID)
	if err != nil {
		s.ZapLogger.Warn("failed to get product inventory", zap.Error(err))
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

func (s *ProductService) GetAndDecreaseInventoryByID(req *dto.GetAndDecreaseInventoryByIDInput) *dto.GetAndDecreaseInventoryByIDOutput {
	if err := s.ProductRepo.GetAndDecreaseInventoryByID(req.ID, req.Quantity); err != nil {
		s.ZapLogger.Warn("failed to decrease inventory", zap.Error(err))
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
