package model

import "gorm.io/datatypes"

type Product struct {
	ID         uint64         `gorm:"primaryKey"`
	Name       string         `gorm:"not null"`
	Price      float64        `gorm:"not null"`
	SellerID   uint64         `gorm:"not null"`
	Inventory  int64          `gorm:"not null"`
	Attributes datatypes.JSON `gorm:"not null"`
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
	SellerID uint64
	ID       uint64
}
type GetProductByIDOutput struct {
	Message string
	Success bool
	Product *Product
}

// GetProductsBySellerID

type GetProductsBySellerIDInput struct {
	SellerID uint64
}
type GetProductsBySellerIDOutput struct {
	Message  string
	Success  bool
	Products *[]Product
}

// GetInventoryByID

type GetInventoryByIDInput struct {
	ID       uint64
	SellerID uint64
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
	SellerID uint64
}
type GetAndDecreaseInventoryByIDOutput struct {
	Message string
	Success bool
}
