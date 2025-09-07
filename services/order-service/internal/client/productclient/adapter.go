package productclient

import productpb "order-service/pkg/client/productclient"

func ProductsProtoToDTO(products []*productpb.Product) []*ProductDTOClient {
	var dtoProducts []*ProductDTOClient
	for _, product := range products {
		productDTO := ProductProtoToDTO(product)
		dtoProducts = append(dtoProducts, productDTO)
	}
	return dtoProducts
}

func ProductProtoToDTO(product *productpb.Product) *ProductDTOClient {
	if product == nil {
		return nil
	}
	return &ProductDTOClient{
		ID:        product.GetId(),
		Name:      product.GetName(),
		Price:     product.GetPrice(),
		SellerID:  product.GetSellerId(),
		Inventory: product.GetInventory(),
	}
}
