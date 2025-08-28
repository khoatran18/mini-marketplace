package service

import (
	"account-service/internal/repository"
	"account-service/pkg/model"
	"fmt"

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

func (s *ProductService) CreateProduct(req *model.CreateProductInput) *model.CreateProductOutput {
	var product = &model.Product{
		Name:       req.Name,
		Price:      req.Price,
		Inventory:  req.Inventory,
		SellerID:   req.SellerID,
		Attributes: req.Attributes,
	}
	if err := s.ProductRepo.CreateProduct(product); err != nil {
		return &model.CreateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	return &model.CreateProductOutput{
		Message: "Product created successfully",
		Success: true,
	}
}

func (s *ProductService) UpdateProduct(req *model.UpdateProductInput) *model.UpdateProductOutput {
	oldProduct, err := s.ProductRepo.GetProductByID(req.Product.ID)
	if err != nil {
		return &model.UpdateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	if oldProduct.SellerID != req.UserID {
		return &model.UpdateProductOutput{
			Message: fmt.Sprintf("Seller is not owner of product"),
			Success: false,
		}
	}

	if err := s.ProductRepo.UpdateProduct(req.Product); err != nil {
		return &model.UpdateProductOutput{
			Message: err.Error(),
			Success: false,
		}
	}
	return &model.UpdateProductOutput{
		Message: "Product updated successfully",
		Success: true,
	}
}

func (s *ProductService) GetProductByID(req *model.GetProductByIDInput) *model.GetProductByIDOutput {
	product, err := s.ProductRepo.GetProductByID(req.ID)
	if err != nil {
		return &model.GetProductByIDOutput{
			Message: err.Error(),
			Success: false,
			Product: nil,
		}
	}
	return &model.GetProductByIDOutput{
		Message: fmt.Sprintf("Get product with id %v successfully", product.ID),
		Success: true,
		Product: product,
	}
}

func (s *ProductService) GetProductsBySellerID(req *model.GetProductsBySellerIDInput) *model.GetProductsBySellerIDOutput {
	products, err := s.ProductRepo.GetProductsBySellerID(req.SellerID)
	if err != nil {
		return &model.GetProductsBySellerIDOutput{
			Message:  err.Error(),
			Success:  false,
			Products: nil,
		}
	}
	return &model.GetProductsBySellerIDOutput{
		Message:  fmt.Sprintf("Get products by sellerID %v", req.SellerID),
		Success:  true,
		Products: &products,
	}
}

func (s *ProductService) GetInventoryByID(req *model.GetInventoryByIDInput) *model.GetInventoryByIDOutput {
	product, err := s.ProductRepo.GetProductByID(req.ID)
	if err != nil {
		return &model.GetInventoryByIDOutput{
			Message:   err.Error(),
			Success:   false,
			Inventory: -1,
		}
	}
	return &model.GetInventoryByIDOutput{
		Message:   fmt.Sprintf("Get product with id %v successfully", product.ID),
		Success:   true,
		Inventory: product.Inventory,
	}
}

func (s *ProductService) GetAndDecreaseInventoryByID(req *model.GetAndDecreaseInventoryByIDInput) *model.GetAndDecreaseInventoryByIDOutput {
	if err := s.ProductRepo.GetAndDecreaseInventoryByID(req.ID, req.Quantity), err {
		return &mode
	}
}
