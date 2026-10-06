package service

import (
	"context"
	"fmt"
	"order-service/internal/repository"
	"order-service/pkg/model"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func act(t *testing.T, s *OrderService, orderID uint64, action string, actor repository.Actor, mutate ...func(*ActionInput)) (*OrderView, error) {
	t.Helper()
	in := &ActionInput{OrderID: orderID, Action: action, Actor: actor}
	for _, m := range mutate {
		m(in)
	}
	return s.ApplyAction(context.Background(), in)
}

var (
	buyer5  = repository.Actor{Type: repository.ActorBuyer, ID: 5}
	buyer6  = repository.Actor{Type: repository.ActorBuyer, ID: 6}
	seller1 = repository.Actor{Type: repository.ActorSeller, ID: 100, StoreID: 10}
	seller2 = repository.Actor{Type: repository.ActorSeller, ID: 200, StoreID: 20}
	admin   = repository.Actor{Type: repository.ActorAdmin, ID: 1}
	shipped = func(i *ActionInput) { i.Carrier, i.TrackingCode = "GHN", "TRK123" }
	because = func(r string) func(*ActionInput) { return func(i *ActionInput) { i.Reason = r } }
)

// reserveAll feeds the "stock reserved" result of product-service for every order of a checkout.
func reserveAll(t *testing.T, s *OrderService, res *CheckoutResult) {
	t.Helper()
	for _, o := range res.Orders {
		if err := s.UpdateOrderStatusByKafka(context.Background(), validated(o.ID, true)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOnlinePaymentHappyPathThroughReturn(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50), product(3, 20, 700000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodMockCard, "k1", Line{1, 2}, Line{3, 1})
	a, b := res.Orders[0], res.Orders[1] // totals 230000 and 700000

	// the first reservation result alone must not request the payment: another order is still PENDING
	if err := s.UpdateOrderStatusByKafka(ctx, validated(a.ID, true)); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, s, a.ID); got.Status != model.StatusAwaitingPayment || got.ExpiresAt == nil || time.Until(*got.ExpiresAt) < 14*time.Minute {
		t.Fatalf("order a: status=%s expires=%v", got.Status, got.ExpiresAt)
	}
	if n := len(emitted(t, db, repository.TopicPaymentRequest)); n != 0 {
		t.Fatalf("payment requested too early (%d events)", n)
	}
	if err := s.UpdateOrderStatusByKafka(ctx, validated(b.ID, true)); err != nil {
		t.Fatal(err)
	}
	reqs := emitted(t, db, repository.TopicPaymentRequest)
	if len(reqs) != 1 || reqs[0]["checkout_id"] != res.Checkout.ID || reqs[0]["amount_minor"] != float64(93000000) || reqs[0]["method"] != "MOCK_CARD" || len(reqs[0]["order_ids"].([]any)) != 2 {
		t.Fatalf("payment.requested: %v", reqs)
	}
	// duplicate / late validation results change nothing
	for i := 0; i < 2; i++ {
		_ = s.UpdateOrderStatusByKafka(ctx, validated(a.ID, true))
		_ = s.UpdateOrderStatusByKafka(ctx, validated(b.ID, false))
	}
	if len(emitted(t, db, repository.TopicPaymentRequest)) != 1 || statusOf(t, s, b.ID) != model.StatusAwaitingPayment {
		t.Fatal("replayed validation events must be ignored")
	}

	// payment arrives: both orders become PAID, replays are harmless
	for i := 0; i < 2; i++ {
		if err := s.HandlePaymentSucceeded(ctx, paymentSucceeded(res.Checkout.ID, 93000000)); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uint64{a.ID, b.ID} {
		if o := reload(t, s, id); o.Status != model.StatusPaid || o.PaymentStatus != model.PaymentPaid || o.PaidAt == nil {
			t.Fatalf("order %d after payment: %s/%s", id, o.Status, o.PaymentStatus)
		}
	}
	if n := len(emitted(t, db, repository.TopicRefundRequested)); n != 0 {
		t.Fatalf("an exact payment must not trigger refunds, got %d", n)
	}

	// store 10 ships: needs carrier + tracking; other stores / buyers can not
	if _, err := act(t, s, a.ID, ActionShip, seller1); status.Code(err) != codes.InvalidArgument {
		t.Errorf("ship without tracking: %v", err)
	}
	if _, err := act(t, s, a.ID, ActionShip, seller2, shipped); status.Code(err) != codes.NotFound {
		t.Errorf("another store must not see the order: %v", err)
	}
	if _, err := act(t, s, a.ID, ActionShip, buyer5, shipped); status.Code(err) != codes.PermissionDenied {
		t.Errorf("a buyer can not ship: %v", err)
	}
	v, err := act(t, s, a.ID, ActionShip, seller1, shipped)
	if err != nil || v.Order.Status != model.StatusShipped || v.Order.Carrier != "GHN" || v.Order.TrackingCode != "TRK123" || v.Order.ShippedAt == nil {
		t.Fatalf("ship: %v %+v", err, v)
	}
	// it can no longer be canceled
	if _, err := act(t, s, a.ID, ActionCancel, buyer5); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("cancel after shipping: %v", err)
	}
	// the buyer confirms receipt
	if v, err = act(t, s, a.ID, ActionConfirmReceived, buyer5); err != nil || v.Order.Status != model.StatusDelivered || v.Order.DeliveredAt == nil {
		t.Fatalf("confirm received: %v", err)
	}
	// the return window is open: request, then the seller approves -> REFUNDED with a refund event
	if _, err := act(t, s, a.ID, ActionRequestReturn, buyer5); status.Code(err) != codes.InvalidArgument {
		t.Errorf("a return needs a reason: %v", err)
	}
	if _, err := act(t, s, a.ID, ActionRequestReturn, buyer6, because("broken")); status.Code(err) != codes.NotFound {
		t.Errorf("another buyer: %v", err)
	}
	if v, err = act(t, s, a.ID, ActionRequestReturn, buyer5, because("broken")); err != nil || v.Order.Status != model.StatusRefundRequested || v.Order.ReturnReason != "broken" {
		t.Fatalf("request return: %v", err)
	}
	if v, err = act(t, s, a.ID, ActionApproveReturn, seller1); err != nil || v.Order.Status != model.StatusRefunded || v.Order.PaymentStatus != model.PaymentRefunded {
		t.Fatalf("approve return: %v %+v", err, v)
	}
	refunds := emitted(t, db, repository.TopicRefundRequested)
	if len(refunds) != 1 || refunds[0]["amount_minor"] != float64(23000000) || refunds[0]["refund_key"] != "order:"+itoa(a.ID)+":return" || refunds[0]["order_id"] != float64(a.ID) {
		t.Fatalf("refund event: %v", refunds)
	}
	// the timeline tells the whole story
	var path []string
	for _, h := range v.History {
		path = append(path, h.ToStatus)
	}
	want := []string{"PENDING", "AWAITING_PAYMENT", "PAID", "SHIPPED", "DELIVERED", "REFUND_REQUESTED", "REFUNDED"}
	if len(path) != len(want) {
		t.Fatalf("history = %v", path)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Fatalf("history = %v, want %v", path, want)
		}
	}
	// every status change was announced for analytics
	if got := len(emitted(t, db, repository.TopicStatusChanged)); got != 2+6+2 { // 2 created + a: 6 changes + b: validate + paid
		t.Errorf("status events = %d", got)
	}
}

func itoa(id uint64) string { return strconv.FormatUint(id, 10) }

func TestCashOnDeliveryFlow(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	res := checkout(t, s, 5, model.MethodCOD, "k1", Line{1, 1})
	o := res.Orders[0]
	reserveAll(t, s, res)
	got := reload(t, s, o.ID)
	if got.Status != model.StatusConfirmed || got.ExpiresAt != nil || got.PaymentStatus != model.PaymentUnpaid {
		t.Fatalf("COD after reservation: %s expires=%v payment=%s", got.Status, got.ExpiresAt, got.PaymentStatus)
	}
	if n := len(emitted(t, db, repository.TopicPaymentRequest)); n != 0 {
		t.Fatal("COD must never request an online payment")
	}
	if _, err := act(t, s, o.ID, ActionShip, seller1, shipped); err != nil {
		t.Fatal(err)
	}
	if got = reload(t, s, o.ID); got.PaymentStatus != model.PaymentUnpaid {
		t.Error("cash is not collected before delivery")
	}
	v, err := act(t, s, o.ID, ActionDeliver, seller1)
	if err != nil || v.Order.PaymentStatus != model.PaymentPaid || v.Order.PaidAt == nil || v.Order.DeliveredAt == nil {
		t.Fatalf("COD delivered must record the cash payment: %v %+v", err, v)
	}
	last := emitted(t, db, repository.TopicStatusChanged)
	if l := last[len(last)-1]; l["status"] != "DELIVERED" || l["payment_status"] != "PAID" {
		t.Errorf("last status event: %v", l)
	}
	// approving a return of a COD order does not ask the payment service for a refund
	_, _ = act(t, s, o.ID, ActionRequestReturn, buyer5, because("changed my mind"))
	if _, err := act(t, s, o.ID, ActionApproveReturn, admin); err != nil {
		t.Fatal(err)
	}
	if n := len(emitted(t, db, repository.TopicRefundRequested)); n != 0 {
		t.Errorf("COD return created %d online refund requests", n)
	}
}

func TestOutOfStockFailsOnlyThatOrderAndPaymentCoversTheRest(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50), product(3, 20, 200000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodMockWallet, "k1", Line{1, 1}, Line{3, 1})
	a, b := res.Orders[0], res.Orders[1]

	if err := s.UpdateOrderStatusByKafka(ctx, validated(a.ID, false)); err != nil { // store 10 is out of stock
		t.Fatal(err)
	}
	failed := reload(t, s, a.ID)
	if failed.Status != model.StatusFailed || failed.CancelReason != "out_of_stock" || failed.OrderItems[0].Status != "CANCELED" {
		t.Fatalf("failed order: %s %q %s", failed.Status, failed.CancelReason, failed.OrderItems[0].Status)
	}
	if len(emitted(t, db, repository.TopicCancelOrder)) != 0 {
		t.Error("a failed order never reserved stock, so nothing is released")
	}
	if len(emitted(t, db, repository.TopicPaymentRequest)) != 0 {
		t.Fatal("store 20 is still pending")
	}
	// the LAST order leaving PENDING (even by succeeding after a failure) requests the payment of the survivors only
	if err := s.UpdateOrderStatusByKafka(ctx, validated(b.ID, true)); err != nil {
		t.Fatal(err)
	}
	reqs := emitted(t, db, repository.TopicPaymentRequest)
	if len(reqs) != 1 || reqs[0]["amount_minor"] != float64(23000000) || len(reqs[0]["order_ids"].([]any)) != 1 {
		t.Fatalf("payment request must cover only the surviving order (230000): %v", reqs)
	}

	// when every order fails nothing is requested
	res2 := checkout(t, s, 5, model.MethodMockCard, "k2", Line{1, 1})
	_ = s.UpdateOrderStatusByKafka(ctx, validated(res2.Orders[0].ID, false))
	if len(emitted(t, db, repository.TopicPaymentRequest)) != 1 {
		t.Error("an all-failed checkout must not request a payment")
	}
}

