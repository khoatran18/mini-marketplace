package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/productclient"
	"api-gateway/pkg/clientname"
	productpb "api-gateway/pkg/pb/productservice"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// fakeCatalog records what the gateway sends to product-service.
type fakeCatalog struct {
	productpb.ProductServiceClient
	search   *productpb.SearchProductsRequest
	status   *productpb.SetProductStatusRequest
	adjust   *productpb.AdjustInventoryRequest
	ledger   *productpb.GetInventoryLedgerRequest
	lowStock *productpb.ListLowStockRequest
	upsert   *productpb.UpsertCategoryRequest
	product  *productpb.Product // returned by GetProductByID
	products []*productpb.Product
}

func (f *fakeCatalog) SearchProducts(_ context.Context, in *productpb.SearchProductsRequest, _ ...grpc.CallOption) (*productpb.SearchProductsResponse, error) {
	f.search = in
	return &productpb.SearchProductsResponse{Success: true, Products: f.products, Total: uint64(len(f.products))}, nil
}
func (f *fakeCatalog) SetProductStatus(_ context.Context, in *productpb.SetProductStatusRequest, _ ...grpc.CallOption) (*productpb.SetProductStatusResponse, error) {
	f.status = in
	return &productpb.SetProductStatusResponse{Success: true}, nil
}
func (f *fakeCatalog) AdjustInventory(_ context.Context, in *productpb.AdjustInventoryRequest, _ ...grpc.CallOption) (*productpb.AdjustInventoryResponse, error) {
	f.adjust = in
	return &productpb.AdjustInventoryResponse{Success: true, Inventory: 9}, nil
}
func (f *fakeCatalog) GetInventoryLedger(_ context.Context, in *productpb.GetInventoryLedgerRequest, _ ...grpc.CallOption) (*productpb.GetInventoryLedgerResponse, error) {
	f.ledger = in
	return &productpb.GetInventoryLedgerResponse{Success: true}, nil
}
func (f *fakeCatalog) ListLowStock(_ context.Context, in *productpb.ListLowStockRequest, _ ...grpc.CallOption) (*productpb.ListLowStockResponse, error) {
	f.lowStock = in
	return &productpb.ListLowStockResponse{Success: true}, nil
}
func (f *fakeCatalog) UpsertCategory(_ context.Context, in *productpb.UpsertCategoryRequest, _ ...grpc.CallOption) (*productpb.UpsertCategoryResponse, error) {
	f.upsert = in
	return &productpb.UpsertCategoryResponse{Success: true, Category: in.Category}, nil
}
func (f *fakeCatalog) GetProductByID(_ context.Context, _ *productpb.GetProductByIDRequest, _ ...grpc.CallOption) (*productpb.GetProductByIDResponse, error) {
	return &productpb.GetProductByIDResponse{Success: true, Product: f.product}, nil
}

// catalogRouter wires CatalogHandler + ProductHandler; role "" means an anonymous caller.
func catalogRouter(role string, storeID uint64) (*gin.Engine, *fakeCatalog) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	fake := &fakeCatalog{}
	cm := client.NewClientManager()
	cm.Clients[clientname.ProductClientName] = &client.ServiceClient{Client: fake}
	auth := authclient.NewAuthClient(&fakeAuthClient{storeID: storeID}, nil, logger)
	h := NewCatalogHandler(cm, auth, logger)
	ph := NewProductHandler(productclient.NewProductClient(fake, nil, logger), logger)
	ph.Auth = auth
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if role != "" {
			c.Set("userID", uint64(callerID))
			c.Set("userRole", role)
			c.Set("username", "u")
		}
	})
	r.GET("/search", h.Search)
	r.GET("/seller/products", h.SellerProducts)
	r.PATCH("/products/:id/status", h.SetStatus)
	r.PATCH("/admin/products/:id/status", h.AdminSetStatus)
	r.POST("/seller/products/:id/inventory/adjust", h.AdjustInventory)
	r.GET("/seller/products/:id/inventory/ledger", h.InventoryLedger)
	r.GET("/seller/inventory/low-stock", h.LowStock)
	r.POST("/admin/categories", h.CreateCategory)
	r.PUT("/admin/categories/:id", h.UpdateCategory)
	r.GET("/products/:id/stock", h.Stock)
	r.GET("/products/:id", ph.GetProductByID)
	return r, fake
}

