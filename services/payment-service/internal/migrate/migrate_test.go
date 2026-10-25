package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func tempDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("payment_mig_%d", rand.Int63())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		admin.Close()
	})
	return db
}

func count(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunOnEmptyDBIsIdempotent(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := Run(ctx, db); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	for _, tbl := range []string{"payments", "attempts", "refunds", "webhook_deliveries", "processed_events", "domain_events"} {
		if n := count(t, db, "SELECT count(*) FROM information_schema.tables WHERE table_name='"+tbl+"'"); n != 1 {
			t.Errorf("thiếu bảng %s", tbl)
		}
	}
	if n := count(t, db, "SELECT count(*) FROM "+VersionTable); n != 3 { // bản ghi 0 + baseline + outbox_dead_letter
		t.Errorf("version rows = %d", n)
	}
}

func TestRunOnLegacyDBKeepsData(t *testing.T) {
	db := tempDB(t)
	// schema "cũ" tối giản như do AutoMigrate tạo, đã có dữ liệu
	if _, err := db.Exec(`CREATE TABLE payments (id BIGSERIAL PRIMARY KEY, checkout_id TEXT NOT NULL, buyer_id BIGINT NOT NULL, method TEXT NOT NULL, status TEXT NOT NULL, amount_minor BIGINT NOT NULL, currency TEXT NOT NULL DEFAULT 'VND', provider TEXT NOT NULL DEFAULT 'mockpay', provider_ref TEXT NOT NULL DEFAULT '', failure_code TEXT NOT NULL DEFAULT '', card_last4 TEXT NOT NULL DEFAULT '', failed_count BIGINT NOT NULL DEFAULT 0, three_ds_passed BOOLEAN NOT NULL DEFAULT false, order_ids JSONB NOT NULL DEFAULT '[]', expires_at TIMESTAMPTZ NOT NULL, paid_at TIMESTAMPTZ, refunded_minor BIGINT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ);
		CREATE UNIQUE INDEX idx_payments_checkout_id ON payments (checkout_id);
		INSERT INTO payments (checkout_id, buyer_id, method, status, amount_minor, expires_at) VALUES ('c1', 1, 'MOCK_CARD', 'SUCCEEDED', 100, now())`); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, "SELECT count(*) FROM payments"); n != 1 {
		t.Errorf("dữ liệu cũ bị mất: %d", n)
	}
	if n := count(t, db, "SELECT count(*) FROM domain_events"); n != 0 {
		t.Errorf("domain_events phải được tạo")
	}
}
