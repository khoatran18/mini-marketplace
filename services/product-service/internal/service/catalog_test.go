package service

import (
	"context"
	"encoding/json"
	"errors"
	"product-service/internal/repository"
	"product-service/pkg/dto"
	"product-service/pkg/events"
	"product-service/pkg/model"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func create(t *testing.T, s *ProductService, in dto.CreateProductInput) uint64 {
	t.Helper()
	if in.Attributes == nil {
		in.Attributes = []byte(`{}`)
	}
	out, err := s.CreateProduct(context.Background(), &in)
	if err != nil {
		t.Fatalf("create %q: %v", in.Name, err)
	}
	if out.ID == 0 {
		t.Fatal("CreateProduct must return the new id")
	}
	return out.ID
}

func categoryID(t *testing.T, s *ProductService, slug string) uint64 {
	t.Helper()
	if err := repository.SeedCategories(s.ProductRepo.DB); err != nil {
		t.Fatal(err)
	}
	var c model.Category
	if err := s.ProductRepo.DB.Where("slug = ?", slug).First(&c).Error; err != nil {
		t.Fatalf("category %s: %v", slug, err)
	}
	return c.ID
}

func names(products []*dto.Product) string {
	var n []string
	for _, p := range products {
		n = append(n, p.Name)
	}
	return strings.Join(n, "|")
}

func TestCreateProductCatalogFieldsAndDefaults(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	cat := categoryID(t, s, "ao")
	id := create(t, s, dto.CreateProductInput{
		Name: "Áo khoác chống nước", Price: 450000, SellerID: 1, Inventory: 10, Description: "Áo khoác dù, nhẹ", CategoryID: cat,
		Brand: "Acme", Tags: []string{"mưa", "nam"}, ImageURLs: []string{"https://img.example/a.jpg"}, SKU: "AK-01", WeightG: 400,
	})
	got, err := s.GetProductByID(ctx, &dto.GetProductByIDInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	p := got.Product
	if p.Status != "active" || p.Brand != "Acme" || p.CategoryID != cat || p.SKU != "AK-01" || len(p.Tags) != 2 || len(p.ImageURLs) != 1 || p.WeightG != 400 {
		t.Errorf("unexpected product %+v", p)
	}
	if p.StockLevel != "ok" || p.Version != 1 || p.CreatedAt == "" {
		t.Errorf("computed fields wrong: level=%s version=%d created=%q", p.StockLevel, p.Version, p.CreatedAt)
	}

	// the same SKU in another store is fine, in the same store it is rejected
	create(t, s, dto.CreateProductInput{Name: "x", Price: 1, SellerID: 2, SKU: "AK-01"})
	_, err = s.CreateProduct(ctx, &dto.CreateProductInput{Name: "y", Price: 1, SellerID: 1, SKU: "AK-01", Attributes: []byte(`{}`)})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("duplicate sku in one store: want AlreadyExists, got %v", err)
	}
	// products without a SKU never collide
	create(t, s, dto.CreateProductInput{Name: "a", Price: 1, SellerID: 1})
	create(t, s, dto.CreateProductInput{Name: "b", Price: 1, SellerID: 1})
}

func TestCreateProductCatalogValidation(t *testing.T) {
	s := testService(t)
	cases := map[string]dto.CreateProductInput{
		"unknown category":   {Name: "x", Price: 1, SellerID: 1, CategoryID: 999},
		"banned on create":   {Name: "x", Price: 1, SellerID: 1, Status: "banned"},
		"unknown status":     {Name: "x", Price: 1, SellerID: 1, Status: "deleted"},
		"too many tags":      {Name: "x", Price: 1, SellerID: 1, Tags: make([]string, 21)},
		"blank tag":          {Name: "x", Price: 1, SellerID: 1, Tags: []string{" "}},
		"javascript image":   {Name: "x", Price: 1, SellerID: 1, ImageURLs: []string{"javascript:alert(1)"}},
		"too many images":    {Name: "x", Price: 1, SellerID: 1, ImageURLs: []string{"/a", "/b", "/c", "/d", "/e", "/f", "/g", "/h", "/i"}},
		"negative weight":    {Name: "x", Price: 1, SellerID: 1, WeightG: -1},
		"huge description":   {Name: "x", Price: 1, SellerID: 1, Description: strings.Repeat("a", 5001)},
		"negative low-stock": {Name: "x", Price: 1, SellerID: 1, LowStockThreshold: -1},
	}
	for name, in := range cases {
		in := in
		in.Attributes = []byte(`{}`)
		if _, err := s.CreateProduct(context.Background(), &in); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, err)
		}
	}
}