func TestSearchPassesFiltersAndNeverLeaksHiddenProductsToStrangers(t *testing.T) {
	r, fake := catalogRouter("", 0)
	w := do(r, "GET", "/search?q=%C3%A1o+kho%C3%A1c&category_id=3&price_min=10&price_max=99.5&brand=Acme&seller_id=7&in_stock=true&sort=price_asc&page=2&page_size=500", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	s := fake.search
	if s.Query != "áo khoác" || s.CategoryId != 3 || s.PriceMin != 10 || s.PriceMax != 99.5 || s.Brand != "Acme" || s.SellerId != 7 || !s.InStockOnly || s.Sort != "price_asc" || s.Page != 2 {
		t.Errorf("filters not forwarded: %+v", s)
	}
	if s.PageSize != 50 {
		t.Errorf("page size must be capped at 50, got %d", s.PageSize)
	}
	if s.IncludeAllStatuses {
		t.Error("an anonymous visitor must never see drafts/hidden products")
	}
	for _, bad := range []string{"price_min=abc", "price_max=-1", "category_id=x", "page=-2"} {
		if w := do(r, "GET", "/search?"+bad, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", bad, w.Code)
		}
	}
}

func TestSearchOwnerViewIsLimitedToTheOwnStore(t *testing.T) {
	r, fake := catalogRouter("seller_admin", 7)
	do(r, "GET", "/search?seller_id=7", "")
	if !fake.search.IncludeAllStatuses {
		t.Error("the owner may list all statuses of their own store")
	}
	do(r, "GET", "/search?seller_id=8", "")
	if fake.search.IncludeAllStatuses {
		t.Error("another store's drafts must stay hidden")
	}
	do(r, "GET", "/search", "") // no seller filter: public view even for a seller
	if fake.search.IncludeAllStatuses {
		t.Error("no seller filter means the public view")
	}

	r, fake = catalogRouter("admin", 0)
	do(r, "GET", "/search?seller_id=8", "")
	if !fake.search.IncludeAllStatuses {
		t.Error("admins may see every store")
	}
}

func TestSearchMasksStockForStrangersOnly(t *testing.T) {
	products := []*productpb.Product{{Id: 1, SellerId: 7, Inventory: 500, Reserved: 40, LowStockThreshold: 9, StockLevel: "ok"}}
	stranger, f1 := catalogRouter("buyer", 0)
	f1.products = products
	var out struct {
		Products []map[string]any `json:"products"`
	}
	w := do(stranger, "GET", "/search", "")
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Products[0]["inventory"] != float64(20) || out.Products[0]["reserved"] != float64(0) || out.Products[0]["low_stock_threshold"] != float64(0) || out.Products[0]["stock_level"] != "ok" {
		t.Errorf("stock must be capped for strangers: %v", out.Products[0])
	}
	owner, f2 := catalogRouter("seller_admin", 7)
	f2.products = []*productpb.Product{{Id: 1, SellerId: 7, Inventory: 500, Reserved: 40}}
	w = do(owner, "GET", "/search?seller_id=7", "")
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Products[0]["inventory"] != float64(500) || out.Products[0]["reserved"] != float64(40) {
		t.Errorf("owner sees the real numbers: %v", out.Products[0])
	}
}

func TestSellerEndpointsUseTheStoreFromTheTokenOnly(t *testing.T) {
	r, fake := catalogRouter("seller_employee", 42)
	do(r, "GET", "/seller/products?seller_id=999", "")
	if fake.search.SellerId != 42 || !fake.search.IncludeAllStatuses {
		t.Errorf("seller listing must use the caller's store: %+v", fake.search)
	}
	do(r, "PATCH", "/products/5/status", `{"status":"hidden","store_id":999,"is_admin":true}`)
	if fake.status.StoreId != 42 || fake.status.IsAdmin || fake.status.ProductId != 5 || fake.status.Status != "hidden" {
		t.Errorf("status request: %+v", fake.status)
	}
	do(r, "POST", "/seller/products/5/inventory/adjust", `{"delta":-3,"reason":"damaged","store_id":999}`)
	if fake.adjust.StoreId != 42 || fake.adjust.Delta != -3 || fake.adjust.Reason != "damaged" {
		t.Errorf("adjust request: %+v", fake.adjust)
	}
	do(r, "GET", "/seller/products/5/inventory/ledger?page=2", "")
	if fake.ledger.StoreId != 42 || fake.ledger.Page != 2 {
		t.Errorf("ledger request: %+v", fake.ledger)
	}
	do(r, "GET", "/seller/inventory/low-stock?threshold=3", "")
	if fake.lowStock.StoreId != 42 || fake.lowStock.DefaultThreshold != 3 {
		t.Errorf("low stock request: %+v", fake.lowStock)
	}
	if w := do(r, "PATCH", "/products/abc/status", `{"status":"hidden"}`); w.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", w.Code)
	}
	if w := do(r, "PATCH", "/products/5/status", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing status: %d", w.Code)
	}
}

