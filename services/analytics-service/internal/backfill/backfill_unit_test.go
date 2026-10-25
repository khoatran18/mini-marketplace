package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"analytics-service/internal/ingest"
	"analytics-service/internal/store"
)

type capture struct {
	rows []struct {
		table string
		row   any
	}
}

func (c *capture) Add(table string, row any) {
	c.rows = append(c.rows, struct {
		table string
		row   any
	}{table, row})
}

// The payloads built from Postgres rows must produce exactly the rows the Kafka consumers produce.
func TestFeedBuildsConsumerRows(t *testing.T) {
	ctx := context.Background()
	cap := &capture{}
	c := &ingest.Consumers{Out: cap}
	at := time.Date(2026, 10, 1, 16, 50, 0, 0, time.UTC)

	if err := feed(ctx, c.HandleOrderStatus, statusChangedEvent{V: 1, OrderID: 9, CheckoutID: "c9", BuyerID: 3, StoreID: 7, Status: "PAID",
		PaymentMethod: "CARD", PaymentStatus: "PAID", SubtotalMinor: 100000000, ShippingFeeMinor: 3000000, TotalMinor: 103000000, At: at,
		Items: []itemEvent{{ItemID: 1, ProductID: 5, CategoryID: 2, Quantity: 2, UnitPriceMinor: 30000000}}}); err != nil {
		t.Fatal(err)
	}
	if len(cap.rows) != 2 {
		t.Fatalf("want status + item row, got %d", len(cap.rows))
	}
	st := cap.rows[0].row.(store.OrderStatusRow)
	if cap.rows[0].table != "fact_order_status" || st.SubtotalMinor != 100000000 || st.At != "2026-10-01 16:50:00.000" || st.Status != "PAID" {
		t.Fatalf("status row: %+v", st)
	}
	it := cap.rows[1].row.(store.OrderItemRow)
	if cap.rows[1].table != "fact_order_items" || it.Qty != 2 || it.UnitPriceMinor != 30000000 || it.StoreID != 7 || it.At != st.At {
		t.Fatalf("item row: %+v", it)
	}

	cap.rows = nil
	_ = feed(ctx, c.HandlePaymentFailed, paymentFailedEvent{PaymentID: 4, FailureCode: "card_declined", Terminal: true, At: at, Method: "CARD"})
	_ = feed(ctx, c.HandleProductChanged, productChangedEvent{ProductID: 5, Op: "delete", Price: 99999.99, Status: "active", UpdatedAt: at})
	_ = feed(ctx, c.HandleInventoryChanged, inventoryChangedEvent{ProductID: 5, Delta: -2, Available: 8, Level: stockLevel(8, 0), At: at})
	pe := cap.rows[0].row.(store.PaymentEventRow)
	if pe.Kind != "failed" || pe.Terminal != 1 || pe.AmountMinor != 0 {
		t.Fatalf("payment row: %+v", pe)
	}
	pr := cap.rows[1].row.(store.ProductRow)
	if pr.Status != "deleted" || pr.PriceMinor != 9999999 {
		t.Fatalf("product row: %+v", pr)
	}
	if inv := cap.rows[2].row.(store.InventoryRow); inv.Level != "ok" || inv.Available != 8 {
		t.Fatalf("inventory row: %+v", inv)
	}
	// malformed / empty payloads are skipped like in the consumer
	cap.rows = nil
	_ = c.HandleOrderStatus(ctx, &kafka.Message{Value: []byte("{")})
	if len(cap.rows) != 0 {
		t.Fatal("malformed payload must be skipped")
	}
}

func TestStockLevel(t *testing.T) {
	for _, c := range []struct {
		avail, thr int64
		want       string
	}{{0, 0, "none"}, {-1, 3, "none"}, {5, 0, "low"}, {6, 0, "ok"}, {3, 3, "low"}, {4, 3, "ok"}} {
		if got := stockLevel(c.avail, c.thr); got != c.want {
			t.Errorf("stockLevel(%d,%d)=%s want %s", c.avail, c.thr, got, c.want)
		}
	}
}

func TestParseDays(t *testing.T) {
	r, err := ParseDays("2026-10-01", "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-10-01 00:00 +07 = 2026-09-30 17:00 UTC; the end is exclusive, after the whole of 10-02
	if r.From != time.Date(2026, 9, 30, 17, 0, 0, 0, time.UTC) || r.To != time.Date(2026, 10, 2, 17, 0, 0, 0, time.UTC) {
		t.Fatalf("range %v", r)
	}
	for _, bad := range [][2]string{{"x", "2026-10-01"}, {"2026-10-01", "y"}, {"2026-10-02", "2026-10-01"}} {
		if _, err := ParseDays(bad[0], bad[1]); err == nil {
			t.Errorf("ParseDays(%v) should fail", bad)
		}
	}
}

func TestParseTables(t *testing.T) {
	if got, _ := ParseTables(""); len(got) != len(AllTables) {
		t.Fatal("empty = all")
	}
	got, err := ParseTables("refunds, payments,refunds")
	if err != nil || len(got) != 2 || got[0] != "refunds" || got[1] != "payments" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ParseTables("orders"); err == nil {
		t.Fatal("unknown table must fail")
	}
}

func TestMkLine(t *testing.T) {
	if l := mkLine("d", "m", 100, 100, 0); l.Mismatch || l.Ratio != 0 {
		t.Fatal("equal must match")
	}
	if l := mkLine("d", "m", 100, 101, 0); !l.Mismatch {
		t.Fatal("tolerance 0 is exact")
	}
	if l := mkLine("d", "m", 100, 101, 0.02); l.Mismatch || l.Diff != 1 {
		t.Fatal("within tolerance")
	}
	if l := mkLine("d", "m", 0, 5, 0.5); !l.Mismatch {
		t.Fatal("zero baseline uses max(|pg|,1)")
	}
}
