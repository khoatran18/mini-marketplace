package productclient

// ProductDTOClient is the part of a catalog entry order-service needs.
type ProductDTOClient struct {
	ID         uint64
	Name       string
	Price      float64
	SellerID   uint64
	Inventory  int64
	Status     string // draft | active | hidden | banned
	SKU        string
	CategoryID uint64
	ImageURL   string // first image
	StockLevel string // none | low | ok
}

type GetProductsByIDInput struct {
	IDs []uint64
}

type GetProductsByIDOutput struct {
	Products []*ProductDTOClient
	Message  string
	Success  bool
}
