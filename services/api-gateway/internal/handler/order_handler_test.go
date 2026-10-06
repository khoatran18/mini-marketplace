package handler

import (
	"api-gateway/internal/client/orderclient"
	orderpb "api-gateway/pkg/pb/orderservice"
	userpb "api-gateway/pkg/pb/userservice"
	"buf.build/go/protovalidate"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeOrderClient is an in-memory order-service; the embedded interface panics on unused methods.
type fakeOrderClient struct {
	orderpb.OrderServiceClient
	ownerOf    uint64 // buyer of every order returned by GetOrderByID
	err        error
	created    *orderpb.CreateOrderRequest
	canceled   []uint64
	listedFor  []uint64
	listStatus string
}

func (f *fakeOrderClient) CreateOrder(_ context.Context, in *orderpb.CreateOrderRequest, _ ...grpc.CallOption) (*orderpb.CreateOrderResponse, error) {
	f.created = in
	return &orderpb.CreateOrderResponse{Message: "ok", Success: true}, f.err
}

func (f *fakeOrderClient) GetOrderByID(_ context.Context, in *orderpb.GetOrderByIDRequest, _ ...grpc.CallOption) (*orderpb.GetOrderByIDResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &orderpb.GetOrderByIDResponse{Message: "ok", Success: true, Order: &orderpb.Order{Id: in.GetId(), BuyerId: f.ownerOf, Status: "SUCCESS"}}, nil
}

func (f *fakeOrderClient) GetOrdersByBuyerIDStatus(_ context.Context, in *orderpb.GetOrdersByBuyerIDStatusRequest, _ ...grpc.CallOption) (*orderpb.GetOrdersByBuyerIDStatusResponse, error) {
	f.listedFor = append(f.listedFor, in.GetBuyerId())
	f.listStatus = in.GetStatus()
	return &orderpb.GetOrdersByBuyerIDStatusResponse{Message: "ok", Success: true}, f.err
}

func (f *fakeOrderClient) CancelOrderByID(_ context.Context, in *orderpb.CancelOrderByIDRequest, _ ...grpc.CallOption) (*orderpb.CancelOrderByIDResponse, error) {
	f.canceled = append(f.canceled, in.GetId())
	return &orderpb.CancelOrderByIDResponse{Message: "ok", Success: true}, f.err
}

const callerID = 42

func orderRouter(fake *fakeOrderClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewOrderHandler(orderclient.NewOrderClient(fake, nil, zap.NewNop()), zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", uint64(callerID)); c.Set("userRole", "buyer") })
	r.POST("/orders", h.CreateOrder)
	r.GET("/orders", h.GetOrdersByBuyerIDStatus)
	r.GET("/orders/:id", h.GetOrderByID)
	r.DELETE("/orders/:id", h.CancelOrderByID)
	return r
}

func do(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateOrderIgnoresClientControlledFields(t *testing.T) {
	fake := &fakeOrderClient{}
	body := `{"order":{"id":7,"buyer_id":999,"status":"SUCCESS","total_price":0.01,
		"order_items":[{"id":5,"order_id":8,"product_id":1,"quantity":2,"price":0.01}]}}`
	w := do(orderRouter(fake), "POST", "/orders", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	o := fake.created.GetOrder()
	if o.GetBuyerId() != callerID || o.GetStatus() != "PENDING" || o.GetTotalPrice() != 0 || o.GetId() != 0 {
		t.Errorf("client-controlled fields reached order-service: %+v", o)
	}
	item := o.GetOrderItem()[0]
	if item.GetPrice() != 0 || item.GetID() != 0 || item.GetOrderId() != 0 || item.GetQuantity() != 2 || item.GetProductId() != 1 {
		t.Errorf("unexpected item %+v", item)
	}
}

func TestCreateOrderRejectsBadPayloads(t *testing.T) {
	for name, body := range map[string]string{
		"no order":      `{}`,
		"null order":    `{"order":null}`,
		"no items":      `{"order":{"order_items":[]}}`,
		"zero quantity": `{"order":{"order_items":[{"product_id":1,"quantity":0}]}}`,
		"negative":      `{"order":{"order_items":[{"product_id":1,"quantity":-3}]}}`,
		"null item":     `{"order":{"order_items":[null]}}`,
		"not json":      `nope`,
	} {
		fake := &fakeOrderClient{}
		w := do(orderRouter(fake), "POST", "/orders", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, w.Code, w.Body)
		}
		if fake.created != nil {
			t.Errorf("%s: request must not reach order-service", name)
		}
	}
}

func TestOrdersOfOtherBuyersAreInvisible(t *testing.T) {
	fake := &fakeOrderClient{ownerOf: callerID + 1}
	r := orderRouter(fake)

	if w := do(r, "GET", "/orders/5", ""); w.Code != http.StatusNotFound {
		t.Errorf("GET foreign order: status %d, want 404", w.Code)
	}
	if w := do(r, "DELETE", "/orders/5", ""); w.Code != http.StatusNotFound {
		t.Errorf("DELETE foreign order: status %d, want 404", w.Code)
	}
	if len(fake.canceled) != 0 {
		t.Errorf("foreign order was canceled: %v", fake.canceled)
	}

	fake.ownerOf = callerID
	if w := do(r, "GET", "/orders/5", ""); w.Code != http.StatusOK {
		t.Errorf("GET own order: status %d, want 200", w.Code)
	}
	if w := do(r, "DELETE", "/orders/5", ""); w.Code != http.StatusOK {
		t.Errorf("DELETE own order: status %d, want 200", w.Code)
	}
	if len(fake.canceled) != 1 || fake.canceled[0] != 5 {
		t.Errorf("own order not canceled: %v", fake.canceled)
	}
}

func TestListOrdersUsesTokenIdentity(t *testing.T) {
	fake := &fakeOrderClient{}
	w := do(orderRouter(fake), "GET", "/orders?buyer_id=1&status=SUCCESS", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if len(fake.listedFor) != 1 || fake.listedFor[0] != callerID || fake.listStatus != "SUCCESS" {
		t.Errorf("listed for %v status %q; the buyer_id query parameter must be ignored", fake.listedFor, fake.listStatus)
	}
}

func TestInvalidOrderIDIsBadRequest(t *testing.T) {
	r := orderRouter(&fakeOrderClient{})
	for _, method := range []string{"GET", "DELETE"} {
		if w := do(r, method, "/orders/abc", ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s /orders/abc: status %d, want 400", method, w.Code)
		}
	}
}

func TestGrpcErrorsMapToHTTPStatus(t *testing.T) {
	cases := map[codes.Code]int{
		codes.InvalidArgument:    http.StatusBadRequest,
		codes.NotFound:           http.StatusNotFound,
		codes.PermissionDenied:   http.StatusForbidden,
		codes.FailedPrecondition: http.StatusUnprocessableEntity,
		codes.Unavailable:        http.StatusServiceUnavailable,
		codes.Internal:           http.StatusInternalServerError,
	}
	for code, want := range cases {
		fake := &fakeOrderClient{err: status.Error(code, "boom")}
		w := do(orderRouter(fake), "POST", "/orders", `{"order":{"order_items":[{"product_id":1,"quantity":1}]}}`)
		if w.Code != want {
			t.Errorf("%v: status %d, want %d", code, w.Code, want)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == nil {
			t.Errorf("%v: want JSON error body, got %s", code, w.Body)
		}
	}
}

func TestProtoValidationErrorsAreBadRequests(t *testing.T) {
	err := protovalidate.Validate(&userpb.Buyer{}) // name must not be empty
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if got := httpStatusFromError(err); got != http.StatusBadRequest {
		t.Errorf("status %d, want 400", got)
	}
}
