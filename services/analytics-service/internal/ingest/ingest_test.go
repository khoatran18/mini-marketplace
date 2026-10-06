package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"analytics-service/internal/store"
)

var now = time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)

func ok(t *testing.T, r Raw) store.EventRow {
	t.Helper()
	row, why := Normalize(r, now)
	if why != "" {
		t.Fatalf("rejected: %s", why)
	}
	return row
}

func TestNormalizeWhitelist(t *testing.T) {
	good := Raw{EventType: "product_view", AnonymousID: "a1", SessionID: "s1", AnalyticsConsent: true, ProductID: 7}
	if row := ok(t, good); row.EventID == "" || row.ProductID != 7 || row.AnalyticsConsent != 1 {
		t.Fatalf("%+v", row)
	}
	for _, typ := range []string{"order_placed", "payment_succeeded", "order_status_changed", "order_canceled", "payment_failed", "", "DROP"} {
		if _, why := Normalize(Raw{EventType: typ, AnalyticsConsent: true}, now); why == "" {
			t.Fatalf("%q must be rejected (server-side events come from the outbox)", typ)
		}
	}
	// identify is written by the gateway only and needs both ids
	if _, why := Normalize(Raw{EventType: "identify", AnonymousID: "a", UserID: 3}, now); why != "" {
		t.Fatal(why)
	}
	if _, why := Normalize(Raw{EventType: "identify", AnonymousID: "a"}, now); why == "" {
		t.Fatal("identify without user must fail")
	}
}

func TestNormalizeLimitsAndSanitising(t *testing.T) {
	if _, why := Normalize(Raw{EventType: "page_view", AnalyticsConsent: true, SessionID: strings.Repeat("x", 65)}, now); why == "" {
		t.Fatal("long ids")
	}
	if _, why := Normalize(Raw{EventType: "search", AnalyticsConsent: true, PropsJSON: strings.Repeat("a", 3000)}, now); why == "" {
		t.Fatal("big props")
	}
	if _, why := Normalize(Raw{EventType: "search", AnalyticsConsent: true, PropsJSON: `[1,2]`}, now); why == "" {
		t.Fatal("props must be an object")
	}
	row := ok(t, Raw{EventType: "page_view", AnalyticsConsent: true, Path: "/search?q=a@b.com&token=zzz#x", Referrer: "https://google.com/search?q=secret", Surface: "evil", DeviceType: "toaster", Position: 999999})
	if row.Path != "/search" || row.Referrer != "google.com/search" || row.Surface != "" || row.DeviceType != "other" || row.Position != 65535 {
		t.Fatalf("%+v", row)
	}
	// PII in search queries is masked
	row = ok(t, Raw{EventType: "search", AnalyticsConsent: true, PropsJSON: `{"q":"mail me bob@example.com or call +84 912 345 678","result_count":0}`})
	var p map[string]any
	_ = json.Unmarshal([]byte(row.Props), &p)
	if q := p["q"].(string); strings.Contains(q, "@") || strings.Contains(q, "912") || !strings.Contains(q, "[email]") || !strings.Contains(q, "[phone]") {
		t.Fatalf("PII not scrubbed: %q", q)
	}
}

func TestNormalizeClock(t *testing.T) {
	good := ok(t, Raw{EventType: "page_view", AnalyticsConsent: true, TsClient: "2026-10-06T07:59:00Z"})
	if good.Ts != store.Time(now.Add(-time.Minute)) {
		t.Fatalf("client clock within 24h is kept: %s", good.Ts)
	}
	for _, c := range []string{"2020-01-01T00:00:00Z", "2099-01-01T00:00:00Z", "garbage"} {
		if row := ok(t, Raw{EventType: "page_view", AnalyticsConsent: true, TsClient: c}); row.Ts != store.Time(now) {
			t.Fatalf("%s: a skewed client clock falls back to server time, got %s", c, row.Ts)
		}
	}
}

