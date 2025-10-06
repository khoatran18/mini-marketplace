package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/datatypes"

	"product-service/internal/server"
	productpb "product-service/pkg/pb"
)

func convertJSONToStruct(data datatypes.JSON) *structpb.Struct {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		log.Printf("⚠️ Lỗi parse JSON: %v", err)
		return nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		log.Printf("⚠️ Lỗi tạo structpb.Struct: %v", err)
		return nil
	}
	return s
}

func SeedProducts(productServer *server.ProductServer) {
	ctx := context.Background()

	products := []struct {
		Name       string
		Price      float64
		SellerID   uint64
		Inventory  int
		Attributes datatypes.JSON
	}{
		{
			Name:       "iPhone 15 Pro",
			Price:      1200.00,
			SellerID:   2,
			Inventory:  50,
			Attributes: datatypes.JSON([]byte(`{"color": "black", "storage": "256GB"}`)),
		},
		{
			Name:       "MacBook Air M3",
			Price:      1500.00,
			SellerID:   2,
			Inventory:  30,
			Attributes: datatypes.JSON([]byte(`{"color": "silver", "ram": "16GB"}`)),
		},
	}

	fmt.Println("🌱 Bắt đầu seed sản phẩm mặc định...")

	for _, p := range products {
		req := &productpb.CreateProductRequest{
			Name:       p.Name,
			Price:      p.Price,
			SellerId:   p.SellerID,
			Inventory:  int64(p.Inventory),
			Attributes: convertJSONToStruct(p.Attributes),
		}

		resp, err := productServer.CreateProduct(ctx, req)
		if err != nil {
			fmt.Printf("❌ Seed %s thất bại: %v\n", p.Name, err)
			continue
		}
		fmt.Printf("✅ Seed %s: %s\n", p.Name, resp.Message)
	}

	fmt.Println("🌱 Hoàn tất seed sản phẩm!")
}