func TestUnpaidOrdersExpireAndReleaseStock(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodMockCard, "k1", Line{1, 2})
	reserveAll(t, s, res)
	o := res.Orders[0]

	if n, err := s.ExpireUnpaid(ctx, time.Now().Add(10*time.Minute)); err != nil || n != 0 {
		t.Fatalf("before the deadline: %d %v", n, err)
	}
	later := time.Now().Add(16 * time.Minute)
	if n, err := s.ExpireUnpaid(ctx, later); err != nil || n != 1 {
		t.Fatalf("after the deadline: %d %v", n, err)
	}
	if n, _ := s.ExpireUnpaid(ctx, later); n != 0 {
		t.Error("an expired order must not expire twice")
	}
	got := reload(t, s, o.ID)
	if got.Status != model.StatusExpired || got.CancelReason != "payment_timeout" || got.CanceledBy != repository.ActorSystem || got.OrderItems[0].Status != "CANCELED" {
		t.Fatalf("expired order: %+v", got)
	}
	rel := emitted(t, db, repository.TopicCancelOrder)
	if len(rel) != 1 || rel[0]["order_id"] != float64(o.ID) {
		t.Fatalf("stock release events: %v", rel)
	}
	if it := rel[0]["items"].([]any)[0].(map[string]any); it["product_id"] != float64(1) || it["quantity"] != float64(2) {
		t.Errorf("release item: %v", it)
	}

	// a payment that lands after the expiry is refunded (the order is not payable any more)
	if err := s.HandlePaymentSucceeded(ctx, paymentSucceeded(res.Checkout.ID, 23000000)); err != nil {
		t.Fatal(err)
	}
	if statusOf(t, s, o.ID) != model.StatusExpired {
		t.Error("a late payment must not resurrect the order")
	}
	refunds := emitted(t, db, repository.TopicRefundRequested)
	if len(refunds) != 1 || refunds[0]["amount_minor"] != float64(23000000) || refunds[0]["refund_key"] != "checkout:"+res.Checkout.ID+":overpay" || refunds[0]["reason"] != "order_not_payable" {
		t.Fatalf("late payment refund: %v", refunds)
	}
	// COD orders never expire
	cod := checkout(t, s, 5, model.MethodCOD, "k2", Line{1, 1})
	reserveAll(t, s, cod)
	if n, _ := s.ExpireUnpaid(ctx, time.Now().Add(100*time.Hour)); n != 0 {
		t.Error("COD orders must not expire")
	}
}

