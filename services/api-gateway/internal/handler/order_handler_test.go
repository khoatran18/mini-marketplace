package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/clientname"
	orderpb "api-gateway/pkg/pb/orderservice"
	userpb "api-gateway/pkg/pb/userservice"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeOrders struct {
	orderpb.OrderServiceClient
	checkout *orderpb.CheckoutRequest
	replayed bool
	list     *orderpb.ListOrdersRequest
	get      *orderpb.GetOrderRequest
	count    *orderpb.CountOrdersRequest
	action   *orderpb.ActionRequest
	setItem  *orderpb.SetCartItemRequest
	merge    *orderpb.MergeCartRequest
	preview  *orderpb.PreviewRequest
	getCo    *orderpb.GetCheckoutRequest
	err      error
}

func (f *fakeOrders) Checkout(_ context.Context, in *orderpb.CheckoutRequest, _ ...grpc.CallOption) (*orderpb.CheckoutResponse, error) {
	f.checkout = in
	return &orderpb.CheckoutResponse{CheckoutId: "co_1", GrandTotal: 130000, PaymentMethod: in.PaymentMethod, Replayed: f.replayed,
		Orders: []*orderpb.Order{{Id: 7, StoreId: 10, Status: "PENDING", TotalPrice: 130000}}}, f.err
}
func (f *fakeOrders) ListOrders(_ context.Context, in *orderpb.ListOrdersRequest, _ ...grpc.CallOption) (*orderpb.ListOrdersResponse, error) {
	f.list = in
	return &orderpb.ListOrdersResponse{Total: 1, Orders: []*orderpb.Order{{Id: 7}}}, f.err
}
func (f *fakeOrders) GetOrder(_ context.Context, in *orderpb.GetOrderRequest, _ ...grpc.CallOption) (*orderpb.Order, error) {
	f.get = in
	return &orderpb.Order{Id: in.Id, Status: "PAID"}, f.err
}
func (f *fakeOrders) CountOrders(_ context.Context, in *orderpb.CountOrdersRequest, _ ...grpc.CallOption) (*orderpb.CountOrdersResponse, error) {
	f.count = in
	return &orderpb.CountOrdersResponse{ByStatus: map[string]int64{"PAID": 2}}, f.err
}
func (f *fakeOrders) ApplyAction(_ context.Context, in *orderpb.ActionRequest, _ ...grpc.CallOption) (*orderpb.Order, error) {
	f.action = in
	return &orderpb.Order{Id: in.OrderId, Status: "CANCELED"}, f.err
}
func (f *fakeOrders) SetCartItem(_ context.Context, in *orderpb.SetCartItemRequest, _ ...grpc.CallOption) (*orderpb.CartResponse, error) {
	f.setItem = in
	return &orderpb.CartResponse{}, f.err
}
func (f *fakeOrders) MergeCart(_ context.Context, in *orderpb.MergeCartRequest, _ ...grpc.CallOption) (*orderpb.CartResponse, error) {
	f.merge = in
	return &orderpb.CartResponse{}, f.err
}
func (f *fakeOrders) PreviewCheckout(_ context.Context, in *orderpb.PreviewRequest, _ ...grpc.CallOption) (*orderpb.PreviewResponse, error) {
	f.preview = in
	return &orderpb.PreviewResponse{CanOrder: true}, f.err
}
func (f *fakeOrders) GetCheckout(_ context.Context, in *orderpb.GetCheckoutRequest, _ ...grpc.CallOption) (*orderpb.CheckoutResponse, error) {
	f.getCo = in
	return &orderpb.CheckoutResponse{CheckoutId: in.CheckoutId}, f.err
}

type fakeUsers struct {
	userpb.UserServiceClient
	gotAddress *userpb.GetAddressRequest
	upserted   *userpb.UpsertAddressRequest
	deleted    *userpb.DeleteAddressRequest
	listed     *userpb.ListAddressesRequest
	missing    bool
}

func (f *fakeUsers) GetAddress(_ context.Context, in *userpb.GetAddressRequest, _ ...grpc.CallOption) (*userpb.AddressResponse, error) {
	f.gotAddress = in
	if f.missing {
		return nil, status.Error(codes.NotFound, "address not found")
	}
	return &userpb.AddressResponse{Address: &userpb.Address{Id: 3, UserId: in.UserId, Label: "home", ReceiverName: "An", Phone: "0901234567", Line1: "1 A St", City: "HCM"}}, nil
}
func (f *fakeUsers) UpsertAddress(_ context.Context, in *userpb.UpsertAddressRequest, _ ...grpc.CallOption) (*userpb.AddressResponse, error) {
	f.upserted = in
	return &userpb.AddressResponse{Address: in.Address}, nil
}
func (f *fakeUsers) DeleteAddress(_ context.Context, in *userpb.DeleteAddressRequest, _ ...grpc.CallOption) (*userpb.DeleteAddressResponse, error) {
	f.deleted = in
	return &userpb.DeleteAddressResponse{Success: true}, nil
}
func (f *fakeUsers) ListAddresses(_ context.Context, in *userpb.ListAddressesRequest, _ ...grpc.CallOption) (*userpb.ListAddressesResponse, error) {
	f.listed = in
	return &userpb.ListAddressesResponse{}, nil
}

