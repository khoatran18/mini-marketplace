package handler

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/clientname"
	paymentpb "api-gateway/pkg/pb/paymentservice"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePayments struct {
	paymentpb.PaymentServiceClient
	get       *paymentpb.GetPaymentRequest
	byCo      *paymentpb.GetPaymentByCheckoutRequest
	confirm   *paymentpb.ConfirmPaymentRequest
	cancel    *paymentpb.CancelPaymentRequest
	list      *paymentpb.ListPaymentsRequest
	refund    *paymentpb.RefundPaymentRequest
	force     *paymentpb.ForceStatusRequest
	err       error
	withAdmin bool
}

func (f *fakePayments) sample(id uint64) *paymentpb.Payment {
	p := &paymentpb.Payment{Id: id, CheckoutId: "co_1", BuyerId: callerID, Method: "MOCK_CARD", Status: "REQUIRES_ACTION", AmountMinor: 23000000, RefundedMinor: 500, NextAction: "enter_card"}
	if f.withAdmin {
		p.Attempts = []*paymentpb.Attempt{{Id: 1, Scenario: "card_declined", Status: "FAILED", FailureCode: "card_declined"}}
	}
	return p
}
func (f *fakePayments) GetPayment(_ context.Context, in *paymentpb.GetPaymentRequest, _ ...grpc.CallOption) (*paymentpb.Payment, error) {
	f.get = in
	return f.sample(in.Id), f.err
}
func (f *fakePayments) GetPaymentByCheckout(_ context.Context, in *paymentpb.GetPaymentByCheckoutRequest, _ ...grpc.CallOption) (*paymentpb.Payment, error) {
	f.byCo = in
	return f.sample(9), f.err
}
func (f *fakePayments) ConfirmPayment(_ context.Context, in *paymentpb.ConfirmPaymentRequest, _ ...grpc.CallOption) (*paymentpb.Payment, error) {
	f.confirm = in
	return f.sample(in.PaymentId), f.err
}
func (f *fakePayments) CancelPayment(_ context.Context, in *paymentpb.CancelPaymentRequest, _ ...grpc.CallOption) (*paymentpb.Payment, error) {
	f.cancel = in
	return f.sample(in.PaymentId), f.err
}
func (f *fakePayments) ListPayments(_ context.Context, in *paymentpb.ListPaymentsRequest, _ ...grpc.CallOption) (*paymentpb.ListPaymentsResponse, error) {
	f.list = in
	return &paymentpb.ListPaymentsResponse{Total: 1, Payments: []*paymentpb.Payment{f.sample(1)}}, f.err
}
func (f *fakePayments) RefundPayment(_ context.Context, in *paymentpb.RefundPaymentRequest, _ ...grpc.CallOption) (*paymentpb.Payment, error) {
	f.refund = in
	return f.sample(in.PaymentId), f.err
}
func (f *fakePayments) ForceStatus(_ context.Context, in *paymentpb.ForceStatusRequest, _ ...grpc.CallOption) (*paymentpb.Payment, error) {
	f.force = in
	return f.sample(in.PaymentId), f.err
}

func paymentRouter(role string) (*gin.Engine, *fakePayments) {
	gin.SetMode(gin.TestMode)
	fake := &fakePayments{}
	cm := client.NewClientManager()
	cm.Clients[clientname.PaymentClientName] = &client.ServiceClient{Client: fake}
	h := NewPaymentHandler(cm, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if role != "" {
			c.Set("userID", uint64(callerID))
			c.Set("userRole", role)
			c.Set("username", "u")
		}
	})
	r.GET("/payments/methods", h.Methods)
	r.GET("/payments/:id", h.Get)
	r.POST("/payments/:id/confirm", h.Confirm)
	r.POST("/payments/:id/cancel", h.Cancel)
	r.GET("/admin/payments", h.AdminList)
	r.GET("/admin/payments/:id", h.AdminGet)
	r.POST("/admin/payments/:id/refund", h.AdminRefund)
	r.POST("/admin/dev/payments/:id/force", h.AdminForce)
	return r, fake
}

