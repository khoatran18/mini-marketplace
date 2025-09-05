package productclient

type ProductDTOClient struct {
	ID        uint64
	Name      string
	Price     float64
	SellerID  uint64
	Inventory int64
}
