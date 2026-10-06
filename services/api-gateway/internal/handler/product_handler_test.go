package handler

import (
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/productclient"
	authpb "api-gateway/pkg/pb/authservice"
	productpb "api-gateway/pkg/pb/productservice"
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type fakeAuthClient struct {
	authpb.AuthServiceClient
	storeID uint64
}

func (f *fakeAuthClient) GetStoreIDRoleById(_ context.Context, _ *authpb.GetStoreIDRoleByIDRequest, _ ...grpc.CallOption) (*authpb.GetStoreIDRoleByIDResponse, error) {
	return &authpb.GetStoreIDRoleByIDResponse{Message: "alice", Success: true, Role: "seller_admin", StoreId: f.storeID}, nil
}

type fakeProductClient struct {
	productpb.ProductServiceClient
	created *productpb.CreateProductRequest
	updated *productpb.UpdateProductRequest
}

func (f *fakeProductClient) CreateProduct(_ context.Context, in *productpb.CreateProductRequest, _ ...grpc.CallOption) (*productpb.CreateProductResponse, error) {
	f.created = in
	return &productpb.CreateProductResponse{Message: "ok", Success: true}, nil
}

func (f *fakeProductClient) UpdateProduct(_ context.Context, in *productpb.UpdateProductRequest, _ ...grpc.CallOption) (*productpb.UpdateProductResponse, error) {
	f.updated = in
	return &productpb.UpdateProductResponse{Message: "ok", Success: true}, nil
}

func productRouter(storeID uint64) (*gin.Engine, *fakeProductClient) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	fake := &fakeProductClient{}
	h := NewProductHandler(productclient.NewProductClient(fake, nil, logger), logger)
	h.Auth = authclient.NewAuthClient(&fakeAuthClient{storeID: storeID}, nil, logger)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", uint64(callerID)); c.Set("userRole", "seller_admin") })
	r.POST("/products", h.CreateProduct)
	r.PUT("/products/:id", h.UpdateProduct)
	r.GET("/products", h.GetProducts)
	return r, fake
}

func TestCreateProductBelongsToCallersStore(t *testing.T) {
	r, fake := productRouter(77)
	w := do(r, "POST", "/products", `{"name":"x","price":10,"inventory":3,"seller_id":999}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if fake.created.GetSellerId() != 77 {
		t.Errorf("seller_id = %d; the product must belong to the caller's store, not the client-supplied seller", fake.created.GetSellerId())
	}
}

func TestSellerWithoutStoreCannotManageProducts(t *testing.T) {
	r, fake := productRouter(0)
	if w := do(r, "POST", "/products", `{"name":"x","price":10,"inventory":3}`); w.Code != http.StatusForbidden {
		t.Errorf("create: status %d, want 403", w.Code)
	}
	if w := do(r, "PUT", "/products/1", `{"product":{"name":"x","price":1}}`); w.Code != http.StatusForbidden {
		t.Errorf("update: status %d, want 403", w.Code)
	}
	if fake.created != nil || fake.updated != nil {
		t.Error("request must not reach product-service")
	}
}

func TestUpdateProductIgnoresClientIdentity(t *testing.T) {
	r, fake := productRouter(77)
	w := do(r, "PUT", "/products/5", `{"user_id":12345,"product":{"id":9,"name":"x","price":2,"seller_id":12345}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if fake.updated.GetUserId() != 77 || fake.updated.GetProduct().GetSellerId() != 77 || fake.updated.GetProduct().GetId() != 5 {
		t.Errorf("client identity leaked: %+v", fake.updated)
	}
}

func TestProductInputValidation(t *testing.T) {
	r, fake := productRouter(77)
	for name, c := range map[string][3]string{
		"create no name":       {"POST", "/products", `{"price":10}`},
		"create zero price":    {"POST", "/products", `{"name":"x","price":0}`},
		"create neg inventory": {"POST", "/products", `{"name":"x","price":1,"inventory":-1}`},
		"update no product":    {"PUT", "/products/1", `{}`},
		"update neg price":     {"PUT", "/products/1", `{"product":{"name":"x","price":-1}}`},
		"update bad id":        {"PUT", "/products/abc", `{"product":{"name":"x","price":1}}`},
	} {
		if w := do(r, c[0], c[1], c[2]); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
	if fake.created != nil || fake.updated != nil {
		t.Error("invalid request reached product-service")
	}
}
