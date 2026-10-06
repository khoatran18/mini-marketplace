package dto

import "gorm.io/datatypes"

type Product struct {
	ID         uint64
	Name       string
	Price      float64
	SellerID   uint64
	Inventory  int64
	Attributes datatypes.JSON

	Description       string
	CategoryID        uint64
	Brand             string
	Tags              []string
	ImageURLs         []string
	Status            string
	SKU               string
	LowStockThreshold int64
	WeightG           int64
	Reserved          int64
	Version           int64
	CreatedAt         string
	UpdatedAt         string
	StockLevel        string
	Sold              int64
}

// CreateProduct

type CreateProductInput struct {
	Name       string
	Price      float64
	SellerID   uint64
	Inventory  int64
	Attributes datatypes.JSON

	Description       string
	CategoryID        uint64
	Brand             string
	Tags              []string
	ImageURLs         []string
	Status            string
	SKU               string
	LowStockThreshold int64
	WeightG           int64
}
type CreateProductOutput struct {
	Message string
	Success bool
	ID      uint64
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
	SellerID   uint64
	OnlyActive bool
}
type GetProductsBySellerIDOutput struct {
	Message  string
	Success  bool
	Products []*Product
}

// GetInventoryByID

type GetInventoryByIDInput struct {
	ID uint64
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

// Get List Products

type GetProductsInput struct {
	Page       uint64
	PageSize   uint64
	OnlyActive bool
}
type GetProductsOutput struct {
	Message  string
	Success  bool
	Products []*Product
}
