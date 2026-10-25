package backfill

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"analytics-service/internal/ch"
	"analytics-service/internal/store"
)

// Integration tests need Postgres + ClickHouse (see CLAUDE.md):
//
//	TEST_POSTGRES_DSN="host=localhost port=5432 user=postgres password=postgres sslmode=disable"
//	TEST_CLICKHOUSE_URL=http://127.0.0.1:8123  [TEST_CLICKHOUSE_USER / TEST_CLICKHOUSE_PASSWORD]
//	SERVICES_DIR=<dir holding order-service/ payment-service/ product-service/>  (default ../../.. )
//
// The real goose migrations of the other services are applied to a throw-away database.

func gooseUp(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Skipf("migration not readable (%v); set SERVICES_DIR", err)
	}
	s := string(b)
	if i := strings.Index(s, "-- +goose Down"); i >= 0 {
		s = s[:i]
	}
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "-- +goose") {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

func newPG(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	dir := os.Getenv("SERVICES_DIR")
	if dir == "" {
		dir = "../../.."
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("bf_%d_%d", time.Now().UnixNano()%1e9, rand.Intn(1e6))
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); admin.Close() })
	db, err := sql.Open("pgx", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, svc := range []string{"order-service", "payment-service", "product-service"} {
		ddl := gooseUp(t, filepath.Join(dir, svc, "migrations", "00001_baseline.sql"))
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("%s migration: %v", svc, err)
		}
	}
	return db
}

func newCH(t *testing.T) (*ch.Client, *store.Sink) {
	t.Helper()
	url := os.Getenv("TEST_CLICKHOUSE_URL")
	if url == "" {
		t.Skip("TEST_CLICKHOUSE_URL not set")
	}
	name := fmt.Sprintf("bf_%d_%d", time.Now().UnixNano()%1e9, rand.Intn(1e6))
	root := ch.New(url, "", os.Getenv("TEST_CLICKHOUSE_USER"), os.Getenv("TEST_CLICKHOUSE_PASSWORD"))
	if err := store.Migrate(context.Background(), root, name, 13); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name, nil) })
	db := root.WithDatabase(name)
	return db, store.NewSink(db, time.Hour, 5000, 100000)
}

