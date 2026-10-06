package store

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"analytics-service/internal/ch"
)

// Tests need a ClickHouse HTTP endpoint (a real server or scripts/dev/chdb_server.py):
//
//	TEST_CLICKHOUSE_URL=http://127.0.0.1:8123 go test ./...
func newDB(t *testing.T) (*Queries, *ch.Client) {
	t.Helper()
	url := os.Getenv("TEST_CLICKHOUSE_URL")
	if url == "" {
		t.Skip("TEST_CLICKHOUSE_URL not set")
	}
	name := fmt.Sprintf("t_%d_%d", time.Now().UnixNano()%1e9, rand.Intn(1e6))
	root := ch.New(url, "", "", "")
	if err := Migrate(context.Background(), root, name, 13); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name, nil) })
	db := root.WithDatabase(name)
	return &Queries{DB: db}, db
}

var base = time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC) // 10:00 in Asia/Ho_Chi_Minh

func at(h int) string { return Time(base.Add(time.Duration(h) * time.Hour)) }

type seedOrder struct {
	id, store uint64
	pm        string
	subtotal  int64 // minor
	statuses  []string
	hours     []int
	items     []OrderItemRow
}

func seed(t *testing.T, db *ch.Client, orders []seedOrder) {
	t.Helper()
	var st, it []any
	for _, o := range orders {
		for i, s := range o.statuses {
			st = append(st, OrderStatusRow{OrderID: o.id, Status: s, StoreID: o.store, PaymentMethod: o.pm,
				SubtotalMinor: o.subtotal, TotalMinor: o.subtotal + 3000000, At: at(o.hours[i])})
		}
		for _, x := range o.items {
			x.OrderID, x.StoreID, x.At = o.id, o.store, at(o.hours[0])
			it = append(it, x)
		}
	}
	ctx := context.Background()
	if err := db.Insert(ctx, "fact_order_status", st); err != nil {
		t.Fatal(err)
	}
	if err := db.Insert(ctx, "fact_order_items", it); err != nil {
		t.Fatal(err)
	}
}

func rangeDay() Range { return Range{From: base.Add(-24 * time.Hour), To: base.Add(48 * time.Hour)} }

func TestMigrateIsIdempotent(t *testing.T) {
	_, db := newDB(t)
	root := ch.New(os.Getenv("TEST_CLICKHOUSE_URL"), "", "", "")
	name := db.Database
	if err := Migrate(context.Background(), root, name, 13); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(context.Background(), "SELECT count() AS n, max(version) AS v FROM schema_migrations", nil)
	if err != nil || rows[0].Int("n") != 7 || rows[0].Int("v") != 7 {
		t.Fatalf("migrations recorded once each: %v %v", rows, err)
	}
	if err := Migrate(context.Background(), root, "bad name;", 13); err == nil {
		t.Fatal("database name must be validated")
	}
}

