package backfill

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/kafka-go"

	"analytics-service/internal/ingest"
	"analytics-service/internal/store"
)

// Backfill table names (the --tables values).
const (
	TOrderStatus = "order_status"
	TOrderItems  = "order_items"
	TPayments    = "payments"
	TRefunds     = "refunds"
	TProducts    = "products"
	TInventory   = "inventory"
)

// AllTables lists every backfillable table in load order (products first so dim_products exists before facts).
var AllTables = []string{TProducts, TInventory, TOrderStatus, TOrderItems, TPayments, TRefunds}

// ParseTables validates a comma separated --tables value ("" = all).
func ParseTables(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return AllTables, nil
	}
	valid := map[string]bool{}
	for _, t := range AllTables {
		valid[t] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !valid[t] {
			return nil, fmt.Errorf("unknown table %q (valid: %s)", t, strings.Join(AllTables, ","))
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out, nil
}

// Options of one backfill run.
type Options struct {
	Range  Range
	Tables []string // nil = all
	// BatchRows is how many rows are buffered before one INSERT per ClickHouse table (default 5000).
	BatchRows int
	Logf      func(format string, args ...any)
}

// Result is what a run did: rows written per ClickHouse table.
type Result struct {
	Rows map[string]int64 `json:"rows"`
}

// batcher is the ingest.Adder of a run: it keeps only the wanted ClickHouse tables, counts rows and flushes the
// shared store.Sink in batches.
type batcher struct {
	ctx    context.Context
	sink   *store.Sink
	keep   map[string]bool
	every  int
	n      int
	counts map[string]int64
	err    error
}

func (b *batcher) Add(table string, row any) {
	if b.err != nil || (b.keep != nil && !b.keep[table]) {
		return
	}
	b.sink.Add(table, row)
	b.counts[table]++
	if b.n++; b.n >= b.every {
		b.n = 0
		b.err = b.sink.Flush(b.ctx)
	}
}

func (b *batcher) flush() error {
	if b.err == nil {
		b.err = b.sink.Flush(b.ctx)
	}
	b.n = 0
	return b.err
}

// Run reads Postgres (one READ ONLY transaction) and writes the rows through sink. Re-running it is safe: the
// ClickHouse tables are ReplacingMergeTree.
func Run(ctx context.Context, pg *sql.DB, sink *store.Sink, o Options) (*Result, error) {
	if o.BatchRows <= 0 {
		o.BatchRows = 5000
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	tables := o.Tables
	if len(tables) == 0 {
		tables = AllTables
	}
	tx, err := pg.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // read-only
	b := &batcher{ctx: ctx, sink: sink, every: o.BatchRows, counts: map[string]int64{}}
	c := &ingest.Consumers{Out: b}
	// load order matters only for readability; ClickHouse does not enforce references
	want := map[string]bool{}
	for _, t := range tables {
		want[t] = true
	}
	steps := []struct {
		name string
		keep []string // ClickHouse tables kept in this step ("" = all)
		fn   func(context.Context, *sql.Tx, *ingest.Consumers, Range, *batcher) error
	}{
		{TProducts, nil, loadProducts},
		{TInventory, nil, loadInventory},
		{TOrderStatus, []string{"fact_order_status"}, loadOrderStatus},
		{TOrderItems, []string{"fact_order_items"}, loadOrderItems},
		{TPayments, nil, loadPayments},
		{TRefunds, nil, loadRefunds},
	}
	for _, s := range steps {
		if !want[s.name] {
			continue
		}
		b.keep = nil
		if s.keep != nil {
			b.keep = map[string]bool{}
			for _, k := range s.keep {
				b.keep[k] = true
			}
		}
		before := sumCounts(b.counts)
		if err := s.fn(ctx, tx, c, o.Range, b); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		if err := b.flush(); err != nil {
			return nil, fmt.Errorf("%s: clickhouse insert: %w", s.name, err)
		}
		o.Logf("backfill %-13s %d rows", s.name, sumCounts(b.counts)-before)
	}
	return &Result{Rows: b.counts}, nil
}

func sumCounts(m map[string]int64) (n int64) {
	for _, v := range m {
		n += v
	}
	return n
}

// feed marshals a payload exactly like the producer's outbox does and hands it to a consumer handler.
func feed(ctx context.Context, h func(context.Context, *kafka.Message) error, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h(ctx, &kafka.Message{Value: raw})
}

func loadOrderStatus(ctx context.Context, tx *sql.Tx, c *ingest.Consumers, r Range, b *batcher) error {
	// One row per (order, status): the first time the order reached it. Payment status / money are the order's
	// CURRENT values (the history does not keep a snapshot of them).
	rows, err := tx.QueryContext(ctx, `
SELECT o.id, o.checkout_id, o.buyer_id, o.store_id, o.payment_method, o.payment_status,
       ROUND(o.subtotal*100)::bigint, ROUND(o.shipping_fee*100)::bigint, ROUND(o.total_price*100)::bigint, h.to_status, h.at
FROM (SELECT order_id, to_status, min(at) AS at FROM order_status_histories GROUP BY order_id, to_status) h
JOIN orders o ON o.id = h.order_id
WHERE h.at >= $1 AND h.at < $2
ORDER BY h.at, o.id, h.to_status`, r.From, r.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() && b.err == nil {
		var e statusChangedEvent
		if err := rows.Scan(&e.OrderID, &e.CheckoutID, &e.BuyerID, &e.StoreID, &e.PaymentMethod, &e.PaymentStatus,
			&e.SubtotalMinor, &e.ShippingFeeMinor, &e.TotalMinor, &e.Status, &e.At); err != nil {
			return err
		}
		e.V = 1
		if err := feed(ctx, c.HandleOrderStatus, e); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.err
}

func loadOrderItems(ctx context.Context, tx *sql.Tx, c *ingest.Consumers, r Range, b *batcher) error {
	// The streamed row of an item carries the `at` of the LAST status event of its order (every event lists all
	// items and the newest one wins), so the backfill uses the time of the order's last status change.
	rows, err := tx.QueryContext(ctx, `
SELECT o.id, o.checkout_id, o.buyer_id, o.store_id, o.payment_method, o.payment_status,
       ROUND(o.subtotal*100)::bigint, ROUND(o.shipping_fee*100)::bigint, ROUND(o.total_price*100)::bigint,
       i.id, i.product_id, i.category_id, i.quantity, ROUND(i.price*100)::bigint, x.at
FROM orders o
JOIN order_items i ON i.order_id = o.id
JOIN LATERAL (SELECT COALESCE((SELECT max(at) FROM order_status_histories WHERE order_id = o.id), o.updated_at, o.created_at) AS at) x ON true
WHERE x.at >= $1 AND x.at < $2
ORDER BY o.id, i.id`, r.From, r.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	var cur *statusChangedEvent
	emit := func() error {
		if cur == nil {
			return nil
		}
		err := feed(ctx, c.HandleOrderStatus, *cur)
		cur = nil
		return err
	}
	for rows.Next() && b.err == nil {
		var e statusChangedEvent
		var it itemEvent
		if err := rows.Scan(&e.OrderID, &e.CheckoutID, &e.BuyerID, &e.StoreID, &e.PaymentMethod, &e.PaymentStatus,
			&e.SubtotalMinor, &e.ShippingFeeMinor, &e.TotalMinor,
			&it.ItemID, &it.ProductID, &it.CategoryID, &it.Quantity, &it.UnitPriceMinor, &e.At); err != nil {
			return err
		}
		if cur != nil && cur.OrderID != e.OrderID {
			if err := emit(); err != nil {
				return err
			}
		}
		if cur == nil {
			e.V, e.Status = 1, "ITEMS" // the status row is discarded (table filter); only the item rows are kept
			cur = &e
		}
		cur.Items = append(cur.Items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := emit(); err != nil {
		return err
	}
	return b.err
}

func loadPayments(ctx context.Context, tx *sql.Tx, c *ingest.Consumers, r Range, b *batcher) error {
	// succeeded: one event per paid payment
	rows, err := tx.QueryContext(ctx, `
SELECT id, checkout_id, buyer_id, method, amount_minor, paid_at FROM payments
WHERE paid_at IS NOT NULL AND paid_at >= $1 AND paid_at < $2 ORDER BY paid_at, id`, r.From, r.To)
	if err != nil {
		return err
	}
	for rows.Next() && b.err == nil {
		var e paymentSucceededEvent
		if err := rows.Scan(&e.PaymentID, &e.CheckoutID, &e.BuyerID, &e.Method, &e.AmountMinor, &e.At); err != nil {
			rows.Close()
			return err
		}
		e.V = 1
		if err := feed(ctx, c.HandlePaymentSucceeded, e); err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	// failed: one event per FAILED attempt; the MaxFailedAttempts-th failure is the terminal one
	rows, err = tx.QueryContext(ctx, `
SELECT p.id, p.checkout_id, p.buyer_id, p.method, a.failure_code, a.rn >= $3, a.at
FROM (SELECT payment_id, failure_code, at, row_number() OVER (PARTITION BY payment_id ORDER BY at, id) AS rn
      FROM attempts WHERE status = 'FAILED') a
JOIN payments p ON p.id = a.payment_id
WHERE a.at >= $1 AND a.at < $2 ORDER BY a.at, p.id`, r.From, r.To, maxFailedAttempts)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() && b.err == nil {
		var e paymentFailedEvent
		if err := rows.Scan(&e.PaymentID, &e.CheckoutID, &e.BuyerID, &e.Method, &e.FailureCode, &e.Terminal, &e.At); err != nil {
			return err
		}
		e.V = 1
		if err := feed(ctx, c.HandlePaymentFailed, e); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.err
}

func loadRefunds(ctx context.Context, tx *sql.Tx, c *ingest.Consumers, r Range, b *batcher) error {
	rows, err := tx.QueryContext(ctx, `
SELECT f.id, f.payment_id, p.checkout_id, f.order_id, f.amount_minor, f.reason, f.at
FROM refunds f JOIN payments p ON p.id = f.payment_id
WHERE f.status = 'SUCCEEDED' AND f.at >= $1 AND f.at < $2 ORDER BY f.at, f.id`, r.From, r.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() && b.err == nil {
		var e paymentRefundedEvent
		if err := rows.Scan(&e.RefundID, &e.PaymentID, &e.CheckoutID, &e.OrderID, &e.AmountMinor, &e.Reason, &e.At); err != nil {
			return err
		}
		e.V = 1
		if err := feed(ctx, c.HandlePaymentRefunded, e); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.err
}

func loadProducts(ctx context.Context, tx *sql.Tx, c *ingest.Consumers, r Range, b *batcher) error {
	// A soft-deleted product becomes op=delete (status "deleted" in dim_products), dated by its deletion.
	rows, err := tx.QueryContext(ctx, `
SELECT id, seller_id, name, sku, COALESCE(category_id, 0), brand, price::float8, status, deleted_at IS NOT NULL, ts
FROM (SELECT p.*, COALESCE(deleted_at, updated_at, created_at) AS ts FROM products p) p
WHERE ts >= $1 AND ts < $2 ORDER BY ts, id`, r.From, r.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() && b.err == nil {
		var e productChangedEvent
		var deleted bool
		if err := rows.Scan(&e.ProductID, &e.StoreID, &e.Name, &e.SKU, &e.CategoryID, &e.Brand, &e.Price, &e.Status, &deleted, &e.UpdatedAt); err != nil {
			return err
		}
		e.V, e.Op = 1, "upsert"
		if deleted {
			e.Op = "delete"
		}
		if err := feed(ctx, c.HandleProductChanged, e); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.err
}

func loadInventory(ctx context.Context, tx *sql.Tx, c *ingest.Consumers, r Range, b *batcher) error {
	// The ledger has no "reserved" snapshot: reserved is written as 0 (documented limit).
	rows, err := tx.QueryContext(ctx, `
SELECT l.product_id, p.seller_id, l.delta, l.balance_after, p.low_stock_threshold, l.reason, l.ref_type, l.ref_id, l.at
FROM inventory_ledgers l JOIN products p ON p.id = l.product_id
WHERE l.at >= $1 AND l.at < $2 ORDER BY l.at, l.id`, r.From, r.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() && b.err == nil {
		var e inventoryChangedEvent
		var threshold int64
		if err := rows.Scan(&e.ProductID, &e.StoreID, &e.Delta, &e.Available, &threshold, &e.Reason, &e.RefType, &e.RefID, &e.At); err != nil {
			return err
		}
		e.V, e.Level = 1, stockLevel(e.Available, threshold)
		if err := feed(ctx, c.HandleInventoryChanged, e); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.err
}