func TestSearchIsAccentInsensitiveAndFiltered(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	ao, quan := categoryID(t, s, "ao"), categoryID(t, s, "quan")
	fashion := categoryID(t, s, "thoi-trang")
	a := create(t, s, dto.CreateProductInput{Name: "Áo khoác chống nước", Price: 450000, SellerID: 1, Inventory: 5, CategoryID: ao, Brand: "Acme", Description: "dành cho mùa mưa"})
	create(t, s, dto.CreateProductInput{Name: "Áo thun cotton", Price: 150000, SellerID: 1, Inventory: 0, CategoryID: ao, Brand: "Béo"})
	create(t, s, dto.CreateProductInput{Name: "Quần jean Đẹp", Price: 600000, SellerID: 2, Inventory: 3, CategoryID: quan})
	hidden := create(t, s, dto.CreateProductInput{Name: "Áo khoác ẩn", Price: 100, SellerID: 1, Inventory: 1, CategoryID: ao, Status: "hidden"})

	search := func(p repository.SearchParams) ([]*dto.Product, int64) {
		t.Helper()
		got, total, err := s.SearchProducts(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return got, total
	}
	if got, _ := search(repository.SearchParams{Query: "ao khoac"}); names(got) != "Áo khoác chống nước" {
		t.Errorf("unaccented query: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{Query: "ÁO KHOÁC chống"}); names(got) != "Áo khoác chống nước" {
		t.Errorf("accented upper-case query: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{Query: "khoa"}); names(got) != "Áo khoác chống nước" {
		t.Errorf("prefix query: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{Query: "mua mua"}); len(got) != 1 || got[0].ID != a {
		t.Errorf("description is searchable: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{Query: "dep"}); names(got) != "Quần jean Đẹp" {
		t.Errorf("đ folds to d: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{Query: "beo"}); names(got) != "Áo thun cotton" {
		t.Errorf("brand is searchable: %q", names(got))
	}
	// hidden products never show in a public search, the owner view includes them
	if got, total := search(repository.SearchParams{Query: "khoac"}); total != 1 || names(got) != "Áo khoác chống nước" {
		t.Errorf("hidden leaked: %d %q", total, names(got))
	}
	if got, _ := search(repository.SearchParams{Query: "khoac", SellerID: 1, IncludeAllStatuses: true}); len(got) != 2 {
		t.Errorf("owner view must include hidden: %q", names(got))
	}
	// category filter includes descendants
	if _, total := search(repository.SearchParams{CategoryID: fashion}); total != 3 {
		t.Errorf("parent category should include children, got %d", total)
	}
	if _, total := search(repository.SearchParams{CategoryID: quan}); total != 1 {
		t.Errorf("leaf category: %d", total)
	}
	// price / stock / brand filters
	if got, _ := search(repository.SearchParams{PriceMin: 200000, PriceMax: 500000}); names(got) != "Áo khoác chống nước" {
		t.Errorf("price range: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{InStockOnly: true, SellerID: 1}); names(got) != "Áo khoác chống nước" {
		t.Errorf("in stock only: %q", names(got))
	}
	if got, _ := search(repository.SearchParams{Brand: "ACME"}); names(got) != "Áo khoác chống nước" {
		t.Errorf("brand filter: %q", names(got))
	}
	// sorting + pagination
	if got, total := search(repository.SearchParams{Sort: repository.SortPriceDesc, PageSize: 2}); total != 3 || names(got) != "Quần jean Đẹp|Áo khoác chống nước" {
		t.Errorf("price_desc page 1: total=%d %q", total, names(got))
	}
	if got, _ := search(repository.SearchParams{Sort: repository.SortPriceDesc, PageSize: 2, Page: 2}); names(got) != "Áo thun cotton" {
		t.Errorf("price_desc page 2: %q", names(got))
	}
	_ = hidden
	// operators in user input are neutralised, nothing but letters/digits reaches tsquery
	for _, q := range []string{"'; DROP TABLE products; --", "a & | ! ( ) :* <->", "\\", "%_"} {
		if _, _, err := s.SearchProducts(ctx, repository.SearchParams{Query: q}); err != nil {
			t.Errorf("query %q must not error: %v", q, err)
		}
	}
	if _, _, err := s.SearchProducts(ctx, repository.SearchParams{Sort: "random"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("unknown sort: %v", err)
	}
	if _, _, err := s.SearchProducts(ctx, repository.SearchParams{PriceMin: 10, PriceMax: 5}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("inverted price range: %v", err)
	}
}

func TestCategoriesTreeCountsAndRules(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	ao := categoryID(t, s, "ao")
	create(t, s, dto.CreateProductInput{Name: "p1", Price: 1, SellerID: 1, Inventory: 1, CategoryID: ao})
	create(t, s, dto.CreateProductInput{Name: "p2", Price: 1, SellerID: 1, Inventory: 1, CategoryID: ao, Status: "draft"})

	cats, err := s.ListCategories(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, c := range cats {
		counts[c.Slug] = c.ProductCount
	}
	if counts["ao"] != 1 || counts["thoi-trang"] != 1 || counts["dien-tu"] != 0 {
		t.Errorf("counts must include descendants and ignore drafts: %v", counts)
	}

	// slug is generated from the name; duplicates and cycles are rejected; depth is capped at 3
	root, err := s.UpsertCategory(ctx, &model.Category{Name: "Đồ chơi trẻ em", Active: true})
	if err != nil || root.Slug != "do-choi-tre-em" {
		t.Fatalf("slug: %v %+v", err, root)
	}
	if _, err := s.UpsertCategory(ctx, &model.Category{Name: "x", Slug: "do-choi-tre-em"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("duplicate slug: %v", err)
	}
	child, _ := s.UpsertCategory(ctx, &model.Category{Name: "Lego", ParentID: root.ID, Active: true})
	grand, err := s.UpsertCategory(ctx, &model.Category{Name: "Lego City", ParentID: child.ID, Active: true})
	if err != nil {
		t.Fatalf("3 levels are allowed: %v", err)
	}
	if _, err := s.UpsertCategory(ctx, &model.Category{Name: "Too deep", ParentID: grand.ID, Active: true}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("4th level must be rejected: %v", err)
	}
	root.ParentID = grand.ID
	if _, err := s.UpsertCategory(ctx, root); status.Code(err) != codes.InvalidArgument {
		t.Errorf("cycle must be rejected: %v", err)
	}
	if _, err := s.UpsertCategory(ctx, &model.Category{Name: "Orphan", ParentID: 9999}); status.Code(err) != codes.NotFound {
		t.Errorf("unknown parent: %v", err)
	}
	// inactive categories disappear from the public list and can not be assigned
	child.Active = false
	if _, err := s.UpsertCategory(ctx, child); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProduct(ctx, &dto.CreateProductInput{Name: "x", Price: 1, SellerID: 1, CategoryID: child.ID, Attributes: []byte(`{}`)}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("inactive category must not be assignable: %v", err)
	}
}

func TestProductStatusRules(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	id := create(t, s, dto.CreateProductInput{Name: "p", Price: 1, SellerID: 1, Inventory: 1})

	if err := s.SetProductStatus(ctx, id, 2, false, "hidden"); status.Code(err) != codes.PermissionDenied {
		t.Errorf("foreign store: %v", err)
	}
	if err := s.SetProductStatus(ctx, id, 1, false, "banned"); status.Code(err) != codes.PermissionDenied {
		t.Errorf("seller can not ban: %v", err)
	}
	if err := s.SetProductStatus(ctx, id, 1, false, "nonsense"); status.Code(err) != codes.InvalidArgument {
		t.Errorf("unknown status: %v", err)
	}
	if err := s.SetProductStatus(ctx, 9999, 1, false, "hidden"); status.Code(err) != codes.NotFound {
		t.Errorf("unknown product: %v", err)
	}
	if err := s.SetProductStatus(ctx, id, 1, false, "hidden"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProductStatus(ctx, id, 0, true, "banned"); err != nil {
		t.Fatalf("admin ban: %v", err)
	}
	// a banned product can not be re-activated by its owner, neither through status nor through an update
	if err := s.SetProductStatus(ctx, id, 1, false, "active"); status.Code(err) != codes.PermissionDenied {
		t.Errorf("seller must not unban: %v", err)
	}
	if _, err := s.UpdateProduct(ctx, &dto.UpdateProductInput{UserID: 1, Product: &dto.Product{ID: id, Name: "p", Price: 1, Status: "active"}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ProductRepo.GetProductByID(ctx, id); got.Status != "banned" {
		t.Errorf("status after seller update = %s, want banned", got.Status)
	}
	if err := s.SetProductStatus(ctx, id, 0, true, "active"); err != nil {
		t.Fatalf("admin unban: %v", err)
	}
}

func TestReservationMovesStockThroughReservedToSold(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	id := seed(t, s, 1, 10, 10)
	item := dto.ItemEvent{ProductID: id, Quantity: 4}

	if err := s.ValidateProductInventory(ctx, createOrderMsg(1, item)); err != nil {
		t.Fatal(err)
	}
	p, _ := s.ProductRepo.GetProductByID(ctx, id)
	if p.Inventory != 6 || p.Reserved != 4 || p.Sold != 0 {
		t.Fatalf("after reserve: inventory=%d reserved=%d sold=%d", p.Inventory, p.Reserved, p.Sold)
	}
	// shipping moves reserved -> sold exactly once; available stock does not change
	for i := 0; i < 3; i++ {
		if err := s.ProductRepo.MarkShipped(ctx, 1, []repository.ItemQuantity{{ProductID: id, Quantity: 4}}); err != nil {
			t.Fatal(err)
		}
	}
	p, _ = s.ProductRepo.GetProductByID(ctx, id)
	if p.Inventory != 6 || p.Reserved != 0 || p.Sold != 4 {
		t.Fatalf("after ship: inventory=%d reserved=%d sold=%d", p.Inventory, p.Reserved, p.Sold)
	}
	// a shipped order can no longer be released
	if err := s.ReleaseProductInventory(ctx, cancelOrderMsg(1, item)); err != nil {
		t.Fatal(err)
	}
	if got := inventoryOf(t, s, id); got != 6 {
		t.Errorf("shipped goods must not be restocked: %d", got)
	}

	// reserve then release restores both counters
	if err := s.ValidateProductInventory(ctx, createOrderMsg(2, dto.ItemEvent{ProductID: id, Quantity: 2})); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseProductInventory(ctx, cancelOrderMsg(2, dto.ItemEvent{ProductID: id, Quantity: 2})); err != nil {
		t.Fatal(err)
	}
	p, _ = s.ProductRepo.GetProductByID(ctx, id)
	if p.Inventory != 6 || p.Reserved != 0 {
		t.Errorf("after release: inventory=%d reserved=%d", p.Inventory, p.Reserved)
	}

	// the ledger tells the story: initial? (seed bypasses the service) reserve, sale, reserve, release
	var reasons []string
	rows, _ := s.ProductRepo.GetLedger(ctx, id, 1, 1, 50)
	for i := len(rows) - 1; i >= 0; i-- {
		reasons = append(reasons, rows[i].Reason)
	}
	if got := strings.Join(reasons, ","); !strings.HasSuffix(got, "reserve,sale,reserve,release") {
		t.Errorf("ledger = %s", got)
	}
	if rows[0].BalanceAfter != 6 {
		t.Errorf("balance_after of the latest entry = %d", rows[0].BalanceAfter)
	}
}

func TestAdjustInventoryAndLedgerAccess(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	id := create(t, s, dto.CreateProductInput{Name: "p", Price: 1, SellerID: 1, Inventory: 5})

	if _, err := s.AdjustInventory(ctx, id, 2, 3, "x"); status.Code(err) != codes.PermissionDenied {
		t.Errorf("foreign store: %v", err)
	}
	if _, err := s.AdjustInventory(ctx, id, 1, 0, "x"); status.Code(err) != codes.InvalidArgument {
		t.Errorf("zero delta: %v", err)
	}
	if _, err := s.AdjustInventory(ctx, id, 1, 3, " "); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing reason: %v", err)
	}
	if _, err := s.AdjustInventory(ctx, id, 1, -6, "lost"); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("negative stock must be refused: %v", err)
	}
	if inv, err := s.AdjustInventory(ctx, id, 1, 7, "new batch"); err != nil || inv != 12 {
		t.Errorf("restock: %d %v", inv, err)
	}
	if inv, err := s.AdjustInventory(ctx, id, 1, -2, "damaged"); err != nil || inv != 10 {
		t.Errorf("adjust: %d %v", inv, err)
	}
	entries, err := s.GetInventoryLedger(ctx, id, 1, 1, 10)
	if err != nil || len(entries) != 3 {
		t.Fatalf("ledger: %d %v", len(entries), err)
	}
	if entries[0].Reason != "adjust" || entries[0].Delta != -2 || entries[0].BalanceAfter != 10 || entries[1].Reason != "restock" || entries[2].Reason != "initial" {
		t.Errorf("unexpected ledger %+v %+v %+v", entries[0], entries[1], entries[2])
	}
	if _, err := s.GetInventoryLedger(ctx, id, 2, 1, 10); status.Code(err) != codes.PermissionDenied {
		t.Errorf("ledger of another store: %v", err)
	}

	// editing the stock through UpdateProduct is recorded too
	if _, err := s.UpdateProduct(ctx, &dto.UpdateProductInput{UserID: 1, Product: &dto.Product{ID: id, Name: "p", Price: 1, Inventory: 15}}); err != nil {
		t.Fatal(err)
	}
	entries, _ = s.GetInventoryLedger(ctx, id, 1, 1, 10)
	if entries[0].Reason != "update" || entries[0].Delta != 5 {
		t.Errorf("update must be ledgered: %+v", entries[0])
	}
}

func TestLowStockList(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	create(t, s, dto.CreateProductInput{Name: "plenty", Price: 1, SellerID: 1, Inventory: 100})
	create(t, s, dto.CreateProductInput{Name: "default-low", Price: 1, SellerID: 1, Inventory: 5})
	create(t, s, dto.CreateProductInput{Name: "custom-threshold", Price: 1, SellerID: 1, Inventory: 20, LowStockThreshold: 20})
	create(t, s, dto.CreateProductInput{Name: "none", Price: 1, SellerID: 1, Inventory: 0})
	create(t, s, dto.CreateProductInput{Name: "draft-low", Price: 1, SellerID: 1, Inventory: 1, Status: "draft"})
	create(t, s, dto.CreateProductInput{Name: "other-store", Price: 1, SellerID: 2, Inventory: 0})

	got, err := s.ListLowStock(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if names(got) != "none|default-low|custom-threshold" {
		t.Errorf("low stock = %q", names(got))
	}
	for _, p := range got {
		if p.Name == "none" && p.StockLevel != "none" {
			t.Errorf("level of an empty product = %s", p.StockLevel)
		}
	}
}

func TestDomainEventsAreEmittedAndPublished(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	id := create(t, s, dto.CreateProductInput{Name: "p", Price: 1, SellerID: 1, Inventory: 3})
	if err := s.ValidateProductInventory(ctx, createOrderMsg(7, dto.ItemEvent{ProductID: id, Quantity: 3})); err != nil {
		t.Fatal(err)
	}

	var topics []string
	pub := func(_ context.Context, topic, key string, payload []byte) error {
		topics = append(topics, topic+":"+key)
		return nil
	}
	n, err := events.PublishBatch(ctx, s.ProductRepo.DB, pub, 100, time.Second)
	if err != nil || n != 3 {
		t.Fatalf("published %d (%v), topics %v", n, err, topics)
	}
	// create -> product.changed + inventory.changed(initial); reserve -> inventory.changed(level none)
	if strings.Join(topics, ",") != "product.changed:1,inventory.changed:1,inventory.changed:1" {
		t.Errorf("topics = %v", topics)
	}
	var last events.Event
	s.ProductRepo.DB.Order("id DESC").First(&last)
	var payload map[string]any
	_ = json.Unmarshal(last.Payload, &payload)
	if payload["level"] != "none" || payload["reason"] != "reserve" || payload["delta"] != float64(-3) || payload["reserved"] != float64(3) {
		t.Errorf("last payload = %s", last.Payload)
	}
	// nothing is published twice
	if n, _ := events.PublishBatch(ctx, s.ProductRepo.DB, pub, 100, time.Second); n != 0 {
		t.Errorf("second batch published %d", n)
	}

	// a failing broker keeps the row for retry (FAILED) and reports the error
	create(t, s, dto.CreateProductInput{Name: "q", Price: 1, SellerID: 1})
	boom := errors.New("broker down")
	if _, err := events.PublishBatch(ctx, s.ProductRepo.DB, func(context.Context, string, string, []byte) error { return boom }, 100, time.Second); !errors.Is(err, boom) {
		t.Errorf("want broker error, got %v", err)
	}
	pending, _, _ := events.Backlog(s.ProductRepo.DB)
	if pending != 1 {
		t.Errorf("backlog = %d, want 1", pending)
	}
	if n, err := events.PublishBatch(ctx, s.ProductRepo.DB, pub, 100, time.Second); err != nil || n != 1 {
		t.Errorf("retry: %d %v", n, err)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"Thời trang": "thoi-trang", "  Đồ chơi  trẻ em!! ": "do-choi-tre-em", "A/B Test": "a-b-test", "!!!": ""} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