func TestRevenueDefinition(t *testing.T) {
	q, db := newDB(t)
	seed(t, db, []seedOrder{
		// online order paid at +1h: revenue recognised at PAID, even though it is not delivered yet
		{id: 1, store: 10, pm: "MOCK_CARD", subtotal: 10000000, statuses: []string{"PENDING", "AWAITING_PAYMENT", "PAID"}, hours: []int{0, 0, 1},
			items: []OrderItemRow{{ItemID: 1, ProductID: 100, Qty: 2, UnitPriceMinor: 5000000}}},
		// COD: confirmed but not delivered => NOT revenue yet
		{id: 2, store: 10, pm: "COD", subtotal: 4000000, statuses: []string{"PENDING", "CONFIRMED"}, hours: []int{0, 0},
			items: []OrderItemRow{{ItemID: 2, ProductID: 101, Qty: 1, UnitPriceMinor: 4000000}}},
		// COD delivered => revenue at DELIVERED
		{id: 3, store: 11, pm: "COD", subtotal: 6000000, statuses: []string{"PENDING", "CONFIRMED", "SHIPPED", "DELIVERED"}, hours: []int{0, 0, 2, 5},
			items: []OrderItemRow{{ItemID: 3, ProductID: 200, Qty: 3, UnitPriceMinor: 2000000}}},
		// expired before payment: never revenue, not in GMV
		{id: 4, store: 10, pm: "MOCK_CARD", subtotal: 9900000, statuses: []string{"PENDING", "AWAITING_PAYMENT", "EXPIRED"}, hours: []int{0, 0, 3}},
		// canceled by the buyer after payment: stays revenue (refund subtracts it)
		{id: 5, store: 11, pm: "MOCK_WALLET", subtotal: 2000000, statuses: []string{"PENDING", "AWAITING_PAYMENT", "PAID", "REFUND_REQUESTED", "REFUNDED"}, hours: []int{0, 0, 1, 2, 3}},
	})
	if err := db.Insert(context.Background(), "fact_refunds", []any{RefundRow{RefundID: 1, PaymentID: 9, OrderID: 5, AmountMinor: 2000000, At: at(3)}}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	m, err := q.Summary(ctx, Scope{Platform: true}, rangeDay())
	if err != nil {
		t.Fatal(err)
	}
	cur := m["current"].(SummaryData)
	// recognised: order 1 (100000.00), order 3 (60000.00), order 5 (20000.00) = 180000.00 VND
	if cur.Revenue != 180000 || cur.OrdersRecognized != 3 {
		t.Fatalf("revenue = %v / %d orders", cur.Revenue, cur.OrdersRecognized)
	}
	if cur.Refunds != 20000 || cur.NetRevenue != 160000 {
		t.Fatalf("refunds %v net %v", cur.Refunds, cur.NetRevenue)
	}
	if cur.OrdersPlaced != 5 || cur.Expired != 1 || cur.Refunded != 1 {
		t.Fatalf("counts: %+v", cur)
	}
	// GMV: everything except the expired order 4 = 100000+40000+60000+20000
	if cur.GMV != 220000 {
		t.Fatalf("gmv = %v", cur.GMV)
	}
	if cur.AOV != 60000 {
		t.Fatalf("aov = %v", cur.AOV)
	}
	if cur.CancelRate != 0.2 {
		t.Fatalf("cancel rate = %v", cur.CancelRate)
	}

	// store 10 sees only its own numbers (order 1; order 2 is unpaid COD; order 4 expired)
	s10, _ := q.Summary(ctx, Scope{StoreID: 10}, rangeDay())
	c10 := s10["current"].(SummaryData)
	if c10.Revenue != 100000 || c10.OrdersRecognized != 1 || c10.OrdersPlaced != 3 {
		t.Fatalf("store 10: %+v", c10)
	}
	// ... and the refund of store 11's order does not touch store 10
	if c10.Refunds != 0 {
		t.Fatalf("store 10 refunds %v", c10.Refunds)
	}
	s11, _ := q.Summary(ctx, Scope{StoreID: 11}, rangeDay())
	if c := s11["current"].(SummaryData); c.Revenue != 80000 || c.Refunds != 20000 {
		t.Fatalf("store 11: %+v", c)
	}
	// a store with no orders sees zeros, not an error
	s99, err := q.Summary(ctx, Scope{StoreID: 99}, rangeDay())
	if err != nil || s99["current"].(SummaryData).Revenue != 0 {
		t.Fatalf("empty store: %v %v", s99, err)
	}

	// the previous period (the day before) is empty
	if m["previous"].(SummaryData).Revenue != 0 {
		t.Fatal("previous period must be empty")
	}
}

func TestIdempotentRedelivery(t *testing.T) {
	q, db := newDB(t)
	o := seedOrder{id: 1, store: 10, pm: "MOCK_CARD", subtotal: 1000000, statuses: []string{"PENDING", "PAID"}, hours: []int{0, 1},
		items: []OrderItemRow{{ItemID: 1, ProductID: 1, Qty: 1, UnitPriceMinor: 1000000}}}
	seed(t, db, []seedOrder{o})
	seed(t, db, []seedOrder{o}) // Kafka redelivery: same rows again
	m, err := q.Summary(context.Background(), Scope{Platform: true}, rangeDay())
	if err != nil {
		t.Fatal(err)
	}
	if c := m["current"].(SummaryData); c.Revenue != 10000 || c.OrdersPlaced != 1 {
		t.Fatalf("duplicate rows must not double count: %+v", c)
	}
}

func TestTimeseriesBucketsInLocalTime(t *testing.T) {
	q, db := newDB(t)
	// 2026-10-01 20:00 UTC is 2026-10-02 03:00 in Ho Chi Minh: a different local DAY than the UTC day
	late := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	var st []any
	st = append(st, OrderStatusRow{OrderID: 1, Status: "PENDING", StoreID: 10, PaymentMethod: "MOCK_CARD", SubtotalMinor: 100000, At: Time(late)})
	st = append(st, OrderStatusRow{OrderID: 1, Status: "PAID", StoreID: 10, PaymentMethod: "MOCK_CARD", SubtotalMinor: 100000, At: Time(late)})
	if err := db.Insert(context.Background(), "fact_order_status", st); err != nil {
		t.Fatal(err)
	}
	r := Range{From: time.Date(2026, 9, 30, 17, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 3, 17, 0, 0, 0, time.UTC)} // local 10-01..10-03
	pts, err := q.Timeseries(context.Background(), Scope{Platform: true}, r, "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 3 || pts[0].Bucket != "2026-10-01 00:00:00" || pts[2].Bucket != "2026-10-03 00:00:00" {
		t.Fatalf("buckets: %+v", pts)
	}
	if pts[0].Revenue != 0 || pts[1].Revenue != 1000 || pts[1].Orders != 1 {
		t.Fatalf("the order belongs to local day 10-02: %+v", pts)
	}
	for _, g := range []string{"hour", "week", "month"} {
		p, err := q.Timeseries(context.Background(), Scope{Platform: true}, r, g)
		if err != nil || len(p) == 0 {
			t.Fatalf("%s: %v %v", g, p, err)
		}
		var sum float64
		for _, x := range p {
			sum += x.Revenue
		}
		if sum != 1000 {
			t.Fatalf("%s buckets must hold all revenue exactly once (zero-fill must align with ClickHouse): %+v", g, p)
		}
	}
	if _, err := q.Timeseries(context.Background(), Scope{Platform: true}, r, "year"); err == nil {
		t.Fatal("bad granularity must be rejected")
	}
}

func TestTopProductsFunnelAndScope(t *testing.T) {
	q, db := newDB(t)
	ctx := context.Background()
	seed(t, db, []seedOrder{
		{id: 1, store: 10, pm: "MOCK_CARD", subtotal: 10000000, statuses: []string{"PENDING", "PAID"}, hours: []int{0, 1},
			items: []OrderItemRow{{ItemID: 1, ProductID: 100, Qty: 2, UnitPriceMinor: 5000000}}},
		{id: 2, store: 11, pm: "MOCK_CARD", subtotal: 3000000, statuses: []string{"PENDING", "PAID"}, hours: []int{0, 1},
			items: []OrderItemRow{{ItemID: 2, ProductID: 200, Qty: 1, UnitPriceMinor: 3000000}}},
	})
	_ = db.Insert(ctx, "dim_products", []any{
		ProductRow{ProductID: 100, StoreID: 10, Name: "Áo thun", Status: "active", UpdatedAt: at(0)},
		ProductRow{ProductID: 101, StoreID: 10, Name: "Quần", Status: "active", UpdatedAt: at(0)},
		ProductRow{ProductID: 200, StoreID: 11, Name: "Giày", Status: "active", UpdatedAt: at(0)},
	})
	ev := func(id, typ string, sess string, pid uint64) EventRow {
		return EventRow{EventID: id, EventType: typ, Ts: at(1), TsServer: at(1), AnonymousID: "a_" + sess, SessionID: sess, ProductID: pid, DeviceType: "mobile", AnalyticsConsent: 1, Props: "{}"}
	}
	_ = db.Insert(ctx, "events_raw", []any{
		ev("e1", "product_view", "s1", 100), ev("e2", "product_view", "s2", 100), ev("e3", "product_view", "s3", 101),
		ev("e4", "product_view", "s3", 101), // same session twice: counted once in the funnel, twice in views
		ev("e5", "add_to_cart", "s1", 100), ev("e6", "checkout_start", "s1", 0), ev("e7", "product_view", "s4", 200),
	})
	top, err := q.TopProducts(ctx, Scope{Platform: true}, rangeDay(), "revenue", 10)
	if err != nil || len(top) < 2 {
		t.Fatalf("%v %v", top, err)
	}
	if top[0].ProductID != 100 || top[0].Revenue != 100000 || top[0].Units != 2 || top[0].Views != 2 || top[0].Name != "Áo thun" {
		t.Fatalf("top[0] = %+v", top[0])
	}
	if top[0].Conversion != 1 { // 2 units / 2 views
		t.Fatalf("conversion %v", top[0].Conversion)
	}
	// store 10 never sees product 200 (store 11)
	mine, err := q.TopProducts(ctx, Scope{StoreID: 10}, rangeDay(), "revenue", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range mine {
		if p.ProductID == 200 {
			t.Fatalf("store 10 sees another store's product: %+v", mine)
		}
	}
	// viewed but never sold
	unsold, err := q.TopProducts(ctx, Scope{StoreID: 10}, rangeDay(), "viewed_unsold", 10)
	if err != nil || len(unsold) != 1 || unsold[0].ProductID != 101 || unsold[0].Views != 2 {
		t.Fatalf("unsold %+v %v", unsold, err)
	}
	if _, err := q.TopProducts(ctx, Scope{Platform: true}, rangeDay(), "price; DROP TABLE x", 10); err == nil {
		t.Fatal("sort must be whitelisted")
	}

	f, err := q.Funnel(ctx, Scope{Platform: true}, rangeDay())
	if err != nil {
		t.Fatal(err)
	}
	steps := f["steps"].([]Step)
	if *steps[0].Count != 4 || *steps[1].Count != 1 || *steps[2].Count != 1 || *steps[3].Count != 2 {
		t.Fatalf("platform funnel: %+v %d %d %d %d", steps, *steps[0].Count, *steps[1].Count, *steps[2].Count, *steps[3].Count)
	}
	fs, err := q.Funnel(ctx, Scope{StoreID: 10}, rangeDay())
	if err != nil {
		t.Fatal(err)
	}
	ss := fs["steps"].([]Step)
	if *ss[0].Count != 3 || ss[2].Count != nil || *ss[3].Count != 1 {
		t.Fatalf("store funnel: %+v", ss)
	}
	if _, has := fs["by_device"]; has {
		t.Fatal("store scope must not get the platform device breakdown")
	}
}

func TestPaymentsTrafficSearchAndHealth(t *testing.T) {
	q, db := newDB(t)
	ctx := context.Background()
	_ = db.Insert(ctx, "fact_payment_events", []any{
		PaymentEventRow{PaymentID: 1, Kind: "succeeded", Method: "MOCK_CARD", AmountMinor: 5000000, At: at(1)},
		PaymentEventRow{PaymentID: 2, Kind: "failed", Method: "MOCK_CARD", FailureCode: "card_declined", Terminal: 1, At: at(1)},
		PaymentEventRow{PaymentID: 3, Kind: "failed", Method: "MOCK_WALLET", FailureCode: "insufficient_funds", At: at(1)},
		PaymentEventRow{PaymentID: 3, Kind: "succeeded", Method: "MOCK_WALLET", AmountMinor: 1000000, At: at(2)},
	})
	seed(t, db, []seedOrder{{id: 1, store: 10, pm: "MOCK_CARD", subtotal: 100, statuses: []string{"PENDING", "AWAITING_PAYMENT"}, hours: []int{0, 0}}})
	p, err := q.Payments(ctx, rangeDay())
	if err != nil {
		t.Fatal(err)
	}
	if p["succeeded"].(int64) != 2 || p["failed"].(int64) != 2 || p["success_rate"].(float64) != 0.5 || p["orders_awaiting_payment"].(int64) != 1 {
		t.Fatalf("%+v", p)
	}
	if p["amount_succeeded"].(float64) != 60000 {
		t.Fatalf("%+v", p)
	}

	e := func(id, typ, anon, sess, path, ref, props string) EventRow {
		return EventRow{EventID: id, EventType: typ, Ts: at(1), TsServer: at(1), AnonymousID: anon, SessionID: sess, Path: path, Referrer: ref, DeviceType: "desktop", Country: "VN", Props: props, AnalyticsConsent: 1}
	}
	_ = db.Insert(ctx, "events_raw", []any{
		e("a", "page_view", "a1", "s1", "/", "google.com/", "{}"), e("b", "page_view", "a1", "s1", "/p/1", "", "{}"),
		e("c", "page_view", "a2", "s2", "/", "", "{}"), e("d", "error_client", "a2", "s2", "/", "", "{}"),
		e("s1", "search", "a1", "s1", "/search", "", `{"q":"Áo Thun","result_count":5}`),
		e("s2", "search", "a2", "s2", "/search", "", `{"q":"áo thun","result_count":3}`),
		e("s3", "search", "a2", "s2", "/search", "", `{"q":"xyz","result_count":0}`),
	})
	tr, err := q.Traffic(ctx, rangeDay(), "day")
	if err != nil {
		t.Fatal(err)
	}
	if tr["page_views"].(int64) != 3 || tr["sessions"].(int64) != 2 || tr["visitors"].(int64) != 2 || tr["client_errors"].(int64) != 1 {
		t.Fatalf("%+v", tr)
	}
	if tr["new_visitors"].(int64) != 2 || tr["returning_visitors"].(int64) != 0 {
		t.Fatalf("new/returning: %+v", tr)
	}
	st, err := q.SearchTerms(ctx, rangeDay(), 10)
	if err != nil {
		t.Fatal(err)
	}
	top := st["top"].([]map[string]any)
	if top[0]["term"] != "áo thun" || top[0]["count"].(int64) != 2 {
		t.Fatalf("terms are case-folded: %+v", top)
	}
	if z := st["zero_results"].([]map[string]any); len(z) != 1 || z[0]["term"] != "xyz" {
		t.Fatalf("zero: %+v", z)
	}
	h, err := q.DataHealth(ctx)
	if err != nil || len(h["tables"].([]map[string]any)) != len(Tables) {
		t.Fatalf("%v %v", h, err)
	}
}

func TestLowStock(t *testing.T) {
	q, db := newDB(t)
	ctx := context.Background()
	q.Now = func() time.Time { return base.Add(24 * time.Hour) }
	_ = db.Insert(ctx, "dim_products", []any{
		ProductRow{ProductID: 1, StoreID: 10, Name: "Sắp hết", Status: "active", UpdatedAt: at(0)},
		ProductRow{ProductID: 2, StoreID: 10, Name: "Dồi dào", Status: "active", UpdatedAt: at(0)},
		ProductRow{ProductID: 3, StoreID: 10, Name: "Hết hàng", Status: "active", UpdatedAt: at(0)},
		ProductRow{ProductID: 4, StoreID: 11, Name: "Của shop khác", Status: "active", UpdatedAt: at(0)},
	})
	_ = db.Insert(ctx, "fact_inventory", []any{
		InventoryRow{ProductID: 1, StoreID: 10, Available: 100, Level: "ok", Reason: "initial", At: at(0)},
		InventoryRow{ProductID: 1, StoreID: 10, Delta: -30, Available: 70, Level: "ok", Reason: "sale", At: at(5)}, // latest
		InventoryRow{ProductID: 2, StoreID: 10, Available: 1000, Level: "ok", Reason: "initial", At: at(0)},
		InventoryRow{ProductID: 3, StoreID: 10, Available: 0, Level: "none", Reason: "sale", At: at(0)},
		InventoryRow{ProductID: 4, StoreID: 11, Available: 0, Level: "none", Reason: "sale", At: at(0)},
	})
	// product 1 sold 140 units in the last 14 days = 10/day => 7 days left
	seed(t, db, []seedOrder{{id: 1, store: 10, pm: "COD", subtotal: 1, statuses: []string{"PENDING"}, hours: []int{1},
		items: []OrderItemRow{{ItemID: 1, ProductID: 1, Qty: 140, UnitPriceMinor: 1}}}})
	rows, err := q.LowStock(ctx, Scope{StoreID: 10}, 50)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range rows {
		names = append(names, r["name"].(string))
	}
	if len(rows) != 2 || names[0] != "Sắp hết" || names[1] != "Hết hàng" {
		t.Fatalf("store 10 low stock (soonest first, no other store): %v", names)
	}
	if rows[0]["days_left"].(float64) != 7 || rows[1]["days_left"] != nil {
		t.Fatalf("%+v", rows)
	}
	if all, _ := q.LowStock(ctx, Scope{Platform: true}, 50); len(all) != 3 {
		t.Fatalf("platform sees every store: %d", len(all))
	}
}

func TestScopeAndRangeValidation(t *testing.T) {
	if (Scope{}).Validate() == nil || (Scope{Platform: true}).Validate() != nil || (Scope{StoreID: 3}).Validate() != nil {
		t.Fatal("scope validation")
	}
	n := time.Now()
	if (Range{From: n, To: n}).Validate() == nil || (Range{From: n.Add(-500 * 24 * time.Hour), To: n}).Validate() == nil || (Range{From: n.Add(-24 * time.Hour), To: n}).Validate() != nil {
		t.Fatal("range validation")
	}
}