func TestCancelRulesAndRefunds(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50), product(3, 20, 200000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodMockCard, "k1", Line{1, 1}, Line{3, 1})
	a, b := res.Orders[0], res.Orders[1]

	// PENDING orders are being processed and can not be canceled
	if _, err := act(t, s, a.ID, ActionCancel, buyer5); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("cancel while pending: %v", err)
	}
	reserveAll(t, s, res)
	// somebody else's order looks like it does not exist
	if _, err := act(t, s, a.ID, ActionCancel, buyer6); status.Code(err) != codes.NotFound {
		t.Errorf("foreign buyer: %v", err)
	}
	if _, err := act(t, s, a.ID, ActionCancel, seller2, because("x")); status.Code(err) != codes.NotFound {
		t.Errorf("foreign store: %v", err)
	}
	// sellers and admins must explain
	if _, err := act(t, s, a.ID, ActionCancel, seller1); status.Code(err) != codes.InvalidArgument {
		t.Errorf("seller cancel without reason: %v", err)
	}
	// the buyer cancels one unpaid order of the checkout: stock is released, nothing to refund yet
	v, err := act(t, s, a.ID, ActionCancel, buyer5, because("changed my mind"))
	if err != nil || v.Order.Status != model.StatusCanceled || v.Order.CanceledBy != "buyer" || v.Order.CancelReason != "changed my mind" {
		t.Fatalf("cancel: %v %+v", err, v)
	}
	if n := len(emitted(t, db, repository.TopicCancelOrder)); n != 1 {
		t.Fatalf("release events = %d", n)
	}
	if n := len(emitted(t, db, repository.TopicRefundRequested)); n != 0 {
		t.Fatal("an unpaid order has nothing to refund")
	}
	// the buyer still pays the whole checkout (230000 + 200000...): the canceled order's share is refunded
	total := toMinor(a.TotalPrice) + toMinor(b.TotalPrice)
	if err := s.HandlePaymentSucceeded(ctx, paymentSucceeded(res.Checkout.ID, total)); err != nil {
		t.Fatal(err)
	}
	if statusOf(t, s, a.ID) != model.StatusCanceled || statusOf(t, s, b.ID) != model.StatusPaid {
		t.Fatal("only the live order is paid")
	}
	refunds := emitted(t, db, repository.TopicRefundRequested)
	if len(refunds) != 1 || refunds[0]["amount_minor"] != float64(toMinor(a.TotalPrice)) {
		t.Fatalf("partial payment refund: %v (want %d)", refunds, toMinor(a.TotalPrice))
	}
	// cancelling a PAID order refunds it in full, once
	if v, err = act(t, s, b.ID, ActionCancel, seller2, because("out of stock")); err != nil || v.Order.CanceledBy != "seller" {
		t.Fatalf("seller cancel of a paid order: %v", err)
	}
	refunds = emitted(t, db, repository.TopicRefundRequested)
	if len(refunds) != 2 || refunds[1]["amount_minor"] != float64(toMinor(b.TotalPrice)) || refunds[1]["refund_key"] != "order:"+itoa(b.ID)+":cancel" {
		t.Fatalf("paid cancel refund: %v", refunds)
	}
	if _, err := act(t, s, b.ID, ActionCancel, admin, because("again")); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("a canceled order is terminal: %v", err)
	}
}

