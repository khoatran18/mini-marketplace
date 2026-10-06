package service

import (
	"context"
	"order-service/internal/repository"
	"order-service/pkg/model"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToMinorRoundsToCents(t *testing.T) {
	for in, want := range map[float64]int64{0.1 + 0.2: 30, 19.99: 1999, 0: 0, 123456789.12: 12345678912} {
		if got := toMinor(in); got != want {
			t.Errorf("toMinor(%v) = %d, want %d", in, got, want)
		}
	}
	if fromMinor(1999) != 19.99 {
		t.Errorf("fromMinor(1999) = %v", fromMinor(1999))
	}
}

func TestCheckoutSplitsOrdersByStoreAndPricesOnTheServer(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50), product(2, 10, 50000, 50), product(3, 20, 700000, 50))
	res := checkout(t, s, 5, model.MethodMockCard, "k1", Line{1, 2}, Line{2, 1}, Line{3, 1}, Line{1, 1}) // product 1 listed twice

	if len(res.Orders) != 2 || res.Replayed {
		t.Fatalf("want 2 orders (one per store), got %d replayed=%v", len(res.Orders), res.Replayed)
	}
	a, b := res.Orders[0], res.Orders[1] // ordered by store id
	if a.StoreID != 10 || b.StoreID != 20 || a.CheckoutID != res.Checkout.ID || b.CheckoutID != res.Checkout.ID {
		t.Fatalf("stores/checkout ids wrong: %+v %+v", a, b)
	}
	// store 10: 3x100000 + 1x50000 = 350000 (+30000 shipping, below the 500000 free-shipping threshold)
	if a.Subtotal != 350000 || a.ShippingFee != 30000 || a.TotalPrice != 380000 {
		t.Errorf("store 10 totals: %v %v %v", a.Subtotal, a.ShippingFee, a.TotalPrice)
	}
	// store 20: 700000 >= 500000 -> free shipping
	if b.Subtotal != 700000 || b.ShippingFee != 0 || b.TotalPrice != 700000 {
		t.Errorf("store 20 totals: %v %v %v", b.Subtotal, b.ShippingFee, b.TotalPrice)
	}
	if res.Checkout.Total != 1080000 {
		t.Errorf("grand total = %v", res.Checkout.Total)
	}
	if len(a.OrderItems) != 2 || a.OrderItems[0].Quantity != 3 || a.OrderItems[0].ProductName != "product-1" || a.OrderItems[0].SKU != "SKU-1" || a.OrderItems[0].CategoryID != 3 {
		t.Errorf("items must be merged and carry a catalog snapshot: %+v", a.OrderItems[0])
	}
	if a.Status != model.StatusPending || a.PaymentStatus != model.PaymentUnpaid || a.ExpiresAt != nil {
		t.Errorf("new order: status=%s payment=%s expires=%v", a.Status, a.PaymentStatus, a.ExpiresAt)
	}
	// one stock reservation request and one status event per order, with the real order ids
	creates := emitted(t, db, repository.TopicCreateOrder)
	if len(creates) != 2 || creates[0]["order_id"] != float64(a.ID) || creates[1]["order_id"] != float64(b.ID) {
		t.Fatalf("create_order events: %v", creates)
	}
	items := creates[0]["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["quantity"] != float64(3) {
		t.Errorf("stock event items: %v", items)
	}
	if st := emitted(t, db, repository.TopicStatusChanged); len(st) != 2 || st[0]["status"] != "PENDING" || st[0]["total_minor"] != float64(38000000) {
		t.Errorf("status events: %v", st)
	}
	// history starts with the creation by the buyer
	_, history, _ := s.OrderRepo.GetOrder(context.Background(), a.ID, repository.Scope{BuyerID: 5})
	if len(history) != 1 || history[0].ToStatus != model.StatusPending || history[0].ActorType != repository.ActorBuyer {
		t.Errorf("history: %+v", history)
	}
}

