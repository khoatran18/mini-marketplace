// Package service implements the analytics use cases: ingesting tracking events and answering scoped reports.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"analytics-service/internal/ingest"
	"analytics-service/internal/store"
)

// Cache is an optional result cache (Redis). Errors are never fatal: reports are just recomputed.
type Cache interface {
	Get(ctx context.Context, key string) (string, bool)
	Set(ctx context.Context, key, value string, ttl time.Duration)
}

// Adder receives event rows.
type Adder interface{ Add(table string, row any) }

// Service answers analytics requests.
type Service struct {
	Q        *store.Queries
	Sink     Adder
	Cache    Cache
	CacheTTL time.Duration
	Now      func() time.Time

	accepted, rejected atomic.Int64
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Counters returns the number of tracking events accepted / rejected since start.
func (s *Service) Counters() (accepted, rejected int64) { return s.accepted.Load(), s.rejected.Load() }

// Ingest validates the events and queues the valid ones. It never fails the batch because of one bad event.
func (s *Service) Ingest(events []ingest.Raw) (accepted int, reasons []string) {
	now := s.now()
	for i, e := range events {
		row, why := ingest.Normalize(e, now)
		if why != "" {
			reasons = append(reasons, fmt.Sprintf("%d:%s", i, why))
			s.rejected.Add(1)
			continue
		}
		s.Sink.Add("events_raw", row)
		accepted++
		s.accepted.Add(1)
	}
	return accepted, reasons
}

// Request is one report request (already decoded from gRPC).
type Request struct {
	Report      string
	Role        string
	StoreID     uint64
	From, To    time.Time // zero = default (last 30 days)
	Granularity string
	Sort        string
	Limit       int
}

// Result is a computed report.
type Result struct {
	AsOf   time.Time
	Source string
	Cached bool
	Data   json.RawMessage
}

// reports and who may run them. "store" reports honour the scope; "platform" ones need role admin.
var reports = map[string]bool{ // value: platform only
	"summary": false, "timeseries": false, "top_products": false, "funnel": false, "low_stock": false,
	"traffic": true, "payments": true, "search_terms": true, "data_health": true,
}

// Query validates the scope and the request, then runs (or serves from cache) the report.
func (s *Service) Query(ctx context.Context, r Request) (*Result, error) {
	platformOnly, known := reports[r.Report]
	if !known {
		return nil, status.Error(codes.InvalidArgument, "unknown report")
	}
	var sc store.Scope
	switch r.Role {
	case "admin":
		sc = store.Scope{Platform: true}
	case "seller":
		sc = store.Scope{StoreID: r.StoreID}
		if err := sc.Validate(); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if platformOnly {
			return nil, status.Error(codes.PermissionDenied, "report is for administrators")
		}
	default:
		return nil, status.Error(codes.PermissionDenied, "role may not read analytics")
	}

	now := s.now().UTC()
	rg := store.Range{From: r.From.UTC(), To: r.To.UTC()}
	if rg.To.IsZero() {
		rg.To = now
	}
	if r.From.IsZero() {
		rg.From = rg.To.Add(-30 * 24 * time.Hour)
	}
	rg.From, rg.To = rg.From.Truncate(time.Second), rg.To.Truncate(time.Second)
	if r.Report != "low_stock" && r.Report != "data_health" {
		if err := rg.Validate(); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	g := r.Granularity
	if g == "" {
		g = "day"
	}
	if !store.ValidGranularity(g) {
		return nil, status.Error(codes.InvalidArgument, "granularity must be hour|day|week|month")
	}
	sort := r.Sort
	if sort == "" {
		sort = "revenue"
	}
	if r.Report == "top_products" && !store.ValidProductSort(sort) {
		return nil, status.Error(codes.InvalidArgument, "unknown sort")
	}

	key := cacheKey(r.Report, sc, rg, g, sort, r.Limit)
	if s.Cache != nil {
		if v, ok := s.Cache.Get(ctx, key); ok {
			var c cached
			if json.Unmarshal([]byte(v), &c) == nil {
				return &Result{AsOf: c.AsOf, Source: "clickhouse", Cached: true, Data: c.Data}, nil
			}
		}
	}

	var data any
	var err error
	switch r.Report {
	case "summary":
		data, err = s.Q.Summary(ctx, sc, rg)
	case "timeseries":
		data, err = s.Q.Timeseries(ctx, sc, rg, g)
	case "top_products":
		data, err = s.Q.TopProducts(ctx, sc, rg, sort, r.Limit)
	case "funnel":
		data, err = s.Q.Funnel(ctx, sc, rg)
	case "low_stock":
		data, err = s.Q.LowStock(ctx, sc, r.Limit)
	case "traffic":
		data, err = s.Q.Traffic(ctx, rg, g)
	case "payments":
		data, err = s.Q.Payments(ctx, rg)
	case "search_terms":
		data, err = s.Q.SearchTerms(ctx, rg, r.Limit)
	case "data_health":
		data, err = s.Q.DataHealth(ctx)
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "analytics store error")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, status.Error(codes.Internal, "encode")
	}
	res := &Result{AsOf: now, Source: "clickhouse", Data: b}
	if s.Cache != nil && s.CacheTTL > 0 && r.Report != "data_health" {
		if cb, err := json.Marshal(cached{AsOf: now, Data: b}); err == nil {
			s.Cache.Set(ctx, key, string(cb), s.CacheTTL)
		}
	}
	return res, nil
}

type cached struct {
	AsOf time.Time       `json:"as_of"`
	Data json.RawMessage `json:"data"`
}

func cacheKey(report string, sc store.Scope, rg store.Range, g, sort string, limit int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%v|%d|%d|%d|%s|%s|%d", report, sc.Platform, sc.StoreID, rg.From.Unix()/60, rg.To.Unix()/60, g, sort, limit)))
	return "analytics:v1:" + hex.EncodeToString(h[:16])
}
