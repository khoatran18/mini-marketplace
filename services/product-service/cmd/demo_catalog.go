package main

// demoProduct is one row of the demo catalog (prices in VND).
type demoProduct struct {
	Name        string
	Price       float64
	Stock       int64
	Category    string // category slug
	Brand       string
	Description string
	Tags        []string
	Attributes  string // JSON
}

// demoCatalog is generated data: 20 electronics plus fashion, home, books, beauty, sports and food (about 100 items).
func demoCatalog() []demoProduct {
	var out []demoProduct
	out = append(out, demoProduct{"iPhone 15 Pro", 29990000, 50, "dien-thoai", "Apple", "Điện thoại cao cấp chip A17 Pro, khung titan, camera 48MP.", []string{"điện thoại", "apple", "5g"}, `{"color": "black", "storage": "256GB", "weight": 187}`})
	out = append(out, demoProduct{"MacBook Air M3", 32990000, 30, "laptop", "Apple", "Laptop mỏng nhẹ chip M3, pin cả ngày, màn hình Liquid Retina 13.6 inch.", []string{"laptop", "apple", "văn phòng"}, `{"color": "silver", "ram": "16GB", "ssd": "512GB", "screen": 13.6}`})
	out = append(out, demoProduct{"iPad Pro M2", 24990000, 40, "dien-tu", "Apple", "Máy tính bảng màn hình 120Hz, hỗ trợ Apple Pencil.", []string{"máy tính bảng", "apple"}, `{"storage": "512GB", "refresh_rate": 120}`})
	out = append(out, demoProduct{"Apple Watch Ultra 2", 21990000, 60, "phu-kien", "Apple", "Đồng hồ thông minh vỏ titan, GPS hai tần số, chống nước 100m.", []string{"đồng hồ", "thể thao"}, `{"size": "49mm", "gps": true}`})
	out = append(out, demoProduct{"AirPods Pro 2", 5990000, 100, "phu-kien", "Apple", "Tai nghe không dây chống ồn chủ động, sạc không dây.", []string{"tai nghe", "chống ồn"}, `{"noise_canceling": true, "chip": "H2"}`})
	out = append(out, demoProduct{"Samsung Galaxy S24 Ultra", 28990000, 45, "dien-thoai", "Samsung", "Điện thoại bút S Pen, zoom 100x, màn hình Dynamic AMOLED.", []string{"điện thoại", "samsung", "5g"}, `{"storage": "512GB", "zoom": "100x"}`})
	out = append(out, demoProduct{"Galaxy Tab S9", 19990000, 35, "dien-tu", "Samsung", "Máy tính bảng 12.4 inch kèm bút S Pen, RAM 8GB.", []string{"máy tính bảng", "samsung"}, `{"display": "12.4 inch", "ram": "8GB"}`})
	out = append(out, demoProduct{"Dell XPS 13 Plus", 36990000, 25, "laptop", "Dell", "Laptop Intel Core i7, SSD 1TB, thiết kế viền mỏng.", []string{"laptop", "dell", "văn phòng"}, `{"cpu": "Intel i7", "ram": "16GB", "ssd": "1TB"}`})
	out = append(out, demoProduct{"Asus ROG Zephyrus G14", 42990000, 20, "laptop", "Asus", "Laptop gaming RTX 4070, màn hình 165Hz.", []string{"laptop", "gaming", "asus"}, `{"gpu": "RTX 4070", "refresh_rate": 165}`})
	out = append(out, demoProduct{"Sony WH-1000XM5", 8490000, 80, "phu-kien", "Sony", "Tai nghe chụp tai chống ồn hàng đầu, pin 30 giờ.", []string{"tai nghe", "chống ồn", "sony"}, `{"battery_life": "30h"}`})
	out = append(out, demoProduct{"Logitech MX Master 3S", 2490000, 120, "phu-kien", "Logitech", "Chuột không dây yên tĩnh, cảm biến 8000 DPI.", []string{"chuột", "văn phòng"}, `{"dpi": 8000}`})
	out = append(out, demoProduct{"Razer BlackWidow V4", 3990000, 70, "phu-kien", "Razer", "Bàn phím cơ gaming switch xanh, đèn RGB.", []string{"bàn phím", "gaming"}, `{"switch_type": "Green", "rgb": true}`})
	out = append(out, demoProduct{"LG UltraFine 5K Monitor", 27990000, 15, "dien-tu", "LG", "Màn hình 27 inch 5K, cổng Thunderbolt 3.", []string{"màn hình"}, `{"resolution": "5120x2880", "size": 27}`})
	out = append(out, demoProduct{"GoPro Hero 12", 9990000, 55, "dien-tu", "GoPro", "Camera hành động 5.3K, chống rung, chống nước 10m.", []string{"camera", "du lịch"}, `{"resolution": "5.3K"}`})
	out = append(out, demoProduct{"Nintendo Switch OLED", 8990000, 90, "dien-tu", "Nintendo", "Máy chơi game cầm tay màn hình OLED 7 inch.", []string{"game", "giải trí"}, `{"screen": "7 inch OLED"}`})
	out = append(out, demoProduct{"Kindle Paperwhite", 3990000, 110, "dien-tu", "Amazon", "Máy đọc sách màn hình 6.8 inch, chống nước, đèn nền điều chỉnh.", []string{"đọc sách"}, `{"storage": "16GB"}`})
	out = append(out, demoProduct{"Dyson V15 Detect", 18990000, 40, "gia-dung", "Dyson", "Máy hút bụi không dây phát hiện bụi bằng laser.", []string{"hút bụi", "nhà cửa"}, `{"power": "240AW"}`})
	out = append(out, demoProduct{"Philips Hue Starter Kit", 3990000, 80, "gia-dung", "Philips", "Bộ 3 bóng đèn thông minh điều khiển bằng giọng nói.", []string{"đèn", "nhà thông minh"}, `{"bulbs": 3}`})
	out = append(out, demoProduct{"Anker PowerCore 20000", 1290000, 200, "phu-kien", "Anker", "Pin sạc dự phòng 20000mAh, sạc nhanh USB-C.", []string{"pin dự phòng", "sạc nhanh"}, `{"capacity": "20000mAh"}`})
	out = append(out, demoProduct{"Xiaomi Robot Vacuum X10", 12990000, 45, "gia-dung", "Xiaomi", "Robot hút bụi lực hút 4000Pa, tự đổ rác.", []string{"robot", "hút bụi"}, `{"suction_power": "4000Pa"}`})
	fashion := []struct {
		Name, Cat, Brand string
		Price            float64
	}{
		{"Áo thun cotton", "ao", "Uniqlo", 199000},
		{"Áo sơ mi trắng", "ao", "Routine", 349000},
		{"Áo khoác chống nước", "ao", "Columbia", 899000},
		{"Áo hoodie nỉ", "ao", "Coolmate", 459000},
		{"Quần jean slim", "quan", "Levi's", 799000},
		{"Quần kaki", "quan", "Routine", 449000},
		{"Quần short thể thao", "quan", "Nike", 329000},
		{"Giày sneaker trắng", "giay-dep", "Adidas", 1890000},
		{"Giày chạy bộ", "giay-dep", "Nike", 2490000},
		{"Dép quai ngang", "giay-dep", "Biti's", 259000},
	}
	colors := []string{"đen", "trắng", "xanh navy", "xám", "be", "đỏ đô"}
	for i, f := range fashion {
		for j, color := range colors {
			out = append(out, demoProduct{
				Name: f.Name + " " + color, Price: f.Price, Stock: int64(20 + (i*7+j*13)%80), Category: f.Cat, Brand: f.Brand,
				Description: f.Name + " màu " + color + ", chất liệu thoải mái, phù hợp mặc hằng ngày.",
				Tags:        []string{"thời trang", color}, Attributes: `{"color":"` + color + `"}`,
			})
		}
	}
	other := []struct {
		Name, Cat, Brand string
		Price            float64
		Desc             string
	}{
		{"Nồi chiên không dầu 5L", "gia-dung", "Philips", 2490000, "Nồi chiên không dầu dung tích 5L, 8 chế độ nấu."},
		{"Bình giữ nhiệt 750ml", "gia-dung", "Lock&Lock", 349000, "Bình giữ nhiệt inox 304, giữ nóng 12 giờ."},
		{"Bộ dao nhà bếp 5 món", "gia-dung", "Tefal", 1290000, "Bộ dao thép không gỉ kèm đế gỗ."},
		{"Máy xay sinh tố", "gia-dung", "Panasonic", 1190000, "Máy xay công suất 600W, cối thủy tinh."},
		{"Nhà giả kim", "sach", "NXB Văn Học", 89000, "Tiểu thuyết kinh điển của Paulo Coelho."},
		{"Đắc nhân tâm", "sach", "First News", 95000, "Cuốn sách về nghệ thuật giao tiếp."},
		{"Sapiens lược sử loài người", "sach", "Omega Plus", 189000, "Lịch sử loài người từ thời nguyên thủy."},
		{"Clean Code", "sach", "Prentice Hall", 420000, "Nghệ thuật viết mã sạch cho lập trình viên."},
		{"Kem chống nắng SPF50", "lam-dep", "Anessa", 459000, "Kem chống nắng dịu nhẹ, không gây bết dính."},
		{"Sữa rửa mặt dịu nhẹ", "lam-dep", "Cerave", 329000, "Làm sạch sâu, không làm khô da."},
		{"Serum vitamin C", "lam-dep", "La Roche-Posay", 899000, "Serum làm sáng da, mờ thâm nám."},
		{"Thảm tập yoga", "the-thao", "Adidas", 399000, "Thảm yoga chống trượt dày 6mm."},
		{"Tạ tay cao su 5kg", "the-thao", "Kingsport", 299000, "Tạ tay bọc cao su, bán theo cặp."},
		{"Vợt cầu lông", "the-thao", "Yonex", 1590000, "Vợt carbon nhẹ, cân bằng đầu."},
		{"Cà phê rang xay 500g", "thuc-pham", "Trung Nguyên", 189000, "Cà phê rang xay nguyên chất."},
		{"Mật ong rừng 500ml", "thuc-pham", "Honeyland", 249000, "Mật ong nguyên chất thu hoạch từ rừng."},
		{"Trà ô long 200g", "thuc-pham", "Phúc Long", 159000, "Trà ô long thượng hạng."},
		{"Hạt điều rang muối 500g", "thuc-pham", "Vinanut", 229000, "Hạt điều Bình Phước rang muối."},
		{"Ổ cứng SSD 1TB", "dien-tu", "Samsung", 2790000, "Ổ cứng SSD NVMe tốc độ đọc 7000MB/s."},
		{"Loa bluetooth chống nước", "dien-tu", "JBL", 2290000, "Loa di động chống nước IP67, pin 12 giờ."},
		{"Balo laptop 15.6 inch", "phu-kien", "Xiaomi", 599000, "Balo chống nước, ngăn đệm laptop."},
		{"Cáp sạc USB-C 2m", "phu-kien", "Anker", 249000, "Cáp bện dù, sạc nhanh 100W."},
	}
	for i, o := range other {
		out = append(out, demoProduct{Name: o.Name, Price: o.Price, Stock: int64(15 + (i*11)%120), Category: o.Cat, Brand: o.Brand, Description: o.Desc, Tags: []string{o.Cat}, Attributes: `{}`})
	}
	return out
}
