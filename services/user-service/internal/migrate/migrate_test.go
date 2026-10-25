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

// newDB tạo DB riêng trên TEST_POSTGRES_DSN; skip nếu chưa set.
func newDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("user_mig_%d", rand.Int63())
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

func tableCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN
		('sellers','buyers','addresses','create_seller_events',$1)`, VersionTable).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunEmptyDBAndIdempotent(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := Run(ctx, db); err != nil {
			t.Fatalf("run #%d: %v", i+1, err)
		}
	}
	if n := tableCount(t, db); n != 5 {
		t.Errorf("tables = %d, want 5", n)
	}
	if _, err := db.Exec(`INSERT INTO addresses (user_id, receiver_name, phone, line1, city) VALUES (1,'a','b','c','d')`); err != nil {
		t.Errorf("insert address (serial default): %v", err)
	}
}

// DB cũ do AutoMigrate tạo (chưa có bảng version, đã có dữ liệu) vẫn lên được và giữ nguyên dữ liệu.
func TestRunLegacyDBWithData(t *testing.T) {
	db := newDB(t)
	_, err := db.Exec(`
CREATE TABLE addresses (id bigserial PRIMARY KEY, user_id bigint NOT NULL, label text NOT NULL DEFAULT '',
  receiver_name text NOT NULL, phone text NOT NULL, line1 text NOT NULL, ward text NOT NULL DEFAULT '',
  district text NOT NULL DEFAULT '', city text NOT NULL, is_default boolean NOT NULL DEFAULT false,
  created_at timestamptz, updated_at timestamptz);
CREATE INDEX idx_addresses_user_id ON addresses (user_id);
INSERT INTO addresses (user_id, receiver_name, phone, line1, city) VALUES (7,'x','y','z','w');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM addresses WHERE user_id=7`).Scan(&n); err != nil || n != 1 {
		t.Errorf("data lost: n=%d err=%v", n, err)
	}
	if c := tableCount(t, db); c != 5 {
		t.Errorf("tables = %d, want 5", c)
	}
}