func TestReturnWindowAndRejection(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	res := checkout(t, s, 5, model.MethodCOD, "k1", Line{1, 1})
	reserveAll(t, s, res)
	o := res.Orders[0]
	_, _ = act(t, s, o.ID, ActionShip, seller1, shipped)
	_, _ = act(t, s, o.ID, ActionDeliver, admin)

	// the window closes after 7 days
	db.Model(&model.Order{}).Where("id = ?", o.ID).Update("delivered_at", time.Now().Add(-8*24*time.Hour))
	if _, err := act(t, s, o.ID, ActionRequestReturn, buyer5, because("late")); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("after the window: %v", err)
	}
	db.Model(&model.Order{}).Where("id = ?", o.ID).Update("delivered_at", time.Now().Add(-2*24*time.Hour))
	if _, err := act(t, s, o.ID, ActionRequestReturn, buyer5, because("damaged")); err != nil {
		t.Fatal(err)
	}
	// only the seller/admin decide; a rejection returns the order to DELIVERED and closes the return
	if _, err := act(t, s, o.ID, ActionApproveReturn, buyer5); status.Code(err) != codes.PermissionDenied {
		t.Errorf("buyer approving own return: %v", err)
	}
	if _, err := act(t, s, o.ID, ActionRejectReturn, seller1); status.Code(err) != codes.InvalidArgument {
		t.Errorf("rejecting needs a reason: %v", err)
	}
	v, err := act(t, s, o.ID, ActionRejectReturn, seller1, because("used product"))
	if err != nil || v.Order.Status != model.StatusDelivered {
		t.Fatalf("reject: %v", err)
	}
	if _, err := act(t, s, o.ID, ActionRequestReturn, buyer5, because("again")); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("a rejected return can not be requested again: %v", err)
	}
	// unknown action
	if _, err := act(t, s, o.ID, "explode", admin); status.Code(err) != codes.InvalidArgument {
		t.Errorf("unknown action: %v", err)
	}
}

