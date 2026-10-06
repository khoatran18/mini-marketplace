package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func newTestServer(checks ...Check) *Server {
	t := New(Options{Service: "test-service", Checks: checks})
	t.cacheTTL = 0 // no caching unless a test asks for it
	return t
}

func get(t *testing.T, h http.Handler, path string) (int, readyReport, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var rep readyReport
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	return rec.Code, rep, rec.Body.String()
}

func TestLivenessIgnoresDependencies(t *testing.T) {
	s := newTestServer(Check{Name: "db", Critical: true, Fn: func(context.Context) error { return errors.New("down") }})
	for _, p := range []string{"/health", "/healthz"} {
		if code, _, body := get(t, s.Handler(), p); code != 200 || !strings.Contains(body, `"ok"`) {
			t.Fatalf("%s = %d %s", p, code, body)
		}
	}
}

func TestReadyBeforeStartedIs503(t *testing.T) {
	s := newTestServer()
	code, rep, _ := get(t, s.Handler(), "/ready")
	if code != 503 || rep.Status != StatusNotReady || rep.Checks["startup"].Status != "fail" {
		t.Fatalf("got %d %+v", code, rep)
	}
}

func TestReadyOKDegradedAndNotReady(t *testing.T) {
	var critical, optional error
	s := newTestServer(
		Check{Name: "postgres", Critical: true, Fn: func(context.Context) error { return critical }},
		Check{Name: "redis", Critical: false, Fn: func(context.Context) error { return optional }},
	)
	s.MarkStarted()
	for _, p := range []string{"/ready", "/readyz"} {
		if code, rep, _ := get(t, s.Handler(), p); code != 200 || rep.Status != StatusReady {
			t.Fatalf("%s healthy: %d %+v", p, code, rep)
		}
	}
	optional = errors.New("dial tcp 10.0.0.5:6379: connection refused")
	code, rep, body := get(t, s.Handler(), "/ready")
	if code != 200 || rep.Status != StatusDegraded {
		t.Fatalf("degraded: %d %+v", code, rep)
	}
	if rep.Checks["redis"].Error != "refused" || strings.Contains(body, "10.0.0.5") {
		t.Fatalf("error must be sanitized, got %s", body)
	}
	critical = errors.New("boom")
	if code, rep, _ := get(t, s.Handler(), "/ready"); code != 503 || rep.Status != StatusNotReady {
		t.Fatalf("not ready: %d %+v", code, rep)
	}
	// liveness is unaffected by the dependency failure
	if code, _, _ := get(t, s.Handler(), "/health"); code != 200 {
		t.Fatalf("liveness must stay 200, got %d", code)
	}
}

