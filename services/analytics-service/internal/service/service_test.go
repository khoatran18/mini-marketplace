package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"analytics-service/internal/ingest"
	"analytics-service/internal/store"
)

type memCache struct {
	mu sync.Mutex
	m  map[string]string
}

func (c *memCache) Get(_ context.Context, k string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok
}
func (c *memCache) Set(_ context.Context, k, v string, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]string{}
	}
	c.m[k] = v
}

type rows struct{ n int }

func (r *rows) Add(string, any) { r.n++ }

func code(err error) codes.Code { return status.Code(err) }

// Access rules do not need a ClickHouse server: they are checked before any query runs.
func TestQueryAccessRules(t *testing.T) {
	s := &Service{Q: &store.Queries{}, Now: func() time.Time { return time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC) }}
	ctx := context.Background()
	cases := []struct {
		name string
		r    Request
		want codes.Code
	}{
		{"unknown report", Request{Report: "drop", Role: "admin"}, codes.InvalidArgument},
		{"buyer denied", Request{Report: "summary", Role: "buyer"}, codes.PermissionDenied},
		{"anonymous denied", Request{Report: "summary"}, codes.PermissionDenied},
		{"seller without store", Request{Report: "summary", Role: "seller"}, codes.InvalidArgument},
		{"seller cannot read traffic", Request{Report: "traffic", Role: "seller", StoreID: 4}, codes.PermissionDenied},
		{"seller cannot read payments", Request{Report: "payments", Role: "seller", StoreID: 4}, codes.PermissionDenied},
		{"seller cannot read data health", Request{Report: "data_health", Role: "seller", StoreID: 4}, codes.PermissionDenied},
		{"seller cannot read search terms", Request{Report: "search_terms", Role: "seller", StoreID: 4}, codes.PermissionDenied},
		{"bad granularity", Request{Report: "timeseries", Role: "admin", Granularity: "year"}, codes.InvalidArgument},
		{"bad sort", Request{Report: "top_products", Role: "admin", Sort: "x; DROP"}, codes.InvalidArgument},
		{"range too long", Request{Report: "summary", Role: "admin", From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, codes.InvalidArgument},
		{"inverted range", Request{Report: "summary", Role: "admin", From: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, codes.InvalidArgument},
	}
	for _, c := range cases {
		if _, err := s.Query(ctx, c.r); code(err) != c.want {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}
}

func TestIngestCountsAndReasons(t *testing.T) {
	out := &rows{}
	s := &Service{Sink: out, Now: time.Now}
	acc, reasons := s.Ingest([]ingest.Raw{
		{EventType: "page_view", AnalyticsConsent: true},
		{EventType: "order_placed", AnalyticsConsent: true},
		{EventType: "product_click"}, // no consent
	})
	if acc != 1 || out.n != 1 || len(reasons) != 2 || !strings.HasPrefix(reasons[0], "1:") || !strings.HasPrefix(reasons[1], "2:") {
		t.Fatalf("%d %v", acc, reasons)
	}
	if a, r := s.Counters(); a != 1 || r != 2 {
		t.Fatal(a, r)
	}
}