func TestAutoDeliverShippedOrders(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodCOD, "k1", Line{1, 1})
	reserveAll(t, s, res)
	o := res.Orders[0]
	_, _ = act(t, s, o.ID, ActionShip, seller1, shipped)

	if n, _ := s.AutoDeliver(ctx, time.Now().Add(3*24*time.Hour)); n != 0 {
		t.Error("too early to auto-deliver")
	}
	if n, err := s.AutoDeliver(ctx, time.Now().Add(8*24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("auto deliver: %d %v", n, err)
	}
	got := reload(t, s, o.ID)
	if got.Status != model.StatusDelivered || got.PaymentStatus != model.PaymentPaid {
		t.Errorf("auto delivered COD order: %s/%s", got.Status, got.PaymentStatus)
	}
	s.Settings.AutoDeliverAfter = 0
	if n, _ := s.AutoDeliver(ctx, time.Now().Add(100*24*time.Hour)); n != 0 {
		t.Error("AUTO_DELIVER_DAYS=0 disables auto delivery")
	}
}

func TestReadsAreScopedToTheCaller(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 1000, 500), product(3, 20, 1000, 500))
	ctx := context.Background()
	mine := checkout(t, s, 5, model.MethodCOD, "a", Line{1, 1}, Line{3, 1}) // 2 orders: stores 10 and 20
	for i := 0; i < 3; i++ {
		checkout(t, s, 6, model.MethodCOD, "b"+string(rune('0'+i)), Line{1, 1})
	}
	reserveAll(t, s, mine)

	own := repository.Scope{BuyerID: 5}
	if _, err := s.GetOrder(ctx, mine.Orders[0].ID, repository.Scope{BuyerID: 6}); status.Code(err) != codes.NotFound {
		t.Errorf("buyer 6 reading buyer 5's order: %v", err)
	}
	if _, err := s.GetOrder(ctx, mine.Orders[0].ID, repository.Scope{StoreID: 20}); status.Code(err) != codes.NotFound {
		t.Errorf("store 20 reading store 10's order: %v", err)
	}
	if _, err := s.GetOrder(ctx, mine.Orders[0].ID, repository.Scope{}); status.Code(err) != codes.NotFound {
		t.Errorf("no scope must see nothing: %v", err)
	}
	if v, err := s.GetOrder(ctx, mine.Orders[0].ID, own); err != nil || len(v.Order.OrderItems) != 1 || len(v.History) != 2 {
		t.Errorf("own order: %v %+v", err, v)
	}
	if v, err := s.GetOrder(ctx, mine.Orders[0].ID, repository.Scope{StoreID: 10}); err != nil || v.Order.ID != mine.Orders[0].ID {
		t.Errorf("the store reads its order: %v", err)
	}

	orders, total, _ := s.ListOrders(ctx, own, "", 1, 20)
	if total != 2 || len(orders) != 2 || orders[0].ID < orders[1].ID {
		t.Errorf("buyer list (newest first): total=%d", total)
	}
	if _, total, _ := s.ListOrders(ctx, repository.Scope{StoreID: 10}, "", 1, 20); total != 4 {
		t.Errorf("store 10 sees %d orders, want 4", total)
	}
	if _, total, _ := s.ListOrders(ctx, repository.Scope{Admin: true}, "", 1, 2); total != 5 {
		t.Errorf("admin total = %d", total)
	}
	if o, _, _ := s.ListOrders(ctx, repository.Scope{Admin: true}, "", 2, 2); len(o) != 2 {
		t.Errorf("page 2 of 2: %d", len(o))
	}
	if o, total, _ := s.ListOrders(ctx, own, "confirmed", 1, 20); total != 2 || len(o) != 2 {
		t.Errorf("status filter is case-insensitive: %d", total)
	}
	if o, total, _ := s.ListOrders(ctx, repository.Scope{}, "", 1, 20); total != 0 || len(o) != 0 {
		t.Error("an empty scope lists nothing")
	}
	if _, _, err := s.ListOrders(ctx, own, "NOPE", 1, 20); status.Code(err) != codes.InvalidArgument {
		t.Errorf("unknown status filter: %v", err)
	}
	counts, _ := s.CountOrders(ctx, repository.Scope{StoreID: 10})
	if counts[model.StatusConfirmed] != 1 || counts[model.StatusPending] != 3 {
		t.Errorf("counts = %v", counts)
	}
	// checkouts are private to the buyer
	if _, err := s.GetCheckout(ctx, 6, mine.Checkout.ID); status.Code(err) != codes.NotFound {
		t.Errorf("checkout of another buyer: %v", err)
	}
	if c, err := s.GetCheckout(ctx, 5, mine.Checkout.ID); err != nil || len(c.Orders) != 2 {
		t.Errorf("own checkout: %v", err)
	}
}