func TestPaymentMethodsAreLabelledAsSimulated(t *testing.T) {
	r, _ := paymentRouter("")
	w := do(r, "GET", "/payments/methods", "")
	var out struct {
		Methods []struct {
			Code      string           `json:"code"`
			Online    bool             `json:"online"`
			Simulated bool             `json:"simulated"`
			TestCards []map[string]any `json:"test_cards"`
		} `json:"methods"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || len(out.Methods) != 4 {
		t.Fatalf("methods: %d %s", w.Code, w.Body)
	}
	for _, m := range out.Methods {
		if m.Code == "COD" && (m.Online || m.Simulated) {
			t.Error("COD is the offline, non-simulated method")
		}
		if m.Code != "COD" && (!m.Online || !m.Simulated) {
			t.Errorf("%s must be an online simulation", m.Code)
		}
		if m.Code == "MOCK_CARD" && len(m.TestCards) != 8 {
			t.Errorf("the 8 documented test cards must be listed, got %d", len(m.TestCards))
		}
	}
}

func TestBuyerPaymentCallsUseTheBuyerFromTheToken(t *testing.T) {
	r, fake := paymentRouter("buyer")
	w := do(r, "GET", "/payments/9?buyer_id=1", "")
	if fake.get.BuyerId != callerID || fake.get.IsAdmin || fake.get.Id != 9 {
		t.Errorf("get: %+v", fake.get)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["amount"] != float64(230000) || out["pay_url"] != "/pay/9" || out["simulated"] != true || out["next_action"] != "enter_card" || out["refunded"] != float64(5) {
		t.Errorf("payment json: %v", out)
	}
	if _, has := out["attempts"]; has {
		t.Error("buyers must not receive admin details")
	}
	do(r, "POST", "/payments/9/confirm", `{"card_number":"4242 4242 4242 4242","card_exp":"12/30","card_cvc":"123","otp":"123456","approve":true,"buyer_id":999}`)
	c := fake.confirm
	if c.PaymentId != 9 || c.BuyerId != callerID || c.CardNumber != "4242 4242 4242 4242" || c.CardExp != "12/30" || c.CardCvc != "123" || c.Otp != "123456" || !c.Approve {
		t.Errorf("confirm: %+v", c)
	}
	do(r, "POST", "/payments/9/cancel", "")
	if fake.cancel.PaymentId != 9 || fake.cancel.BuyerId != callerID {
		t.Errorf("cancel: %+v", fake.cancel)
	}
}

func TestPaymentBadInputIsNotEchoedBack(t *testing.T) {
	r, fake := paymentRouter("buyer")
	w := do(r, "POST", "/payments/9/confirm", `{"card_number":"4242424242424242","card_cvc":`) // truncated JSON containing card data
	if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "4242") || fake.confirm != nil {
		t.Errorf("bad body: %d %s", w.Code, w.Body)
	}
	if w := do(r, "POST", "/payments/abc/confirm", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", w.Code)
	}
	for code, want := range map[codes.Code]int{codes.NotFound: 404, codes.FailedPrecondition: 422, codes.InvalidArgument: 400, codes.Unavailable: 503} {
		fake.err = status.Error(code, "boom")
		if w := do(r, "POST", "/payments/9/confirm", `{}`); w.Code != want {
			t.Errorf("%v -> %d, want %d", code, w.Code, want)
		}
	}
}

func TestAnonymousCannotTouchPayments(t *testing.T) {
	r, fake := paymentRouter("")
	for _, route := range [][2]string{{"GET", "/payments/1"}, {"POST", "/payments/1/confirm"}, {"POST", "/payments/1/cancel"}} {
		if w := do(r, route[0], route[1], `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", route[0], route[1], w.Code)
		}
	}
	if fake.get != nil || fake.confirm != nil || fake.cancel != nil {
		t.Error("anonymous requests must not reach payment-service")
	}
}