func TestCheckoutIsIdempotentPerBuyerAndKey(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 1000, 50))
	first := checkout(t, s, 5, model.MethodCOD, "same-key", Line{1, 1})
	again := checkout(t, s, 5, model.MethodCOD, "same-key", Line{1, 7}) // even a different body returns the original
	if !again.Replayed || again.Checkout.ID != first.Checkout.ID || len(again.Orders) != 1 || again.Orders[0].ID != first.Orders[0].ID {
		t.Fatalf("replay must return the original checkout: %+v", again)
	}
	if again.Orders[0].OrderItems[0].Quantity != 1 {
		t.Error("the replay must not re-price or change the order")
	}
	// another buyer may reuse the key; another key creates a new checkout
	other := checkout(t, s, 6, model.MethodCOD, "same-key", Line{1, 1})
	third := checkout(t, s, 5, model.MethodCOD, "other-key", Line{1, 1})
	if other.Checkout.ID == first.Checkout.ID || third.Checkout.ID == first.Checkout.ID {
		t.Error("distinct (buyer, key) pairs must create distinct checkouts")
	}
	var n int64
	db.Model(&model.Order{}).Count(&n)
	if n != 3 {
		t.Errorf("orders in db = %d, want 3", n)
	}

	// concurrent duplicates (double click, client retry) create exactly one checkout
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := s.Checkout(context.Background(), &CheckoutInput{BuyerID: 9, Items: []Line{{1, 1}}, PaymentMethod: model.MethodCOD, Address: testAddress, IdempotencyKey: "race"})
			if err == nil {
				ids[i] = r.Checkout.ID
			}
		}(i)
	}
	wg.Wait()
	db.Model(&model.Checkout{}).Where("buyer_id = 9").Count(&n)
	if n != 1 {
		t.Errorf("concurrent duplicates created %d checkouts", n)
	}
	for _, id := range ids {
		if id != "" && id != ids[0] && ids[0] != "" {
			t.Errorf("duplicates must all return the same checkout: %v", ids)
		}
	}
}

