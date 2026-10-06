// This file defines adapter functions that convert between
// internal domain dto used by service
// and the dto used by database.

package adapter

import (
	"encoding/json"
	"gorm.io/datatypes"
	"product-service/pkg/dto"
	"product-service/pkg/model"
	"time"
)

func jsonStrings(v []string) datatypes.JSON {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return datatypes.JSON(b)
}

func stringsFromJSON(j datatypes.JSON) []string {
	var out []string
	if len(j) > 0 {
		_ = json.Unmarshal(j, &out)
	}
	return out
}

func ProductDTOToModel(product *dto.Product) *model.Product {
	if product == nil {
		return nil
	}
	attributes := product.Attributes
	if len(attributes) == 0 {
		attributes = datatypes.JSON("{}")
	}
	return &model.Product{
		ID:                product.ID,
		Name:              product.Name,
		Price:             product.Price,
		SellerID:          product.SellerID,
		Inventory:         product.Inventory,
		Attributes:        attributes,
		Description:       product.Description,
		CategoryID:        product.CategoryID,
		Brand:             product.Brand,
		Tags:              jsonStrings(product.Tags),
		ImageURLs:         jsonStrings(product.ImageURLs),
		Status:            product.Status,
		SKU:               product.SKU,
		LowStockThreshold: product.LowStockThreshold,
		WeightG:           product.WeightG,
	}
}
func ProductModelToDTO(product *model.Product) *dto.Product {
	if product == nil {
		return nil
	}
	return &dto.Product{
		ID:                product.ID,
		Name:              product.Name,
		Price:             product.Price,
		SellerID:          product.SellerID,
		Inventory:         product.Inventory,
		Attributes:        product.Attributes,
		Description:       product.Description,
		CategoryID:        product.CategoryID,
		Brand:             product.Brand,
		Tags:              stringsFromJSON(product.Tags),
		ImageURLs:         stringsFromJSON(product.ImageURLs),
		Status:            product.Status,
		SKU:               product.SKU,
		LowStockThreshold: product.LowStockThreshold,
		WeightG:           product.WeightG,
		Reserved:          product.Reserved,
		Version:           product.Version,
		CreatedAt:         product.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         product.UpdatedAt.UTC().Format(time.RFC3339),
		StockLevel:        product.StockLevel(model.DefaultLowStockThreshold),
		Sold:              product.Sold,
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