func TestAdminPaymentEndpoints(t *testing.T) {
	r, fake := paymentRouter("admin")
	fake.withAdmin = true
	w := do(r, "GET", "/admin/payments/4", "")
	if !fake.get.IsAdmin || fake.get.BuyerId != 0 {
		t.Errorf("admin get: %+v", fake.get)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out["attempts"].([]any)) != 1 {
		t.Errorf("admins see attempts: %v", out)
	}
	do(r, "GET", "/admin/payments?status=SUCCEEDED&method=MOCK_CARD&page=2&page_size=500", "")
	if l := fake.list; l.Status != "SUCCEEDED" || l.Method != "MOCK_CARD" || l.Page != 2 || l.PageSize != 20 {
		t.Errorf("list: %+v", l)
	}
	do(r, "POST", "/admin/payments/4/refund", `{"amount_minor":500000,"reason":"goodwill"}`)
	if fake.refund.PaymentId != 4 || fake.refund.AmountMinor != 500000 || fake.refund.Reason != "goodwill" {
		t.Errorf("refund: %+v", fake.refund)
	}
	for _, body := range []string{`{}`, `{"amount_minor":-5}`, `nope`} {
		if w := do(r, "POST", "/admin/payments/4/refund", body); w.Code != http.StatusBadRequest {
			t.Errorf("refund body %q: %d", body, w.Code)
		}
	}
	do(r, "POST", "/admin/dev/payments/4/force", `{"status":"SUCCEEDED"}`)
	if fake.force.PaymentId != 4 || fake.force.Status != "SUCCEEDED" {
		t.Errorf("force: %+v", fake.force)
	}
	fake.err = status.Error(codes.PermissionDenied, "only in development")
	if w := do(r, "POST", "/admin/dev/payments/4/force", `{"status":"SUCCEEDED"}`); w.Code != http.StatusForbidden {
		t.Errorf("force outside dev: %d", w.Code)
	}
}

func TestCheckoutResponseCarriesThePayment(t *testing.T) {
	r, orders, _ := orderRouter("buyer", 0)
	_ = orders
	// compose a PaymentHandler backed by a fake payment client into the order handler of a fresh router
	gin.SetMode(gin.TestMode)
	fakeOrd, fakePay := &fakeOrders{}, &fakePayments{}
	cm := client.NewClientManager()
	cm.Clients[clientname.OrderClientName] = &client.ServiceClient{Client: fakeOrd}
	cm.Clients[clientname.PaymentClientName] = &client.ServiceClient{Client: fakePay}
	cm.Clients[clientname.UserClientName] = &client.ServiceClient{Client: &fakeUsers{}}
	oh := NewOrderHandler(cm, nil, zap.NewNop())
	oh.Payments = NewPaymentHandler(cm, zap.NewNop())
	r = gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", uint64(callerID)); c.Set("userRole", "buyer") })
	r.GET("/checkouts/:id", oh.GetCheckout)
	w := do(r, "GET", "/checkouts/co_1", "")
	var out struct {
		CheckoutID string         `json:"checkout_id"`
		Payment    map[string]any `json:"payment"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.CheckoutID != "co_1" || out.Payment["pay_url"] != "/pay/9" || fakePay.byCo.BuyerId != callerID || fakePay.byCo.CheckoutId != "co_1" {
		t.Errorf("checkout with payment: %s (lookup %+v)", w.Body, fakePay.byCo)
	}
	// a missing payment (COD or not created yet) simply leaves the field out
	fakePay.err = status.Error(codes.NotFound, "payment not found")
	w = do(r, "GET", "/checkouts/co_1", "")
	var plain map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &plain)
	if _, has := plain["payment"]; has || w.Code != 200 {
		t.Errorf("no payment: %d %v", w.Code, plain)
	}
}
