package dto

type Product struct {
	ID         uint64         `json:"id"`
	Name       string         `json:"name"`
	Price      float64        `json:"price"`
	SellerID   uint64         `json:"seller_id"`
	Inventory  int64          `json:"inventory"` // available stock (capped for people who do not own the product)
	Attributes map[string]any `json:"attributes"`

	Description       string   `json:"description"`
	CategoryID        uint64   `json:"category_id"`
	Brand             string   `json:"brand"`
	Tags              []string `json:"tags"`
	ImageURLs         []string `json:"image_urls"`
	Status            string   `json:"status"` // draft | active | hidden | banned
	SKU               string   `json:"sku"`
	LowStockThreshold int64    `json:"low_stock_threshold"`
	WeightG           int64    `json:"weight_g"`
	Reserved          int64    `json:"reserved"` // owner only
	Sold              int64    `json:"sold"`
	Version           int64    `json:"version"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	StockLevel        string   `json:"stock_level"` // none | low | ok
}

type CreateProductInput struct {
	Name       string         `json:"name"`
	Price      float64        `json:"price"`
	SellerID   uint64         `json:"seller_id"`
	Inventory  int64          `json:"inventory"`
	Attributes map[string]any `json:"attributes"`

	Description       string   `json:"description"`
	CategoryID        uint64   `json:"category_id"`
	Brand             string   `json:"brand"`
	Tags              []string `json:"tags"`
	ImageURLs         []string `json:"image_urls"`
	Status            string   `json:"status"`
	SKU               string   `json:"sku"`
	LowStockThreshold int64    `json:"low_stock_threshold"`
	WeightG           int64    `json:"weight_g"`
}
type CreateProductOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
	ID      uint64 `json:"id"`
}

type UpdateProductInput struct {
	Product *Product `json:"product"`
	UserId  uint64   `json:"user_id"`
}
type UpdateProductOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GetProductByIDInput struct {
	ProductID uint64 `json:"product_id"`
}
type GetProductByIDOutput struct {
	Message string   `json:"message"`
	Success bool     `json:"success"`
	Product *Product `json:"product"`
}

type GetProductsBySellerIDInput struct {
	SellerID   uint64 `json:"seller_id"`
	OnlyActive bool   `json:"-"`
}
type GetProductsBySellerIDOutput struct {
	Message  string     `json:"message"`
	Success  bool       `json:"success"`
	Products []*Product `json:"products"`
}

type GetProductsInput struct {
	Page       uint64 `json:"page"`
	PageSize   uint64 `json:"page_size"`
	OnlyActive bool   `json:"-"`
}
type GetProductsOutput struct {
	Message  string     `json:"message"`
	Success  bool       `json:"success"`
	Products []*Product `json:"products"`
}
