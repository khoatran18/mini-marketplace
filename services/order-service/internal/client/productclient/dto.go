package productclient

type ProductDTOClient struct {
	ID        uint64
	Name      string
	Price     float64
	SellerID  uint64
	Inventory int64
}

type GetProductsByIDInput struct {
	IDs []uint64
}

type GetProductsByIDOutput struct {
	Products []*ProductDTOClient
	Message  string
	Success  bool
}