func TestNormalizeConsent(t *testing.T) {
	// no consent: only anonymous page_view / error_client, all identifiers dropped
	row := ok(t, Raw{EventType: "page_view", AnonymousID: "a", SessionID: "s", UserID: 5, IPHash: "h", ProductID: 9, PropsJSON: `{"x":1}`})
	if row.AnonymousID != "" || row.SessionID != "" || row.UserID != 0 || row.IPHash != "" || row.ProductID != 0 || row.Props != "{}" || row.AnalyticsConsent != 0 {
		t.Fatalf("identifiers must be dropped: %+v", row)
	}
	if _, why := Normalize(Raw{EventType: "product_click", AnonymousID: "a"}, now); why != "no_consent" {
		t.Fatalf("got %q", why)
	}
}

type memSink struct{ rows map[string][]any }

func (m *memSink) Add(table string, row any) {
	if m.rows == nil {
		m.rows = map[string][]any{}
	}
	m.rows[table] = append(m.rows[table], row)
}

func msg(v any) *kafka.Message {
	b, _ := json.Marshal(v)
	return &kafka.Message{Value: b}
}

func TestConsumersMapEvents(t *testing.T) {
	out := &memSink{}
	c := &Consumers{Out: out}
	ctx := context.Background()
	at := now.Format(time.RFC3339Nano)

	_ = c.HandleOrderStatus(ctx, msg(map[string]any{"order_id": 5, "status": "PAID", "store_id": 3, "checkout_id": "c1", "buyer_id": 8,
		"payment_method": "MOCK_CARD", "subtotal_minor": 1000, "total_minor": 1300, "at": at,
		"items": []map[string]any{{"item_id": 1, "product_id": 9, "category_id": 2, "quantity": 2, "unit_price_minor": 500}}}))
	st := out.rows["fact_order_status"][0].(store.OrderStatusRow)
	if st.OrderID != 5 || st.Status != "PAID" || st.StoreID != 3 || st.TotalMinor != 1300 || st.At != store.Time(now) {
		t.Fatalf("%+v", st)
	}
	if it := out.rows["fact_order_items"][0].(store.OrderItemRow); it.ProductID != 9 || it.Qty != 2 || it.StoreID != 3 || it.UnitPriceMinor != 500 {
		t.Fatalf("%+v", it)
	}
	_ = c.HandlePaymentSucceeded(ctx, msg(map[string]any{"payment_id": 1, "method": "MOCK_CARD", "amount_minor": 1300, "at": at}))
	_ = c.HandlePaymentFailed(ctx, msg(map[string]any{"payment_id": 2, "failure_code": "card_declined", "terminal": true, "at": at}))
	if p := out.rows["fact_payment_events"]; len(p) != 2 || p[0].(store.PaymentEventRow).Kind != "succeeded" || p[1].(store.PaymentEventRow).FailureCode != "card_declined" || p[1].(store.PaymentEventRow).Terminal != 1 {
		t.Fatalf("%+v", p)
	}
	_ = c.HandlePaymentRefunded(ctx, msg(map[string]any{"payment_id": 1, "refund_id": 4, "order_id": 5, "amount_minor": 100, "at": at}))
	_ = c.HandleProductChanged(ctx, msg(map[string]any{"product_id": 9, "op": "upsert", "store_id": 3, "name": "N", "price": 12.5, "status": "active", "updated_at": at}))
	_ = c.HandleProductChanged(ctx, msg(map[string]any{"product_id": 10, "op": "delete", "store_id": 3, "updated_at": at}))
	pr := out.rows["dim_products"]
	if pr[0].(store.ProductRow).PriceMinor != 1250 || pr[1].(store.ProductRow).Status != "deleted" {
		t.Fatalf("%+v", pr)
	}
	_ = c.HandleInventoryChanged(ctx, msg(map[string]any{"product_id": 9, "store_id": 3, "delta": -2, "available": 8, "level": "low", "reason": "sale", "at": at}))
	if inv := out.rows["fact_inventory"][0].(store.InventoryRow); inv.Available != 8 || inv.Level != "low" {
		t.Fatalf("%+v", inv)
	}
	// malformed or incomplete messages are skipped without an error (a retry cannot fix them)
	before := len(out.rows["fact_order_status"])
	for _, h := range c.Topics() {
		if err := h(ctx, &kafka.Message{Value: []byte("{not json")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.HandleOrderStatus(ctx, msg(map[string]any{"status": "PAID"})); err != nil || len(out.rows["fact_order_status"]) != before {
		t.Fatal("order without id must be skipped")
	}
	if len(c.Topics()) != 6 {
		t.Fatal("topics")
	}
}