func TestSellerWithoutStoreAndNonSellersAreRejected(t *testing.T) {
	r, fake := catalogRouter("seller_admin", 0)
	if w := do(r, "GET", "/seller/products", ""); w.Code != http.StatusForbidden {
		t.Errorf("seller without store: %d", w.Code)
	}
	if fake.search != nil {
		t.Error("must not reach product-service")
	}
	r, _ = catalogRouter("buyer", 0)
	if w := do(r, "POST", "/seller/products/1/inventory/adjust", `{"delta":1,"reason":"x"}`); w.Code != http.StatusForbidden {
		t.Errorf("buyer adjusting stock: %d", w.Code)
	}
}

func TestAdminStatusAndCategories(t *testing.T) {
	r, fake := catalogRouter("admin", 0)
	do(r, "PATCH", "/admin/products/9/status", `{"status":"banned"}`)
	if !fake.status.IsAdmin || fake.status.Status != "banned" || fake.status.ProductId != 9 {
		t.Errorf("admin ban: %+v", fake.status)
	}
	w := do(r, "POST", "/admin/categories", `{"name":"Đồ chơi","parent_id":2}`)
	if w.Code != http.StatusCreated || fake.upsert.Category.Id != 0 || fake.upsert.Category.ParentId != 2 || !fake.upsert.Category.Active {
		t.Errorf("create category: %d %+v", w.Code, fake.upsert)
	}
	w = do(r, "PUT", "/admin/categories/5", `{"name":"x","active":false}`)
	if w.Code != http.StatusOK || fake.upsert.Category.Id != 5 || fake.upsert.Category.Active {
		t.Errorf("update category: %d %+v", w.Code, fake.upsert)
	}
}

func TestProductDetailHidesNonActiveProductsFromStrangers(t *testing.T) {
	draft := &productpb.Product{Id: 3, Name: "x", Price: 1, SellerId: 7, Status: "draft", Inventory: 100}
	for role, tc := range map[string]struct {
		store uint64
		want  int
	}{"": {0, 404}, "buyer": {0, 404}, "seller_admin": {8, 404}} {
		r, fake := catalogRouter(role, tc.store)
		fake.product = draft
		if w := do(r, "GET", "/products/3", ""); w.Code != tc.want {
			t.Errorf("role %q store %d: status %d, want %d", role, tc.store, w.Code, tc.want)
		}
	}
	r, fake := catalogRouter("seller_admin", 7)
	fake.product = draft
	if w := do(r, "GET", "/products/3", ""); w.Code != 200 {
		t.Errorf("the owner can see their draft: %d", w.Code)
	}
	r, fake = catalogRouter("admin", 0)
	fake.product = draft
	if w := do(r, "GET", "/products/3", ""); w.Code != 200 {
		t.Errorf("admin can see any product: %d", w.Code)
	}
	r, fake = catalogRouter("", 0)
	fake.product = &productpb.Product{Id: 3, Name: "x", Price: 1, SellerId: 7, Status: "active", Inventory: 100, Reserved: 5}
	w := do(r, "GET", "/products/3", "")
	var out struct {
		Product map[string]any `json:"product"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Product["inventory"] != float64(20) || out.Product["reserved"] != float64(0) {
		t.Errorf("public detail must mask stock: %d %v", w.Code, out.Product)
	}
}

func TestStockEndpointShowsOnlyTheLevel(t *testing.T) {
	r, fake := catalogRouter("", 0)
	fake.product = &productpb.Product{Id: 3, Status: "active", Inventory: 3, StockLevel: "low"}
	w := do(r, "GET", "/products/3/stock", "")
	if w.Code != 200 || w.Body.String() != `{"level":"low","product_id":3}` {
		t.Errorf("stock: %d %s", w.Code, w.Body)
	}
	fake.product = &productpb.Product{Id: 3, Status: "hidden", StockLevel: "ok"}
	if w := do(r, "GET", "/products/3/stock", ""); w.Code != 404 {
		t.Errorf("hidden product stock: %d", w.Code)
	}
}