// seedPG inserts a small business history over 2026-10-01 and 2026-10-02 (Asia/Ho_Chi_Minh, +07).
func seedPG(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		// O1 online 1,000,000 VND: PAID at 23:50 on day 1, shipped on day 2
		`INSERT INTO orders (id,buyer_id,status,total_price,created_at,updated_at,checkout_id,store_id,payment_method,payment_status,subtotal,shipping_fee)
		 VALUES (1,3,'SHIPPED',1030000,'2026-10-01 09:00+07','2026-10-02 08:00+07','c1',7,'CARD','PAID',1000000,30000),
		        (2,4,'DELIVERED',530000,'2026-10-01 10:00+07','2026-10-02 00:10+07','c2',7,'COD','UNPAID',500000,30000),
		        (3,5,'CANCELED',130000,'2026-10-02 11:00+07','2026-10-02 11:05+07','c3',8,'CARD','UNPAID',100000,30000),
		        (4,6,'PAID',129999.99,'2026-10-02 12:00+07','2026-10-02 12:01+07','c4',8,'CARD','PAID',99999.99,30000)`,
		`INSERT INTO order_items (order_id,product_id,quantity,price,store_id,category_id,line_total) VALUES
		 (1,1,2,300000,7,10,600000),(1,2,1,400000,7,10,400000),(2,1,5,100000,7,10,500000),(3,3,1,100000,8,11,100000),(4,3,3,33333.33,8,11,99999.99)`,
		`INSERT INTO order_status_histories (order_id,from_status,to_status,actor_type,at) VALUES
		 (1,'','PENDING','buyer','2026-10-01 09:00+07'),(1,'PENDING','AWAITING_PAYMENT','system','2026-10-01 09:00:05+07'),
		 (1,'AWAITING_PAYMENT','PAID','system','2026-10-01 23:50+07'),(1,'PAID','SHIPPED','seller','2026-10-02 08:00+07'),
		 (2,'','PENDING','buyer','2026-10-01 10:00+07'),(2,'PENDING','CONFIRMED','seller','2026-10-01 10:05+07'),
		 (2,'CONFIRMED','SHIPPED','seller','2026-10-01 18:00+07'),(2,'SHIPPED','DELIVERED','system','2026-10-02 00:10+07'),
		 (3,'','PENDING','buyer','2026-10-02 11:00+07'),(3,'PENDING','CANCELED','buyer','2026-10-02 11:05+07'),
		 (4,'','PENDING','buyer','2026-10-02 12:00+07'),(4,'PENDING','PAID','system','2026-10-02 12:01+07')`,
		`INSERT INTO payments (id,checkout_id,buyer_id,method,status,amount_minor,expires_at,paid_at,refunded_minor) VALUES
		 (1,'c1',3,'CARD','PARTIALLY_REFUNDED',103000000,'2026-10-02 00:00+07','2026-10-01 23:50+07',2000000),
		 (2,'c4',6,'CARD','SUCCEEDED',13000000,'2026-10-03 00:00+07','2026-10-02 12:01+07',0),
		 (3,'c3',5,'CARD','FAILED',13000000,'2026-10-03 00:00+07',NULL,0)`,
		`INSERT INTO attempts (payment_id,scenario,status,failure_code,at) VALUES
		 (3,'card_declined','FAILED','card_declined','2026-10-02 11:01+07'),(3,'card_declined','FAILED','card_declined','2026-10-02 11:02+07'),
		 (3,'card_declined','FAILED','insufficient_funds','2026-10-02 11:03+07'),(3,'card_declined','FAILED','card_declined','2026-10-02 11:04+07'),
		 (3,'card_declined','FAILED','card_declined','2026-10-02 11:05+07'),(1,'success','SUCCEEDED','','2026-10-01 23:50+07')`,
		`INSERT INTO refunds (payment_id,order_id,refund_key,amount_minor,reason,status,at) VALUES
		 (1,1,'k1',2000000,'return','SUCCEEDED','2026-10-02 09:00+07'),(1,1,'k2',999,'too_much','FAILED','2026-10-02 09:30+07')`,
		`INSERT INTO products (id,name,price,seller_id,inventory,attributes,sku,category_id,brand,status,created_at,updated_at,deleted_at) VALUES
		 (1,'Áo thun',250000.50,7,8,'{}','SKU1',10,'Acme','active','2026-10-01 08:00+07','2026-10-01 08:00+07',NULL),
		 (2,'Quần',400000,7,3,'{}','SKU2',10,'','hidden','2026-10-01 08:30+07','2026-10-01 08:30+07','2026-10-02 07:00+07'),
		 (3,'Mũ',100000,8,0,'{}','',11,'','active','2026-09-20 08:00+07','2026-10-02 10:00+07',NULL)`,
		`INSERT INTO inventory_ledgers (product_id,delta,reason,ref_type,ref_id,balance_after,at) VALUES
		 (1,10,'initial','product','1',10,'2026-10-01 08:00+07'),(1,-2,'reserve','order','1',8,'2026-10-01 09:00+07'),
		 (2,3,'initial','product','2',3,'2026-10-01 08:30+07'),(3,0,'update','product','3',0,'2026-10-02 10:00+07')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
}

func chCount(t *testing.T, db *ch.Client, table string) int64 {
	t.Helper()
	rows, err := db.Query(context.Background(), "SELECT count() AS n FROM "+table+" FINAL", nil)
	if err != nil {
		t.Fatal(err)
	}
	return rows[0].Int("n")
}

func find(rep *Report, metric string) Line {
	for _, l := range rep.Totals {
		if l.Metric == metric {
			return l
		}
	}
	return Line{Metric: "NOT FOUND " + metric}
}

func mismatchDays(rep *Report) map[string]bool {
	m := map[string]bool{}
	for _, l := range rep.Lines {
		if l.Mismatch {
			m[l.Day+" "+l.Metric] = true
		}
	}
	return m
}

func TestBackfillReconcileRoundTrip(t *testing.T) {
	ctx := context.Background()
	pg := newPG(t)
	cdb, sink := newCH(t)
	seedPG(t, pg)
	r, err := ParseDays("2026-10-01", "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}

	// before any backfill ClickHouse is empty: reconcile must complain
	rep, err := Reconcile(ctx, pg, cdb, r, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.Mismatches == 0 {
		t.Fatal("empty ClickHouse must not reconcile")
	}

	res, err := Run(ctx, pg, sink, Options{Range: r, BatchRows: 3, Logf: t.Logf}) // tiny batch: exercise flushing
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"fact_order_status": 12, "fact_order_items": 5, "fact_payment_events": 7, "fact_refunds": 1, "dim_products": 3, "fact_inventory": 4}
	for tb, n := range want {
		if res.Rows[tb] != n {
			t.Errorf("backfilled %s = %d want %d", tb, res.Rows[tb], n)
		}
		if got := chCount(t, cdb, tb); got != n {
			t.Errorf("clickhouse %s = %d want %d", tb, got, n)
		}
	}

	rep, err = Reconcile(ctx, pg, cdb, r, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("reconcile after backfill: %+v", mismatchDays(rep))
	}
	// ADR-22: O1 PAID 1,000,000 + O2 COD DELIVERED 500,000 + O4 PAID 99,999.99 (minor = x100); refund 20,000 VND
	if l := find(rep, "revenue.subtotal_minor"); l.Postgres != 159999999 || l.ClickHouse != 159999999 {
		t.Errorf("revenue %+v", l)
	}
	if l := find(rep, "revenue.rows"); l.Postgres != 3 {
		t.Errorf("revenue orders %+v", l)
	}
	if l := find(rep, "net_revenue.subtotal_minor_minus_refunds"); l.Postgres != 157999999 || l.ClickHouse != 157999999 {
		t.Errorf("net %+v", l)
	}
	var d1, d2 int64
	for _, l := range rep.Lines {
		if l.Metric == "revenue.subtotal_minor" && l.Day == "2026-10-01" {
			d1 = l.Postgres
		}
		if l.Metric == "revenue.subtotal_minor" && l.Day == "2026-10-02" {
			d2 = l.Postgres
		}
	}
	if d1 != 100000000 || d2 != 59999999 {
		t.Errorf("revenue by day: %d %d (23:50 PAID is day 1; COD DELIVERED 00:10 is day 2)", d1, d2)
	}

	// idempotent: a second run (and a partial one) leaves the FINAL row counts unchanged
	if _, err := Run(ctx, pg, sink, Options{Range: r}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, pg, sink, Options{Range: r, Tables: []string{TRefunds, TPayments}}); err != nil {
		t.Fatal(err)
	}
	for tb, n := range want {
		if got := chCount(t, cdb, tb); got != n {
			t.Errorf("after re-run %s = %d want %d", tb, got, n)
		}
	}
	if rep, _ = Reconcile(ctx, pg, cdb, r, 0); !rep.OK {
		t.Fatalf("not ok after re-run: %+v", mismatchDays(rep))
	}

	// a sub-range / table subset
	cdb2, sink2 := newCH(t)
	r1, _ := ParseDays("2026-10-02", "2026-10-02")
	if _, err := Run(ctx, pg, sink2, Options{Range: r1, Tables: []string{TRefunds}}); err != nil {
		t.Fatal(err)
	}
	if got := chCount(t, cdb2, "fact_refunds"); got != 1 {
		t.Errorf("refunds subset = %d", got)
	}
	if got := chCount(t, cdb2, "fact_order_status"); got != 0 {
		t.Errorf("order_status must be untouched, got %d", got)
	}

	// tamper 1: delete the refund row -> the day of the refund (day 2) is reported
	if err := cdb.Exec(ctx, "ALTER TABLE fact_refunds DELETE WHERE 1 SETTINGS mutations_sync = 2", nil); err != nil {
		t.Fatal(err)
	}
	rep, _ = Reconcile(ctx, pg, cdb, r, 0)
	if rep.OK {
		t.Fatal("deleted refund must be detected")
	}
	m := mismatchDays(rep)
	for _, k := range []string{"2026-10-02 refunds.rows", "2026-10-02 refunds.amount_minor", "2026-10-02 net_revenue.subtotal_minor_minus_refunds"} {
		if !m[k] {
			t.Errorf("missing mismatch %q in %v", k, m)
		}
	}
	for k := range m {
		if strings.HasPrefix(k, "2026-10-01") {
			t.Errorf("day 1 must be clean, got %q", k)
		}
	}
	if rep, _ = Reconcile(ctx, pg, cdb, r, 1.0); !rep.OK {
		t.Errorf("ratio 1.0 tolerates a full loss of the refund: %v", mismatchDays(rep))
	}
	if _, err := Run(ctx, pg, sink, Options{Range: r, Tables: []string{TRefunds}}); err != nil {
		t.Fatal(err)
	}
	if rep, _ = Reconcile(ctx, pg, cdb, r, 0); !rep.OK {
		t.Fatalf("backfill must heal the deleted refund: %v", mismatchDays(rep))
	}

	// tamper 2: replacement rows (newer `at`, same key) with another subtotal for every status of order 1
	// (the revenue CTE reads the subtotal with any(), so all rows of the order must agree) -> day 1 revenue differs
	var tam []any
	for _, x := range []struct{ st, at string }{{"PENDING", "2026-10-01 02:00:00.001"}, {"AWAITING_PAYMENT", "2026-10-01 02:00:05.001"},
		{"PAID", "2026-10-01 16:50:00.001"}, {"SHIPPED", "2026-10-02 01:00:00.001"}} {
		tam = append(tam, store.OrderStatusRow{OrderID: 1, Status: x.st, CheckoutID: "c1", BuyerID: 3, StoreID: 7,
			PaymentMethod: "CARD", PaymentStatus: "PAID", SubtotalMinor: 100000001, TotalMinor: 103000000, At: x.at})
	}
	if err := cdb.Insert(ctx, "fact_order_status", tam); err != nil {
		t.Fatal(err)
	}
	rep, _ = Reconcile(ctx, pg, cdb, r, 0)
	m = mismatchDays(rep)
	if !m["2026-10-01 revenue.subtotal_minor"] || !m["2026-10-01 order_status.subtotal_minor"] || m["2026-10-01 revenue.rows"] {
		t.Errorf("tampered subtotal not reported on day 1 only: %v", m)
	}
	if m["2026-10-02 revenue.subtotal_minor"] {
		t.Errorf("day 2 revenue must stay clean: %v", m)
	}
	// tolerance: a 1-minor-unit difference on 100,000,000 is 1e-8
	if rep, _ = Reconcile(ctx, pg, cdb, r, 1e-6); !rep.OK {
		t.Errorf("1e-8 difference must pass tolerance 1e-6: %v", mismatchDays(rep))
	}
}

func TestBackfillOutOfRangeAndReadOnly(t *testing.T) {
	ctx := context.Background()
	pg := newPG(t)
	cdb, sink := newCH(t)
	seedPG(t, pg)
	r, _ := ParseDays("2026-09-01", "2026-09-30") // nothing happened then, except product 3 created 09-20 (updated later)
	res, err := Run(ctx, pg, sink, Options{Range: r})
	if err != nil {
		t.Fatal(err)
	}
	for tb, n := range res.Rows {
		if n != 0 {
			t.Errorf("%s: %d rows outside the range", tb, n)
		}
	}
	if got := chCount(t, cdb, "fact_order_status"); got != 0 {
		t.Errorf("got %d", got)
	}
	// the backfill transaction is read-only: writing through it must be impossible (smoke: DB unchanged)
	var n int
	if err := pg.QueryRow("SELECT count(*) FROM orders").Scan(&n); err != nil || n != 4 {
		t.Fatalf("orders %d %v", n, err)
	}
}