// orderRouter wires the order routes as the given role; storeID is what auth-service reports for sellers.
func orderRouter(role string, storeID uint64) (*gin.Engine, *fakeOrders, *fakeUsers) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	orders, users := &fakeOrders{}, &fakeUsers{}
	cm := client.NewClientManager()
	cm.Clients[clientname.OrderClientName] = &client.ServiceClient{Client: orders}
	cm.Clients[clientname.UserClientName] = &client.ServiceClient{Client: users}
	h := NewOrderHandler(cm, authclient.NewAuthClient(&fakeAuthClient{storeID: storeID}, nil, logger), logger)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if role != "" {
			c.Set("userID", uint64(callerID))
			c.Set("userRole", role)
			c.Set("username", "u")
		}
	})
	r.GET("/cart", h.GetCart)
	r.PUT("/cart/items/:product_id", h.SetCartItem)
	r.POST("/cart/merge", h.MergeCart)
	r.POST("/checkout/preview", h.PreviewCheckout)
	r.POST("/orders", h.Checkout)
	r.GET("/checkouts/:id", h.GetCheckout)
	r.GET("/orders", h.ListMyOrders())
	r.GET("/orders/summary", h.CountMyOrders())
	r.GET("/orders/:id", h.GetMyOrder())
	r.POST("/orders/:id/cancel", h.BuyerCancel())
	r.DELETE("/orders/:id", h.BuyerCancel())
	r.POST("/orders/:id/return", h.BuyerRequestReturn())
	r.GET("/seller/orders", h.ListStoreOrders())
	r.GET("/seller/orders/:id", h.GetStoreOrder())
	r.POST("/seller/orders/:id/ship", h.SellerShip())
	r.POST("/seller/orders/:id/cancel", h.SellerCancel())
	r.GET("/admin/orders", h.ListAllOrders())
	r.POST("/admin/orders/:id/cancel", h.AdminCancel())
	r.GET("/users/me/addresses", h.ListAddresses)
	r.POST("/users/me/addresses", h.CreateAddress)
	r.PUT("/users/me/addresses/:id", h.UpdateAddress)
	r.DELETE("/users/me/addresses/:id", h.DeleteAddress)
	return r, orders, users
}

