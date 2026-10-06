package handler

import (
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/userclient"
	userpb "api-gateway/pkg/pb/userservice"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type fakeUserClient struct {
	userpb.UserServiceClient
	createdBuyer  *userpb.CreateBuyerRequest
	updatedBuyer  *userpb.UpdateBuyerByUserIDRequest
	gotBuyerFor   []uint64
	deletedBuyer  []uint64
	createdSeller *userpb.CreateSellerRequest
	updatedSeller *userpb.UpdateSellerByIDRequest
	deletedSeller []uint64
}

func (f *fakeUserClient) CreateBuyer(_ context.Context, in *userpb.CreateBuyerRequest, _ ...grpc.CallOption) (*userpb.CreateBuyerResponse, error) {
	f.createdBuyer = in
	return &userpb.CreateBuyerResponse{Message: "ok", Success: true}, nil
}

func (f *fakeUserClient) UpdateBuyerByUserID(_ context.Context, in *userpb.UpdateBuyerByUserIDRequest, _ ...grpc.CallOption) (*userpb.UpdateBuyerByUserIDResponse, error) {
	f.updatedBuyer = in
	return &userpb.UpdateBuyerByUserIDResponse{Message: "ok", Success: true}, nil
}

func (f *fakeUserClient) GetBuyerByUserID(_ context.Context, in *userpb.GetBuyerByUserIDRequest, _ ...grpc.CallOption) (*userpb.GetBuyerByUserIDResponse, error) {
	f.gotBuyerFor = append(f.gotBuyerFor, in.GetUserId())
	return &userpb.GetBuyerByUserIDResponse{Message: "ok", Success: true, Buyer: &userpb.Buyer{UserId: in.GetUserId(), Name: "Alice", Phone: "123"}}, nil
}

func (f *fakeUserClient) DelBuyerByUserID(_ context.Context, in *userpb.DelBuyerByUserIDRequest, _ ...grpc.CallOption) (*userpb.DelBuyerByUserIDResponse, error) {
	f.deletedBuyer = append(f.deletedBuyer, in.GetUserId())
	return &userpb.DelBuyerByUserIDResponse{Message: "ok", Success: true}, nil
}

func (f *fakeUserClient) CreateSeller(_ context.Context, in *userpb.CreateSellerRequest, _ ...grpc.CallOption) (*userpb.CreateSellerResponse, error) {
	f.createdSeller = in
	return &userpb.CreateSellerResponse{Message: "ok", Success: true}, nil
}

func (f *fakeUserClient) UpdateSellerByID(_ context.Context, in *userpb.UpdateSellerByIDRequest, _ ...grpc.CallOption) (*userpb.UpdateSellerByIDResponse, error) {
	f.updatedSeller = in
	return &userpb.UpdateSellerByIDResponse{Message: "ok", Success: true}, nil
}

func (f *fakeUserClient) GetSellerByID(_ context.Context, in *userpb.GetSellerByIDRequest, _ ...grpc.CallOption) (*userpb.GetSellerByIDResponse, error) {
	return &userpb.GetSellerByIDResponse{Message: "ok", Success: true, Seller: &userpb.Seller{
		Id: in.GetUserId(), Name: "Shop", Description: "d", BankAccount: "VN-123", TaxCode: "TAX", Phone: "999", Address: "street",
	}}, nil
}

func (f *fakeUserClient) DelSellerByID(_ context.Context, in *userpb.DelSellerByIDRequest, _ ...grpc.CallOption) (*userpb.DelSellerByIDResponse, error) {
	f.deletedSeller = append(f.deletedSeller, in.GetUserId())
	return &userpb.DelSellerByIDResponse{Message: "ok", Success: true}, nil
}

func userRouter(storeID uint64) (*gin.Engine, *fakeUserClient) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	fake := &fakeUserClient{}
	h := NewUserHandler(userclient.NewUserClient(fake, nil, logger), logger)
	h.Auth = authclient.NewAuthClient(&fakeAuthClient{storeID: storeID}, nil, logger)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", uint64(callerID)); c.Set("userRole", "buyer") })
	r.POST("/users/buyers", h.CreateBuyer)
	r.GET("/users/buyers/:id", h.GetBuyerByUserID)
	r.PUT("/users/buyers/:id", h.UpdateBuyerByUserID)
	r.DELETE("/users/buyers/:id", h.DelBuyerByUserID)
	r.POST("/users/sellers", h.CreateSeller)
	r.GET("/users/sellers/:id", h.GetSellerByID)
	r.PUT("/users/sellers/:id", h.UpdateSellerByID)
	r.DELETE("/users/sellers/:id", h.DelSellerByID)
	return r, fake
}

