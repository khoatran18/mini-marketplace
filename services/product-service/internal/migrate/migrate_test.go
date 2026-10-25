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

// testDB tạo DB riêng trên TEST_POSTGRES_DSN; bỏ qua khi chưa đặt biến môi trường.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("product_mig_%d", rand.Int63())
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
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func checkSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, tb := range []string{"products", "categories", "inventory_ledgers", "validate_order_events", "domain_events", VersionTable} {
		if count(t, db, "SELECT count(*) FROM information_schema.tables WHERE table_name='"+tb+"'") != 1 {
			t.Errorf("thiếu bảng %s", tb)
		}
	}
	if count(t, db, "SELECT count(*) FROM pg_indexes WHERE indexname='idx_products_search'") != 1 {
		t.Error("thiếu idx_products_search")
	}
	var s string
	if err := db.QueryRow("SELECT mm_unaccent('Điện Thoại Ở Đây')").Scan(&s); err != nil || s != "dien thoai o day" {
		t.Errorf("mm_unaccent = %q, %v", s, err)
	}
}

func TestRunEmptyDBAndIdempotent(t *testing.T) {
	db := testDB(t)
	for i := 0; i < 2; i++ {
		if err := Run(context.Background(), db); err != nil {
			t.Fatalf("run #%d: %v", i+1, err)
		}
	}
	checkSchema(t, db)
}

// DB cũ (AutoMigrate tạo sẵn bảng + dữ liệu, chưa có bảng version) vẫn lên được và giữ nguyên dữ liệu.
func TestRunOnLegacyDB(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec(`CREATE TABLE products (id bigserial PRIMARY KEY, name text NOT NULL, price numeric(18,2) NOT NULL,
		seller_id bigint NOT NULL, inventory bigint NOT NULL, attributes jsonb NOT NULL, sku text NOT NULL DEFAULT '',
		description text NOT NULL DEFAULT '', category_id bigint, brand text NOT NULL DEFAULT '', tags jsonb NOT NULL DEFAULT '[]',
		image_urls jsonb NOT NULL DEFAULT '[]', status text NOT NULL DEFAULT 'active', low_stock_threshold bigint NOT NULL DEFAULT 0,
		weight_g bigint NOT NULL DEFAULT 0, reserved bigint NOT NULL DEFAULT 0, sold bigint NOT NULL DEFAULT 0,
		version bigint NOT NULL DEFAULT 1, created_at timestamptz, updated_at timestamptz, deleted_at timestamptz);
		INSERT INTO products (name, price, seller_id, inventory, attributes) VALUES ('cũ', 1, 1, 1, '{}')`); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	checkSchema(t, db)
	if count(t, db, "SELECT count(*) FROM products") != 1 {
		t.Error("mất dữ liệu cũ")
	}
}
