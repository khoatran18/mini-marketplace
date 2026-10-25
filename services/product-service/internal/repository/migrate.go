package repository

import (
	"context"
	"product-service/internal/migrate"
	"product-service/pkg/model"

	"gorm.io/gorm"
)

// Event topics published by product-service through the transactional outbox (pkg/events).
const (
	TopicProductChanged   = "product.changed"
	TopicInventoryChanged = "inventory.changed"
)

// Migrate áp dụng các migration goose (migrations/*.sql: bảng, hàm mm_unaccent, index GIN tìm kiếm).
// Idempotent; dùng bởi main và test.
func Migrate(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return migrate.Run(context.Background(), sqlDB)
}

// searchDocument is the text a query is matched against; it must be identical in the index (migrations/00001_baseline.sql) and in queries.
const searchDocument = `mm_unaccent(coalesce(name,'') || ' ' || coalesce(brand,'') || ' ' || coalesce(sku,'') || ' ' || coalesce(description,''))`

// DefaultCategories is the initial category tree (inserted only into an empty table). Admins edit it afterwards.
var DefaultCategories = []struct {
	Name     string
	Slug     string
	Children []struct{ Name, Slug string }
}{
	{"Thời trang", "thoi-trang", []struct{ Name, Slug string }{{"Áo", "ao"}, {"Quần", "quan"}, {"Giày dép", "giay-dep"}}},
	{"Điện tử", "dien-tu", []struct{ Name, Slug string }{{"Điện thoại", "dien-thoai"}, {"Laptop", "laptop"}, {"Phụ kiện", "phu-kien"}}},
	{"Gia dụng", "gia-dung", nil},
	{"Sách", "sach", nil},
	{"Làm đẹp", "lam-dep", nil},
	{"Thể thao", "the-thao", nil},
	{"Thực phẩm", "thuc-pham", nil},
}

// SeedCategories inserts DefaultCategories when no category exists.
func SeedCategories(db *gorm.DB) error {
	var n int64
	if err := db.Model(&model.Category{}).Count(&n).Error; err != nil || n > 0 {
		return err
	}
	for i, c := range DefaultCategories {
		parent := &model.Category{Name: c.Name, Slug: c.Slug, Sort: i, Active: true}
		if err := db.Create(parent).Error; err != nil {
			return err
		}
		for j, ch := range c.Children {
			if err := db.Create(&model.Category{ParentID: parent.ID, Name: ch.Name, Slug: ch.Slug, Sort: j, Active: true}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