func TestCheckoutNeedsIdempotencyKeyAndIgnoresClientControlledFields(t *testing.T) {
	r, orders, users := orderRouter("buyer", 0)
	body := `{"buyer_id":999,"from_cart":true,"payment_method":"MOCK_CARD","note":"leave at door","address_id":3,
		"total":0.01,"status":"PAID","store_id":5,"items":[{"product_id":1,"quantity":2,"price":0.01}],"shipping_address":{"line1":"evil"}}`
	if w := do(r, "POST", "/orders", body); w.Code != http.StatusBadRequest {
		t.Fatalf("without Idempotency-Key: %d %s", w.Code, w.Body)
	}
	if orders.checkout != nil {
		t.Fatal("the request must not reach order-service without a key")
	}
	w := do(r, "POST", "/orders", body, "Idempotency-Key", "abc-123")
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	got := orders.checkout
	if got.BuyerId != callerID || got.IdempotencyKey != "abc-123" || got.PaymentMethod != "MOCK_CARD" || !got.FromCart || got.Note != "leave at door" || len(got.Items) != 1 {
		t.Errorf("checkout request: %+v", got)
	}
	// the address comes from user-service for THIS buyer; a body-supplied snapshot is ignored
	if users.gotAddress.UserId != callerID || users.gotAddress.Id != 3 {
		t.Errorf("address lookup: %+v", users.gotAddress)
	}
	if got.ShippingAddress.AsMap()["line1"] != "1 A St" || got.ShippingAddress.AsMap()["receiver_name"] != "An" {
		t.Errorf("snapshot: %v", got.ShippingAddress.AsMap())
	}
	var out struct {
		CheckoutID string           `json:"checkout_id"`
		GrandTotal float64          `json:"grand_total"`
		Orders     []map[string]any `json:"orders"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.CheckoutID != "co_1" || out.GrandTotal != 130000 || len(out.Orders) != 1 || out.Orders[0]["id"] != float64(7) {
		t.Errorf("response: %s", w.Body)
	}
	// a replayed key answers 200 instead of 201
	orders.replayed = true
	if w := do(r, "POST", "/orders", body, "Idempotency-Key", "abc-123"); w.Code != http.StatusOK {
		t.Errorf("replay status %d", w.Code)
	}
	// somebody else's address id -> user-service says not found -> 404, nothing created
	orders.checkout, users.missing = nil, true
	if w := do(r, "POST", "/orders", body, "Idempotency-Key", "k2"); w.Code != http.StatusNotFound || orders.checkout != nil {
		t.Errorf("unknown address: %d (order service called: %v)", w.Code, orders.checkout != nil)
	}
}

func TestBuyerOnlyEndpointsRejectOtherRoles(t *testing.T) {
	for _, role := range []string{"seller_admin", "admin"} {
		r, orders, _ := orderRouter(role, 7)
		for _, route := range [][2]string{{"GET", "/cart"}, {"POST", "/orders"}, {"GET", "/orders"}, {"GET", "/orders/1"}, {"POST", "/orders/1/cancel"}, {"DELETE", "/orders/1"}, {"GET", "/users/me/addresses"}} {
			if w := do(r, route[0], route[1], `{}`, "Idempotency-Key", "k"); w.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403", route[0], route[1], role, w.Code)
			}
		}
		if orders.checkout != nil || orders.list != nil || orders.action != nil {
			t.Errorf("%s: requests must not reach order-service", role)
		}
	}
	r, _, _ := orderRouter("", 0)
	if w := do(r, "GET", "/cart", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
}

func TestOrderReadsAreScopedByRole(t *testing.T) {
	r, orders, _ := orderRouter("buyer", 0)
	do(r, "GET", "/orders?status=paid&page=2&page_size=500&buyer_id=1&store_id=2", "")
	if l := orders.list; l.BuyerId != callerID || l.StoreId != 0 || l.IsAdmin || l.Status != "paid" || l.Page != 2 || l.PageSize != 20 {
		t.Errorf("buyer list: %+v (a client-supplied buyer/store must be ignored, big pages reset)", l)
	}
	do(r, "GET", "/orders/9", "")
	if g := orders.get; g.Id != 9 || g.BuyerId != callerID || g.StoreId != 0 || g.IsAdmin {
		t.Errorf("buyer get: %+v", g)
	}
	do(r, "GET", "/orders/summary", "")
	if orders.count.BuyerId != callerID {
		t.Errorf("count: %+v", orders.count)
	}

	r, orders, _ = orderRouter("seller_employee", 42)
	do(r, "GET", "/seller/orders?buyer_id=5", "")
	if l := orders.list; l.StoreId != 42 || l.BuyerId != 0 || l.IsAdmin {
		t.Errorf("seller list: %+v", l)
	}
	do(r, "GET", "/seller/orders/4", "")
	if g := orders.get; g.StoreId != 42 || g.BuyerId != 0 {
		t.Errorf("seller get: %+v", g)
	}

	r, orders, _ = orderRouter("admin", 0)
	do(r, "GET", "/admin/orders", "")
	if l := orders.list; !l.IsAdmin || l.BuyerId != 0 || l.StoreId != 0 {
		t.Errorf("admin list: %+v", l)
	}
	// a seller without a store and a buyer calling seller routes are refused
	r, _, _ = orderRouter("seller_admin", 0)
	if w := do(r, "GET", "/seller/orders", ""); w.Code != http.StatusForbidden {
		t.Errorf("seller without store: %d", w.Code)
	}
	r, _, _ = orderRouter("buyer", 0)
	if w := do(r, "GET", "/seller/orders", ""); w.Code != http.StatusForbidden {
		t.Errorf("buyer on seller route: %d", w.Code)
	}
	if w := do(r, "GET", "/admin/orders", ""); w.Code != http.StatusForbidden {
		t.Errorf("buyer on admin route: %d", w.Code)
	}
}

func TestActionsUseTheActorFromTheToken(t *testing.T) {
	r, orders, _ := orderRouter("buyer", 0)
	do(r, "POST", "/orders/5/cancel", `{"reason":"changed my mind","actor_type":"admin","actor_id":1,"store_id":9}`)
	a := orders.action
	if a.OrderId != 5 || a.Action != "cancel" || a.ActorType != "buyer" || a.ActorId != callerID || a.StoreId != 0 || a.Reason != "changed my mind" {
		t.Errorf("buyer cancel: %+v", a)
	}
	do(r, "DELETE", "/orders/6", "")
	if orders.action.OrderId != 6 || orders.action.Action != "cancel" || orders.action.ActorType != "buyer" {
		t.Errorf("legacy DELETE: %+v", orders.action)
	}
	do(r, "POST", "/orders/6/return", `{"reason":"broken"}`)
	if orders.action.Action != "request_return" || orders.action.Reason != "broken" {
		t.Errorf("return: %+v", orders.action)
	}

	r, orders, _ = orderRouter("seller_admin", 42)
	do(r, "POST", "/seller/orders/8/ship", `{"carrier":"GHN","tracking_code":"T1","store_id":999}`)
	a = orders.action
	if a.Action != "ship" || a.ActorType != "seller" || a.StoreId != 42 || a.Carrier != "GHN" || a.TrackingCode != "T1" || a.OrderId != 8 {
		t.Errorf("seller ship: %+v", a)
	}
	// a buyer can not use the seller action handlers even if the route were reachable
	r, orders, _ = orderRouter("buyer", 0)
	if w := do(r, "POST", "/seller/orders/8/ship", `{}`); w.Code != http.StatusForbidden || orders.action != nil {
		t.Errorf("buyer shipping: %d", w.Code)
	}
	r, orders, _ = orderRouter("admin", 0)
	do(r, "POST", "/admin/orders/8/cancel", `{"reason":"fraud"}`)
	if a = orders.action; a.ActorType != "admin" || a.Reason != "fraud" || a.StoreId != 0 {
		t.Errorf("admin cancel: %+v", a)
	}
	// malformed ids and bodies
	r, _, _ = orderRouter("buyer", 0)
	if w := do(r, "POST", "/orders/abc/cancel", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", w.Code)
	}
	if w := do(r, "POST", "/orders/1/cancel", `{bad json`); w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", w.Code)
	}
}

func TestOrderServiceErrorsAreMappedToHTTP(t *testing.T) {
	r, orders, _ := orderRouter("buyer", 0)
	for code, want := range map[codes.Code]int{
		codes.NotFound: 404, codes.FailedPrecondition: 422, codes.InvalidArgument: 400, codes.PermissionDenied: 403, codes.Unavailable: 503,
	} {
		orders.err = status.Error(code, "boom")
		if w := do(r, "POST", "/orders/1/cancel", `{}`); w.Code != want {
			t.Errorf("%v -> %d, want %d", code, w.Code, want)
		}
	}
}

func TestCartAndPreviewUseTheBuyerFromTheToken(t *testing.T) {
	r, orders, _ := orderRouter("buyer", 0)
	do(r, "PUT", "/cart/items/4", `{"quantity":3,"user_id":999}`)
	if s := orders.setItem; s.UserId != callerID || s.ProductId != 4 || s.Quantity != 3 {
		t.Errorf("set item: %+v", s)
	}
	do(r, "POST", "/cart/merge", `{"items":[{"product_id":1,"quantity":2},{"product_id":2,"quantity":1}],"user_id":999}`)
	if m := orders.merge; m.UserId != callerID || len(m.Items) != 2 {
		t.Errorf("merge: %+v", m)
	}
	do(r, "POST", "/checkout/preview", `{"from_cart":true,"buyer_id":999}`)
	if p := orders.preview; p.BuyerId != callerID || !p.FromCart {
		t.Errorf("preview: %+v", p)
	}
	do(r, "GET", "/checkouts/co_9", "")
	if g := orders.getCo; g.BuyerId != callerID || g.CheckoutId != "co_9" {
		t.Errorf("get checkout: %+v", g)
	}
	if w := do(r, "PUT", "/cart/items/0", `{"quantity":1}`); w.Code != http.StatusBadRequest {
		t.Errorf("product 0: %d", w.Code)
	}
}

func TestAddressEndpointsBelongToTheCaller(t *testing.T) {
	r, _, users := orderRouter("buyer", 0)
	do(r, "POST", "/users/me/addresses", `{"user_id":999,"receiver_name":"An","phone":"0901234567","line1":"1 A","city":"HCM","is_default":true}`)
	if a := users.upserted.Address; a.UserId != callerID || a.Id != 0 || !a.IsDefault || a.ReceiverName != "An" {
		t.Errorf("create: %+v", a)
	}
	do(r, "PUT", "/users/me/addresses/12", `{"id":99,"user_id":999,"receiver_name":"B","phone":"0901234567","line1":"2 B","city":"HN"}`)
	if a := users.upserted.Address; a.UserId != callerID || a.Id != 12 || a.ReceiverName != "B" {
		t.Errorf("update: %+v", a)
	}
	do(r, "DELETE", "/users/me/addresses/12", "")
	if users.deleted.UserId != callerID || users.deleted.Id != 12 {
		t.Errorf("delete: %+v", users.deleted)
	}
	do(r, "GET", "/users/me/addresses", "")
	if users.listed.UserId != callerID {
		t.Errorf("list: %+v", users.listed)
	}
}
