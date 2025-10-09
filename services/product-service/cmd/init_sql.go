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

	products := []struct {
		Name       string
		Price      float64
		SellerID   uint64
		Inventory  int
		Attributes datatypes.JSON
	}{
		{Name: "iPhone 15 Pro", Price: 1200.00, SellerID: 2, Inventory: 50, Attributes: datatypes.JSON([]byte(`{"brand":"Apple","color":"black","storage":"256GB","weight":187}`))},
		{Name: "MacBook Air M3", Price: 1500.00, SellerID: 2, Inventory: 30, Attributes: datatypes.JSON([]byte(`{"brand":"Apple","color":"silver","ram":"16GB","ssd":"512GB","screen":13.6}`))},
		{Name: "iPad Pro M2", Price: 999.00, SellerID: 2, Inventory: 40, Attributes: datatypes.JSON([]byte(`{"brand":"Apple","color":"space gray","storage":"512GB","refresh_rate":120}`))},
		{Name: "Apple Watch Ultra 2", Price: 899.00, SellerID: 2, Inventory: 60, Attributes: datatypes.JSON([]byte(`{"size":"49mm","band":"orange alpine","gps":true,"water_resistance":"100m"}`))},
		{Name: "AirPods Pro 2", Price: 249.00, SellerID: 2, Inventory: 100, Attributes: datatypes.JSON([]byte(`{"noise_canceling":true,"wireless_charging":true,"chip":"H2"}`))},
		{Name: "Samsung Galaxy S24 Ultra", Price: 1300.00, SellerID: 2, Inventory: 45, Attributes: datatypes.JSON([]byte(`{"brand":"Samsung","color":"titanium gray","storage":"512GB","zoom":"100x"}`))},
		{Name: "Galaxy Tab S9", Price: 999.00, SellerID: 2, Inventory: 35, Attributes: datatypes.JSON([]byte(`{"brand":"Samsung","display":"12.4 inch","ram":"8GB","pen_included":true}`))},
		{Name: "Dell XPS 13 Plus", Price: 1600.00, SellerID: 2, Inventory: 25, Attributes: datatypes.JSON([]byte(`{"brand":"Dell","cpu":"Intel i7","ram":"16GB","ssd":"1TB","os":"Windows 11"}`))},
		{Name: "Asus ROG Zephyrus G14", Price: 1800.00, SellerID: 2, Inventory: 20, Attributes: datatypes.JSON([]byte(`{"brand":"Asus","gpu":"RTX 4070","refresh_rate":165,"weight":1.6}`))},
		{Name: "Sony WH-1000XM5", Price: 399.00, SellerID: 2, Inventory: 80, Attributes: datatypes.JSON([]byte(`{"brand":"Sony","color":"black","battery_life":"30h","bluetooth_version":"5.3"}`))},
		{Name: "Logitech MX Master 3S", Price: 129.00, SellerID: 2, Inventory: 120, Attributes: datatypes.JSON([]byte(`{"dpi":8000,"connectivity":"Bluetooth + 2.4GHz","color":"graphite"}`))},
		{Name: "Razer BlackWidow V4", Price: 179.00, SellerID: 2, Inventory: 70, Attributes: datatypes.JSON([]byte(`{"switch_type":"Green","rgb":true,"macro_keys":5}`))},
		{Name: "LG UltraFine 5K Monitor", Price: 1400.00, SellerID: 2, Inventory: 15, Attributes: datatypes.JSON([]byte(`{"resolution":"5120x2880","size":27,"ports":["Thunderbolt 3","USB-C"],"refresh_rate":60}`))},
		{Name: "GoPro Hero 12", Price: 499.00, SellerID: 2, Inventory: 55, Attributes: datatypes.JSON([]byte(`{"resolution":"5.3K","stabilization":true,"waterproof_depth":"10m"}`))},
		{Name: "Nintendo Switch OLED", Price: 349.00, SellerID: 2, Inventory: 90, Attributes: datatypes.JSON([]byte(`{"screen":"7 inch OLED","storage":"64GB","battery":"9h","dock_included":true}`))},
		{Name: "Kindle Paperwhite", Price: 159.00, SellerID: 2, Inventory: 110, Attributes: datatypes.JSON([]byte(`{"screen":"6.8 inch","storage":"16GB","waterproof":true,"light_adjustable":true}`))},
		{Name: "Dyson V15 Detect", Price: 749.00, SellerID: 2, Inventory: 40, Attributes: datatypes.JSON([]byte(`{"power":"240AW","dust_capacity":"0.76L","battery_life":"60min","laser_detection":true}`))},
		{Name: "Philips Hue Starter Kit", Price: 199.00, SellerID: 2, Inventory: 80, Attributes: datatypes.JSON([]byte(`{"bulbs":3,"supports_voice_control":true,"connectivity":"Zigbee"}`))},
		{Name: "Anker PowerCore 20000", Price: 59.00, SellerID: 2, Inventory: 200, Attributes: datatypes.JSON([]byte(`{"capacity":"20000mAh","ports":["USB-A","USB-C"],"fast_charging":true}`))},
		{Name: "Xiaomi Robot Vacuum X10", Price: 599.00, SellerID: 2, Inventory: 45, Attributes: datatypes.JSON([]byte(`{"suction_power":"4000Pa","battery":"5200mAh","smart_mapping":true,"auto_empty":true}`))},
	}

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
			fmt.Printf("Seed %s thất bại: %v\n", p.Name, err)
			continue
		}
		fmt.Printf("Seed %s: %s\n", p.Name, resp.Message)
	}

	fmt.Println("Hoàn tất seed 20 sản phẩm!")
}