func TestLegacySuccessOrdersAreMigratedToConfirmed(t *testing.T) {
	db := testDB(t)
	db.Create(&model.Order{BuyerID: 1, Status: model.LegacyStatusSuccess, TotalPrice: 10})
	if err := repository.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var o model.Order
	db.First(&o)
	if o.Status != model.StatusConfirmed || o.PaymentMethod != model.MethodCOD {
		t.Errorf("legacy order after migration: %s / %s", o.Status, o.PaymentMethod)
	}
}

func TestPaymentRefundedEventMarksTheOrder(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodMockCard, "k", Line{1, 1})
	reserveAll(t, s, res)
	_ = s.HandlePaymentSucceeded(ctx, paymentSucceeded(res.Checkout.ID, 13000000))
	o := res.Orders[0]
	_, _ = act(t, s, o.ID, ActionCancel, buyer5, because("x"))
	if err := s.HandlePaymentRefunded(ctx, msg(fmt.Sprintf(`{"order_id":%d}`, o.ID))); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, s, o.ID); got.PaymentStatus != model.PaymentRefunded {
		t.Errorf("payment status = %s", got.PaymentStatus)
	}
	// malformed events are dropped, not retried
	if err := s.HandlePaymentRefunded(ctx, msg("nope")); err != nil {
		t.Error("malformed refunded event must be dropped")
	}
	if err := s.HandlePaymentSucceeded(ctx, msg("nope")); err != nil {
		t.Error("malformed succeeded event must be dropped")
	}
	if err := s.UpdateOrderStatusByKafka(ctx, msg("nope")); err != nil {
		t.Error("malformed validate event must be dropped")
	}
	if err := s.UpdateOrderStatusByKafka(ctx, validated(987654, true)); err != nil {
		t.Errorf("events for unknown orders are ignored: %v", err)
	}
}