func TestBuyerProfilesAreOwnerOnly(t *testing.T) {
	r, fake := userRouter(0)
	other := "/users/buyers/43" // caller is 42

	if w := do(r, "GET", other, ""); w.Code != http.StatusForbidden {
		t.Errorf("GET other profile: status %d, want 403", w.Code)
	}
	if w := do(r, "PUT", other, `{"buyer":{"name":"hacked"}}`); w.Code != http.StatusForbidden {
		t.Errorf("PUT other profile: status %d, want 403", w.Code)
	}
	if w := do(r, "DELETE", other, ""); w.Code != http.StatusForbidden {
		t.Errorf("DELETE other profile: status %d, want 403", w.Code)
	}
	if len(fake.gotBuyerFor) != 0 || fake.updatedBuyer != nil || len(fake.deletedBuyer) != 0 {
		t.Error("foreign profile requests reached user-service")
	}

	if w := do(r, "GET", "/users/buyers/42", ""); w.Code != http.StatusOK {
		t.Errorf("GET own profile: status %d", w.Code)
	}
	if w := do(r, "PUT", "/users/buyers/42", `{"buyer":{"user_id":1,"name":"me"}}`); w.Code != http.StatusOK {
		t.Errorf("PUT own profile: status %d: %s", w.Code, w.Body)
	}
	if fake.updatedBuyer.GetBuyer().GetUserId() != callerID {
		t.Errorf("update targeted user %d", fake.updatedBuyer.GetBuyer().GetUserId())
	}
	if w := do(r, "DELETE", "/users/buyers/42", ""); w.Code != http.StatusOK {
		t.Errorf("DELETE own profile: status %d", w.Code)
	}
}

func TestCreateBuyerAlwaysForTheCaller(t *testing.T) {
	r, fake := userRouter(0)
	if w := do(r, "POST", "/users/buyers", `{"buyer":{"user_id":999,"name":"x"}}`); w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if fake.createdBuyer.GetBuyer().GetUserId() != callerID {
		t.Errorf("profile created for user %d", fake.createdBuyer.GetBuyer().GetUserId())
	}
	if w := do(r, "POST", "/users/buyers", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing buyer: status %d, want 400", w.Code)
	}
}

func TestCreateSellerAlwaysForTheCaller(t *testing.T) {
	r, fake := userRouter(0)
	if w := do(r, "POST", "/users/sellers", `{"user_id":999,"seller":{"id":5,"name":"Shop"}}`); w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if fake.createdSeller.GetUserId() != callerID || fake.createdSeller.GetSeller().GetId() != 0 {
		t.Errorf("store bound to user %d / id %d", fake.createdSeller.GetUserId(), fake.createdSeller.GetSeller().GetId())
	}
	if w := do(r, "POST", "/users/sellers", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing seller: status %d, want 400", w.Code)
	}
}

func TestSellerManagementIsLimitedToOwnStore(t *testing.T) {
	r, fake := userRouter(77)

	if w := do(r, "PUT", "/users/sellers/78", `{"seller":{"name":"hijack"}}`); w.Code != http.StatusForbidden {
		t.Errorf("update other store: status %d, want 403", w.Code)
	}
	if w := do(r, "DELETE", "/users/sellers/78", ""); w.Code != http.StatusForbidden {
		t.Errorf("delete other store: status %d, want 403", w.Code)
	}
	if fake.updatedSeller != nil || len(fake.deletedSeller) != 0 {
		t.Error("foreign store requests reached user-service")
	}

	if w := do(r, "PUT", "/users/sellers/77", `{"user_id":5,"seller":{"id":5,"name":"Mine"}}`); w.Code != http.StatusOK {
		t.Fatalf("update own store: status %d: %s", w.Code, w.Body)
	}
	if fake.updatedSeller.GetSeller().GetId() != 77 || fake.updatedSeller.GetUserID() != callerID {
		t.Errorf("update sent seller=%d user=%d, want 77/%d", fake.updatedSeller.GetSeller().GetId(), fake.updatedSeller.GetUserID(), callerID)
	}
	if w := do(r, "PUT", "/users/sellers/77", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing seller: status %d, want 400", w.Code)
	}
	if w := do(r, "DELETE", "/users/sellers/77", ""); w.Code != http.StatusOK {
		t.Errorf("delete own store: status %d", w.Code)
	}
}

func TestSellerWithoutStoreCannotManageStores(t *testing.T) {
	r, _ := userRouter(0)
	if w := do(r, "PUT", "/users/sellers/0", `{"seller":{"name":"x"}}`); w.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403 (store 0 must never match)", w.Code)
	}
}

func TestSellerSensitiveFieldsAreOwnerOnly(t *testing.T) {
	get := func(storeID uint64) map[string]any {
		r, _ := userRouter(storeID)
		w := do(r, "GET", "/users/sellers/77", "")
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body)
		}
		var body struct {
			Seller map[string]any `json:"seller"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Seller
	}

	owner := get(77)
	if owner["bank_account"] != "VN-123" || owner["tax_code"] != "TAX" || owner["phone"] != "999" {
		t.Errorf("owner must see full profile: %v", owner)
	}
	public := get(1)
	if public["name"] != "Shop" || public["description"] != "d" {
		t.Errorf("public view lost its basic fields: %v", public)
	}
	for _, field := range []string{"bank_account", "tax_code", "phone", "address"} {
		if v, ok := public[field]; ok && v != "" {
			t.Errorf("public view leaks %s=%v", field, v)
		}
	}
}
