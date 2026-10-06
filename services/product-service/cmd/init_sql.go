package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/datatypes"

	"product-service/internal/server"
	"product-service/pkg/dto"
	"product-service/pkg/model"
	productpb "product-service/pkg/pb"
)

func convertJSONToStruct(data datatypes.JSON) *structpb.Struct {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		log.Printf("Lỗi parse JSON: %v", err)
		return nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		log.Printf("Lỗi tạo structpb.Struct: %v", err)
		return nil
	}
	return s
}

func SeedProducts(productServer *server.ProductServer) {
	ctx := context.Background()

	if existing, err := productServer.ProductService.GetProducts(ctx, &dto.GetProductsInput{Page: 1, PageSize: 1}); err == nil && len(existing.Products) > 0 {
		fmt.Println("Catalog is not empty, skip seeding demo products")
		return
	}

	// resolve category slugs to ids (SeedCategories runs before the demo seed)
	categoryIDs := map[string]uint64{}
	var cats []model.Category
	productServer.ProductService.ProductRepo.DB.Find(&cats)
	for _, c := range cats {
		categoryIDs[c.Slug] = c.ID
	}

	count := 0
	for _, p := range demoCatalog() {
		req := &productpb.CreateProductRequest{
			Name:        p.Name,
			Price:       p.Price,
			SellerId:    2,
			Inventory:   p.Stock,
			Attributes:  convertJSONToStruct(datatypes.JSON([]byte(p.Attributes))),
			Description: p.Description,
			CategoryId:  categoryIDs[p.Category],
			Brand:       p.Brand,
			Tags:        p.Tags,
		}
		if _, err := productServer.CreateProduct(ctx, req); err != nil {
			fmt.Printf("Seed %s thất bại: %v\n", p.Name, err)
			continue
		}
		count++
	}

	fmt.Printf("Hoàn tất seed %d sản phẩm demo!\n", count)
}