func TestCheckTimeout(t *testing.T) {
	s := newTestServer(Check{Name: "slow", Critical: true, Fn: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	s.checkTimeout = 50 * time.Millisecond
	s.MarkStarted()
	start := time.Now()
	code, rep, _ := get(t, s.Handler(), "/ready")
	if code != 503 || rep.Checks["slow"].Error != "timeout" || time.Since(start) > time.Second {
		t.Fatalf("got %d %+v after %v", code, rep, time.Since(start))
	}
}

func TestReadyIsCached(t *testing.T) {
	calls := 0
	s := newTestServer(Check{Name: "c", Critical: true, Fn: func(context.Context) error { calls++; return nil }})
	s.cacheTTL = time.Minute
	s.MarkStarted()
	for i := 0; i < 5; i++ {
		get(t, s.Handler(), "/ready")
	}
	if calls != 1 {
		t.Fatalf("expected 1 evaluation within cache TTL, got %d", calls)
	}
}

func TestDrainingMakesNotReadyButMetricsStay(t *testing.T) {
	s := newTestServer()
	s.MarkStarted()
	if code, _, _ := get(t, s.Handler(), "/ready"); code != 200 {
		t.Fatalf("want ready, got %d", code)
	}
	s.StartDraining()
	if code, rep, _ := get(t, s.Handler(), "/ready"); code != 503 || rep.Checks["draining"].Status != "fail" {
		t.Fatalf("draining: %d %+v", code, rep)
	}
	if code, _, body := get(t, s.Handler(), "/metrics"); code != 200 || !strings.Contains(body, "mm_ready") {
		t.Fatalf("metrics must stay available: %d", code)
	}
}

func TestVersionAndMetrics(t *testing.T) {
	t.Setenv("SERVICE_VERSION", "1.2.3")
	s := newTestServer()
	s.MarkStarted()
	_, _, body := get(t, s.Handler(), "/version")
	if !strings.Contains(body, `"service":"test-service"`) || !strings.Contains(body, `"version":"1.2.3"`) {
		t.Fatalf("version body: %s", body)
	}
	s.ObserveHTTP("GET", "/orders/:id", 200, 20*time.Millisecond)
	s.observeGRPC("/order.OrderService/GetOrder", nil, time.Millisecond)
	get(t, s.Handler(), "/ready")
	_, _, m := get(t, s.Handler(), "/metrics")
	for _, want := range []string{
		`mm_build_info{commit=`, `mm_http_requests_total{method="GET",route="/orders/:id",service="test-service",status="200"} 1`,
		`mm_grpc_server_handled_total{grpc_code="OK"`, `mm_ready{service="test-service"} 1`, `go_goroutines{service="test-service"}`, `process_resident_memory_bytes{service="test-service"}`,
	} {
		if !strings.Contains(m, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestUnmatchedRouteLabel(t *testing.T) {
	s := newTestServer()
	s.ObserveHTTP("GET", "", 404, time.Millisecond)
	_, _, m := get(t, s.Handler(), "/metrics")
	if !strings.Contains(m, `route="unmatched"`) {
		t.Fatal("empty route must be labelled unmatched to avoid cardinality blow-up")
	}
}

func TestGRPCHealthFollowsReadiness(t *testing.T) {
	s := newTestServer()
	g := grpc.NewServer()
	s.RegisterGRPC(g)
	s.MarkStarted()
	s.StartDraining()
	// status is set synchronously by StartDraining; no network round-trip needed to assert the flag
	if rep := s.evaluate(context.Background()); rep.Status != StatusNotReady {
		t.Fatalf("expected not_ready while draining, got %s", rep.Status)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
}

// Real sockets: admin HTTP port + gRPC health on the same process.
func TestServeGRPCEndToEnd(t *testing.T) {
	t.Setenv("ADMIN_PORT", freePort(t))
	s := New(Options{Service: "e2e", Checks: []Check{FuncCheck("always", true, func(context.Context) error { return nil })}})
	g := grpc.NewServer(grpc.ChainUnaryInterceptor(s.UnaryInterceptor()))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.ServeGRPC(g, lis) }()
	defer g.Stop()
	defer s.Shutdown(context.Background())

	base := "http://127.0.0.1" + s.addr
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/ready")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if err == nil {
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("admin /ready never became 200 (err=%v)", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, p := range []string{"/health", "/healthz", "/readyz", "/version", "/metrics"} {
		resp, err := http.Get(base + p)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: err=%v resp=%v", p, err, resp)
		}
		resp.Body.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := GRPCHealthCheck("self", lis.Addr().String(), true).Fn(ctx); err != nil {
		t.Fatalf("grpc health should be SERVING: %v", err)
	}
	s.StartDraining()
	if err := GRPCHealthCheck("self", lis.Addr().String(), true).Fn(ctx); err == nil {
		t.Fatal("grpc health must report NOT_SERVING while draining")
	}
}

func TestTCPCheck(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	ctx := context.Background()
	if err := TCPCheck("k", "127.0.0.1:1,"+l.Addr().String(), true).Fn(ctx); err != nil {
		t.Fatalf("second address is reachable: %v", err)
	}
	if err := TCPCheck("k", "", true).Fn(ctx); err == nil {
		t.Fatal("empty address list must fail")
	}
}
