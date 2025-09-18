package dto

type Product struct {
	ID         uint64         `json:"id"`
	Name       string         `json:"name"`
	Price      float64        `json:"price"`
	SellerID   uint64         `json:"seller_id"`
	Inventory  int64          `json:"inventory"`
	Attributes map[string]any `json:"attributes"`
}

type CreateProductInput struct {
	Name       string         `json:"name"`
	Price      float64        `json:"price"`
	SellerID   uint64         `json:"seller_id"`
	Inventory  int64          `json:"inventory"`
	Attributes map[string]any `json:"attributes"`
}
type CreateProductOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
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
	SellerID uint64 `json:"seller_id"`
}
type GetProductsBySellerIDOutput struct {
	Message  string     `json:"message"`
	Success  bool       `json:"success"`
	Products []*Product `json:"products"`
}
