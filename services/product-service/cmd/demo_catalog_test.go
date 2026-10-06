package main

import "testing"

func TestDemoCatalogIsLargeValidAndUniqueEnough(t *testing.T) {
	items := demoCatalog()
	if len(items) < 100 {
		t.Fatalf("the demo catalog should have at least 100 products, got %d", len(items))
	}
	seen := map[string]bool{}
	cats := map[string]int{}
	for _, p := range items {
		if p.Name == "" || p.Price <= 0 || p.Stock <= 0 || p.Description == "" || p.Brand == "" || p.Category == "" {
			t.Errorf("incomplete product: %+v", p)
		}
		if seen[p.Name] {
			t.Errorf("duplicate product name %q", p.Name)
		}
		seen[p.Name] = true
		cats[p.Category]++
	}
	for _, slug := range []string{"dien-thoai", "laptop", "ao", "quan", "giay-dep", "gia-dung", "sach", "lam-dep", "the-thao", "thuc-pham"} {
		if cats[slug] == 0 {
			t.Errorf("category %q has no demo products", slug)
		}
	}
}
