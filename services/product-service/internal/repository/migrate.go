package repository

import (
	"fmt"
	"product-service/pkg/events"
	"product-service/pkg/model"
	"product-service/pkg/outbox"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// Event topics published by product-service through the transactional outbox (pkg/events).
const (
	TopicProductChanged   = "product.changed"
	TopicInventoryChanged = "inventory.changed"
)

// Migrate creates/updates every table and the search helpers. Idempotent; used by main and by tests.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.Product{}, &model.Category{}, &model.InventoryLedger{}, &outbox.ValidateOrderEvent{}); err != nil {
		return err
	}
	if err := events.Migrate(db); err != nil {
		return err
	}
	return migrateSearch(db)
}

// vietnameseFold maps accented Vietnamese letters (both cases) to the ASCII letter used for searching.
func vietnameseFold() (from, to string) {
	groups := map[rune]string{
		'a': "àáạảãâầấậẩẫăằắặẳẵ", 'e': "èéẹẻẽêềếệểễ", 'i': "ìíịỉĩ",
		'o': "òóọỏõôồốộổỗơờớợởỡ", 'u': "ùúụủũưừứựửữ", 'y': "ỳýỵỷỹ", 'd': "đ",
	}
	var f, t strings.Builder
	for base, letters := range groups {
		for _, r := range letters {
			f.WriteRune(r)
			t.WriteRune(base)
			if up := unicode.ToUpper(r); up != r {
				f.WriteRune(up)
				t.WriteRune(base)
			}
		}
	}
	return f.String(), t.String()
}

// migrateSearch installs mm_unaccent (pure SQL, IMMUTABLE, no extension needed: lower-cases and removes
// Vietnamese diacritics) and a GIN index on the product search document.
func migrateSearch(db *gorm.DB) error {
	from, to := vietnameseFold()
	fn := fmt.Sprintf(`CREATE OR REPLACE FUNCTION mm_unaccent(t text) RETURNS text
		LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT lower(translate(t, '%s', '%s')) $$`, from, to)
	if err := db.Exec(fn).Error; err != nil {
		return err
	}
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_products_search ON products USING GIN (to_tsvector('simple', ` + searchDocument + `))`).Error
}

// searchDocument is the text a query is matched against; it must be identical in the index and in queries.
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
