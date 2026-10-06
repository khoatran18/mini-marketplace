package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"analytics-service/internal/ch"
	"analytics-service/internal/store"
)

func TestQueryWithClickHouseAndCache(t *testing.T) {
	url := os.Getenv("TEST_CLICKHOUSE_URL")
	if url == "" {
		t.Skip("TEST_CLICKHOUSE_URL not set")
	}
	name := fmt.Sprintf("svc_%d_%d", time.Now().UnixNano()%1e9, rand.Intn(1e6))
	root := ch.New(url, "", "", "")
	if err := store.Migrate(context.Background(), root, name, 13); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name, nil)
	db := root.WithDatabase(name)
	now := time.Date(2026, 10, 6, 5, 0, 0, 0, time.UTC)
	at := store.Time(now.Add(-time.Hour))
	_ = db.Insert(context.Background(), "fact_order_status", []any{
		store.OrderStatusRow{OrderID: 1, Status: "PAID", StoreID: 7, PaymentMethod: "MOCK_CARD", SubtotalMinor: 1000000, At: at},
		store.OrderStatusRow{OrderID: 2, Status: "PAID", StoreID: 8, PaymentMethod: "MOCK_CARD", SubtotalMinor: 5000000, At: at},
	})
	cache := &memCache{}
	s := &Service{Q: &store.Queries{DB: db}, Cache: cache, CacheTTL: time.Minute, Now: func() time.Time { return now }}
	ctx := context.Background()

	get := func(role string, storeID uint64) (map[string]any, *Result) {
		res, err := s.Query(ctx, Request{Report: "summary", Role: role, StoreID: storeID})
		if err != nil {
			t.Fatal(err)
		}
		var d struct {
			Current map[string]any `json:"current"`
		}
		if err := json.Unmarshal(res.Data, &d); err != nil {
			t.Fatal(err)
		}
		return d.Current, res
	}
	if cur, res := get("admin", 0); cur["revenue"].(float64) != 60000 || res.Cached || res.Source != "clickhouse" {
		t.Fatalf("%v %+v", cur, res)
	}
	if cur, res := get("admin", 0); cur["revenue"].(float64) != 60000 || !res.Cached {
		t.Fatalf("second call must come from the cache: %v %+v", cur, res)
	}
	// the cache is keyed by scope: store 7 must never receive the platform (or another store's) numbers
	if cur, res := get("seller", 7); cur["revenue"].(float64) != 10000 || res.Cached {
		t.Fatalf("store 7: %v %+v", cur, res)
	}
	if cur, _ := get("seller", 8); cur["revenue"].(float64) != 50000 {
		t.Fatalf("store 8: %v", cur)
	}
	if res, err := s.Query(ctx, Request{Report: "data_health", Role: "admin"}); err != nil || res.Cached {
		t.Fatalf("data health is never cached: %v %+v", err, res)
	}
}