func TestConcurrentActionsOnOneOrderAreSerialised(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	ctx := context.Background()
	res := checkout(t, s, 5, model.MethodMockCard, "k", Line{1, 1})
	reserveAll(t, s, res)
	if err := s.HandlePaymentSucceeded(ctx, paymentSucceeded(res.Checkout.ID, 13000000)); err != nil {
		t.Fatal(err)
	}
	o := res.Orders[0]

	// the seller ships while the buyer cancels: exactly one wins, the history never shows both
	results := make(chan string, 2)
	go func() {
		_, err := act(t, s, o.ID, ActionShip, seller1, shipped)
		results <- map[bool]string{true: "ship", false: ""}[err == nil]
	}()
	go func() {
		_, err := act(t, s, o.ID, ActionCancel, buyer5)
		results <- map[bool]string{true: "cancel", false: ""}[err == nil]
	}()
	won := []string{<-results, <-results}
	if (won[0] == "") == (won[1] == "") {
		t.Fatalf("exactly one action must win, got %v", won)
	}
	final := statusOf(t, s, o.ID)
	if final != model.StatusShipped && final != model.StatusCanceled {
		t.Fatalf("final status %s", final)
	}
	_, hist, _ := s.OrderRepo.GetOrder(ctx, o.ID, repository.Scope{Admin: true})
	if last := hist[len(hist)-1].ToStatus; last != final {
		t.Errorf("history ends with %s but the order is %s", last, final)
	}
	// duplicate deliveries of the same payment event running in parallel pay once and refund nothing
	res2 := checkout(t, s, 5, model.MethodMockCard, "k2", Line{1, 1})
	reserveAll(t, s, res2)
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { done <- s.HandlePaymentSucceeded(ctx, paymentSucceeded(res2.Checkout.ID, 13000000)) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if got := reload(t, s, res2.Orders[0].ID); got.Status != model.StatusPaid {
		t.Errorf("status %s", got.Status)
	}
	var paid int64
	db.Model(&model.OrderStatusHistory{}).Where("order_id = ? AND to_status = ?", res2.Orders[0].ID, model.StatusPaid).Count(&paid)
	if paid != 1 {
		t.Errorf("the order was marked paid %d times", paid)
	}
	if n := len(emitted(t, db, repository.TopicRefundRequested)); final == model.StatusShipped && n != 0 {
		t.Errorf("no refunds expected, got %d", n)
	}
}
