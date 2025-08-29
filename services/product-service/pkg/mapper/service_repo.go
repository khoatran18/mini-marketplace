package mapper

import (
	"product-service/pkg/dto"
	"product-service/pkg/model"
)

func ProductDTOToModel(product *dto.Product) *model.Product {
	if product == nil {
		return nil
	}
	return &model.Product{
		ID:         product.ID,
		Name:       product.Name,
		Price:      product.Price,
		SellerID:   product.SellerID,
		Inventory:  product.Inventory,
		Attributes: product.Attributes,
	}
}
func ProductModelToDTO(product *model.Product) *dto.Product {
	if product == nil {
		return nil
	}
	return &dto.Product{
		ID:         product.ID,
		Name:       product.Name,
		Price:      product.Price,
		SellerID:   product.SellerID,
		Inventory:  product.Inventory,
		Attributes: product.Attributes,
	}
}

func ProductsDTOToModel(products []*dto.Product) []*model.Product {
	var Products = make([]*model.Product, len(products))
	for _, product := range products {
		Products = append(Products, ProductDTOToModel(product))
	}
	return Products
}

func ProductsModelToDTO(products []*model.Product) []*dto.Product {
	var productsDTO []*dto.Product
	for _, product := range products {
		productsDTO = append(productsDTO, ProductModelToDTO(product))
	}
	return productsDTO
}