func TestCheckoutValidation(t *testing.T) {
	db := testDB(t)
	inactive := product(2, 10, 1000, 5)
	inactive.Status = "hidden"
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 1000, 5), inactive, product(3, 10, 1000, 0))
	ctx := context.Background()
	base := func() *CheckoutInput {
		return &CheckoutInput{BuyerID: 5, Items: []Line{{1, 1}}, PaymentMethod: model.MethodCOD, Address: testAddress, IdempotencyKey: "k"}
	}
	cases := map[string]struct {
		mutate func(*CheckoutInput)
		code   codes.Code
	}{
		"no buyer":              {func(i *CheckoutInput) { i.BuyerID = 0 }, codes.InvalidArgument},
		"bad method":            {func(i *CheckoutInput) { i.PaymentMethod = "BITCOIN" }, codes.InvalidArgument},
		"empty key":             {func(i *CheckoutInput) { i.IdempotencyKey = "" }, codes.InvalidArgument},
		"key with spaces":       {func(i *CheckoutInput) { i.IdempotencyKey = "a b" }, codes.InvalidArgument},
		"key too long":          {func(i *CheckoutInput) { i.IdempotencyKey = string(make([]byte, 65)) }, codes.InvalidArgument},
		"no address":            {func(i *CheckoutInput) { i.Address = nil }, codes.InvalidArgument},
		"address without phone": {func(i *CheckoutInput) { i.Address = map[string]any{"receiver_name": "A", "line1": "x", "city": "y"} }, codes.InvalidArgument},
		"no items":              {func(i *CheckoutInput) { i.Items = nil }, codes.InvalidArgument},
		"zero quantity":         {func(i *CheckoutInput) { i.Items = []Line{{1, 0}} }, codes.InvalidArgument},
		"huge quantity":         {func(i *CheckoutInput) { i.Items = []Line{{1, 1 << 40}} }, codes.InvalidArgument},
		"product 0":             {func(i *CheckoutInput) { i.Items = []Line{{0, 1}} }, codes.InvalidArgument},
		"unknown product":       {func(i *CheckoutInput) { i.Items = []Line{{99, 1}} }, codes.FailedPrecondition},
		"hidden product":        {func(i *CheckoutInput) { i.Items = []Line{{2, 1}} }, codes.FailedPrecondition},
		"out of stock":          {func(i *CheckoutInput) { i.Items = []Line{{3, 1}} }, codes.FailedPrecondition},
		"not enough stock":      {func(i *CheckoutInput) { i.Items = []Line{{1, 6}} }, codes.FailedPrecondition},
		"long note":             {func(i *CheckoutInput) { i.Note = string(make([]rune, 501)) + "x" }, codes.InvalidArgument},
		"cart is empty":         {func(i *CheckoutInput) { i.FromCart, i.Items = true, nil }, codes.InvalidArgument},
	}
	for name, c := range cases {
		in := base()
		c.mutate(in)
		if _, err := s.Checkout(ctx, in); status.Code(err) != c.code {
			t.Errorf("%s: want %v, got %v", name, c.code, err)
		}
	}
	var n int64
	db.Model(&model.Order{}).Count(&n)
	if n != 0 {
		t.Errorf("failed checkouts must not create orders, found %d", n)
	}
	// catalog down -> Unavailable, nothing written
	s2, fake := newTestService(repository.NewOrderRepository(db), product(1, 10, 1000, 5))
	fake.err = status.Error(codes.Unavailable, "down")
	if _, err := s2.Checkout(ctx, base()); status.Code(err) != codes.Unavailable {
		t.Errorf("catalog down: %v", err)
	}
	// 51 different products
	var many []Line
	for i := uint64(1); i <= 51; i++ {
		many = append(many, Line{i, 1})
	}
	in := base()
	in.Items = many
	if _, err := s.Checkout(ctx, in); status.Code(err) != codes.InvalidArgument {
		t.Errorf("too many lines: %v", err)
	}
}

func TestPreviewFlagsProblemsWithoutFailing(t *testing.T) {
	db := testDB(t)
	hidden := product(2, 10, 1000, 5)
	hidden.Status = "draft"
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 5), hidden)
	pv, err := s.PreviewCheckout(context.Background(), 5, false, []Line{{1, 2}, {2, 1}, {99, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if pv.CanOrder {
		t.Error("a preview with unavailable lines must say can_order=false")
	}
	issues := map[uint64]string{}
	for _, g := range pv.Groups {
		for _, l := range g.Lines {
			issues[l.ProductID] = l.Issue
		}
	}
	if issues[1] != "" || issues[2] != "product is not available" || issues[99] != "product not found" {
		t.Errorf("issues: %v", issues)
	}
	// only available lines are priced; the total equals what checkout would charge
	if pv.Subtotal != 200000 || pv.ShippingFee != 30000 || pv.GrandTotal != 230000 {
		t.Errorf("totals: %v %v %v", pv.Subtotal, pv.ShippingFee, pv.GrandTotal)
	}
	if len(pv.Groups) != 2 || pv.Groups[0].StoreID != 0 {
		t.Errorf("unknown products form their own store-0 group: %+v", pv.Groups)
	}
}

func TestShippingRules(t *testing.T) {
	s := &OrderService{Settings: testSettings()}
	for subtotal, want := range map[int64]int64{1: 3000000, 49999999: 3000000, 50000000: 0, 90000000: 0} {
		if got := s.shippingFeeMinor(subtotal); got != want {
			t.Errorf("shipping(%d) = %d, want %d", subtotal, got, want)
		}
	}
	s.Settings.FreeShipOverMinor = 0 // never free
	if got := s.shippingFeeMinor(1 << 40); got != 3000000 {
		t.Errorf("free shipping disabled: %d", got)
	}
}
