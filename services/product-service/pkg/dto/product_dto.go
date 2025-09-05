package dto

import "gorm.io/datatypes"

type Product struct {
	ID         uint64
	Name       string
	Price      float64
	SellerID   uint64
	Inventory  int64
	Attributes datatypes.JSON
}

// CreateProduct

type CreateProductInput struct {
	Name       string
	Price      float64
	SellerID   uint64
	Inventory  int64
	Attributes datatypes.JSON
}
type CreateProductOutput struct {
	Message string
	Success bool
}

// UpdateProduct

type UpdateProductInput struct {
	UserID  uint64
	Product *Product
}
type UpdateProductOutput struct {
	Message string
	Success bool
}

// GetProductByID

type GetProductByIDInput struct {
	ID uint64
}
type GetProductByIDOutput struct {
	Message string
	Success bool
	Product *Product
}

// GetProductsByID

type GetProductsByIDInput struct {
	IDs []uint64
}
type GetProductsByIDOutput struct {
	Message  string
	Success  bool
	Products []*Product
}

// GetProductsBySellerID

type GetProductsBySellerIDInput struct {
	SellerID uint64
}
type GetProductsBySellerIDOutput struct {
	Message  string
	Success  bool
	Products []*Product
}

// GetInventoryByID

type GetInventoryByIDInput struct {
	ID     uint64
	UserID uint64
}
type GetInventoryByIDOutput struct {
	Message   string
	Success   bool
	Inventory int64
}

// GetAndDecreaseInventoryByID

type GetAndDecreaseInventoryByIDInput struct {
	ID       uint64
	Quantity int64
	UserID   uint64
}
type GetAndDecreaseInventoryByIDOutput struct {
	Message string
	Success bool
}
