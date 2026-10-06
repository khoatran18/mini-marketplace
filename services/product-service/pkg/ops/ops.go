// Package ops provides the operational HTTP endpoints every service exposes on a
// separate, internal-only admin port: /health, /healthz (liveness), /ready, /readyz
// (readiness), /metrics (Prometheus) and /version. It also registers the standard
// gRPC health service and records gRPC/HTTP/DB-pool metrics.
//
// The package has no dependencies on service code; the same file is copied into each
// service module (see ADR-8). Keep the copies identical.
package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const (
	StatusReady    = "ready"
	StatusDegraded = "degraded"
	StatusNotReady = "not_ready"

	defaultAdminPort     = "8081"
	defaultDrainSeconds  = 10
	defaultCacheSeconds  = 3
	defaultCheckTimeout  = time.Second
	grpcHealthRefreshGap = 5 * time.Second
)

// Check is one readiness probe. Critical checks make /ready return 503 when they fail;
// non-critical ones only mark the service "degraded" (still HTTP 200).
type Check struct {
	Name     string
	Critical bool
	Fn       func(ctx context.Context) error
}

// Options configures a Server.
type Options struct {
	Service string
	Checks  []Check
}

type checkResult struct {
	Status    string `json:"status"` // ok | fail
	LatencyMs int64  `json:"latency_ms"`
	Critical  bool   `json:"critical"`
	Error     string `json:"error,omitempty"`
}

type readyReport struct {
	Status  string                 `json:"status"`
	Service string                 `json:"service"`
	Version string                 `json:"version"`
	UptimeS int64                  `json:"uptime_s"`
	Checks  map[string]checkResult `json:"checks"`
}

// Server is the admin HTTP server plus the metrics registry of one service.
type Server struct {
	service string
	addr    string
	checks  []Check
	started time.Time

	startedFlag atomicBool
	draining    atomicBool

	cacheTTL     time.Duration
	checkTimeout time.Duration
	drain        time.Duration

	mu       sync.Mutex
	cached   readyReport
	cachedAt time.Time

	registry    *prometheus.Registry
	readyGauge  prometheus.Gauge
	grpcHandled *prometheus.CounterVec
	grpcLatency *prometheus.HistogramVec
	httpTotal   *prometheus.CounterVec
	httpLatency *prometheus.HistogramVec

	grpcHealth *health.Server
	httpServer *http.Server
}

type atomicBool struct {
	mu sync.RWMutex
	v  bool
}

func (a *atomicBool) Set(v bool) { a.mu.Lock(); a.v = v; a.mu.Unlock() }
func (a *atomicBool) Get() bool  { a.mu.RLock(); defer a.mu.RUnlock(); return a.v }

// New creates the admin server. Nothing is listening until Start (or ServeGRPC) is called.
func New(opts Options) *Server {
	svc := opts.Service
	s := &Server{
		service:      svc,
		addr:         ":" + envOr("ADMIN_PORT", defaultAdminPort),
		checks:       opts.Checks,
		started:      time.Now(),
		cacheTTL:     time.Duration(envInt("READY_CACHE_SECONDS", defaultCacheSeconds)) * time.Second,
		checkTimeout: time.Duration(envInt("READY_CHECK_TIMEOUT_MS", int(defaultCheckTimeout/time.Millisecond))) * time.Millisecond,
		drain:        time.Duration(envInt("DRAIN_SECONDS", defaultDrainSeconds)) * time.Second,
		registry:     prometheus.NewRegistry(),
	}
	constLabels := prometheus.Labels{"service": svc}
	s.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "mm_build_info",
		Help:        "Build information (value is always 1).",
		ConstLabels: prometheus.Labels{"service": svc, "version": version(), "commit": commit()},
	})
	buildInfo.Set(1)
	s.readyGauge = prometheus.NewGauge(prometheus.GaugeOpts{Name: "mm_ready", Help: "1 when /ready reports ready or degraded.", ConstLabels: constLabels})
	s.grpcHandled = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_grpc_server_handled_total", Help: "gRPC calls handled.", ConstLabels: constLabels}, []string{"grpc_method", "grpc_code"})
	s.grpcLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "mm_grpc_server_handling_seconds", Help: "gRPC handling time.", ConstLabels: constLabels, Buckets: latencyBuckets}, []string{"grpc_method"})
	s.httpTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_http_requests_total", Help: "HTTP requests handled.", ConstLabels: constLabels}, []string{"method", "route", "status"})
	s.httpLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "mm_http_request_duration_seconds", Help: "HTTP request time.", ConstLabels: constLabels, Buckets: latencyBuckets}, []string{"method", "route"})
	s.registry.MustRegister(buildInfo, s.readyGauge, s.grpcHandled, s.grpcLatency, s.httpTotal, s.httpLatency)
	return s
}

var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// AttachDB exports database/sql pool statistics as mm_db_pool_* metrics.
func (s *Server) AttachDB(db *sql.DB) {
	if db == nil {
		return
	}
	labels := prometheus.Labels{"service": s.service}
	s.registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "mm_db_pool_open_connections", Help: "Open DB connections.", ConstLabels: labels}, func() float64 { return float64(db.Stats().OpenConnections) }))
	s.registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "mm_db_pool_in_use_connections", Help: "In-use DB connections.", ConstLabels: labels}, func() float64 { return float64(db.Stats().InUse) }))
	s.registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "mm_db_pool_idle_connections", Help: "Idle DB connections.", ConstLabels: labels}, func() float64 { return float64(db.Stats().Idle) }))
	s.registry.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_db_pool_wait_seconds_total", Help: "Total time blocked waiting for a DB connection.", ConstLabels: labels}, func() float64 { return db.Stats().WaitDuration.Seconds() }))
}

// Registry exposes the registry so a service can register its own business metrics (prefix mm_).
func (s *Server) Registry() *prometheus.Registry { return s.registry }

// ObserveHTTP records one HTTP request. route must be the route template (e.g. /orders/:id), never the raw path.
func (s *Server) ObserveHTTP(method, route string, statusCode int, d time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	s.httpTotal.WithLabelValues(method, route, strconv.Itoa(statusCode)).Inc()
	s.httpLatency.WithLabelValues(method, route).Observe(d.Seconds())
}

// UnaryInterceptor records mm_grpc_server_* metrics for unary calls.
func (s *Server) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		s.observeGRPC(info.FullMethod, err, time.Since(start))
		return resp, err
	}
}

// StreamInterceptor records mm_grpc_server_* metrics for streaming calls.
func (s *Server) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		s.observeGRPC(info.FullMethod, err, time.Since(start))
		return err
	}
}

func (s *Server) observeGRPC(method string, err error, d time.Duration) {
	s.grpcHandled.WithLabelValues(method, status.Code(err).String()).Inc()
	s.grpcLatency.WithLabelValues(method).Observe(d.Seconds())
}

// MarkStarted flips the service to "started": call it once migrations, connections and workers are up.
func (s *Server) MarkStarted() { s.startedFlag.Set(true); s.invalidate(); s.refreshGRPCHealth() }

// StartDraining makes /ready (and gRPC health) report not ready; call it on shutdown before stopping servers.
func (s *Server) StartDraining() { s.draining.Set(true); s.invalidate(); s.refreshGRPCHealth() }

// Handler returns the admin routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	live := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
	ready := func(w http.ResponseWriter, r *http.Request) {
		rep := s.evaluate(r.Context())
		code := http.StatusOK
		if rep.Status == StatusNotReady {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, rep)
	}
	mux.HandleFunc("GET /health", live)
	mux.HandleFunc("GET /healthz", live)
	mux.HandleFunc("GET /ready", ready)
	mux.HandleFunc("GET /readyz", ready)
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": s.service, "version": version(), "commit": commit(), "built_at": envOr("BUILD_TIME", "unknown"),
		})
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	if os.Getenv("PPROF_ENABLED") == "true" {
		registerPprof(mux)
	}
	return mux
}

// Start listens on the admin port in the background.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.httpServer = &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("ops: admin server stopped: %v", err)
		}
	}()
	log.Printf("ops: %s admin endpoints on %s (/health /ready /metrics /version)", s.service, ln.Addr())
	return nil
}

// Shutdown stops the admin HTTP server.
func (s *Server) Shutdown(ctx context.Context) {
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(ctx)
	}
}

// ServeGRPC registers the gRPC health service, starts the admin port, marks the service
// started and serves g on lis. On SIGINT/SIGTERM it reports not-ready, waits DRAIN_SECONDS
// so load balancers can drop the instance, then stops gRPC gracefully (forced after 20 s).
func (s *Server) ServeGRPC(g *grpc.Server, lis net.Listener) error {
	s.RegisterGRPC(g)
	if err := s.Start(); err != nil {
		return err
	}
	go s.handleSignals(func() {
		done := make(chan struct{})
		go func() { g.GracefulStop(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			g.Stop()
		}
	})
	s.MarkStarted()
	return g.Serve(lis)
}

// ServeHTTPMain is the equivalent of ServeGRPC for the gateway's main HTTP server.
func (s *Server) ServeHTTPMain(srv *http.Server) error {
	if err := s.Start(); err != nil {
		return err
	}
	go s.handleSignals(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	s.MarkStarted()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handleSignals(stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	sig := <-ch
	log.Printf("ops: %s received %v, draining for %v", s.service, sig, s.drain)
	s.StartDraining()
	time.Sleep(s.drain)
	stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Shutdown(ctx)
}

// RegisterGRPC registers grpc.health.v1 on g and keeps its status in sync with readiness.
func (s *Server) RegisterGRPC(g *grpc.Server) {
	s.grpcHealth = health.NewServer()
	healthpb.RegisterHealthServer(g, s.grpcHealth)
	s.refreshGRPCHealth()
	go func() {
		t := time.NewTicker(grpcHealthRefreshGap)
		defer t.Stop()
		for range t.C {
			s.refreshGRPCHealth()
		}
	}()
}

func (s *Server) refreshGRPCHealth() {
	if s.grpcHealth == nil {
		return
	}
	st := healthpb.HealthCheckResponse_NOT_SERVING
	if rep := s.evaluate(context.Background()); rep.Status != StatusNotReady {
		st = healthpb.HealthCheckResponse_SERVING
	}
	s.grpcHealth.SetServingStatus("", st)
}

func (s *Server) invalidate() { s.mu.Lock(); s.cachedAt = time.Time{}; s.mu.Unlock() }

// evaluate runs all checks (in parallel, each with a timeout) and caches the report briefly so
// frequent probes cannot overload the dependencies.
func (s *Server) evaluate(ctx context.Context) readyReport {
	s.mu.Lock()
	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < s.cacheTTL {
		rep := s.cached
		s.mu.Unlock()
		return rep
	}
	s.mu.Unlock()

	rep := readyReport{Service: s.service, Version: version(), UptimeS: int64(time.Since(s.started).Seconds()), Checks: map[string]checkResult{}}
	var wg sync.WaitGroup
	var rmu sync.Mutex
	for _, c := range s.checks {
		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, s.checkTimeout)
			defer cancel()
			t0 := time.Now()
			err := c.Fn(cctx)
			res := checkResult{Status: "ok", LatencyMs: time.Since(t0).Milliseconds(), Critical: c.Critical}
			if err != nil {
				res.Status, res.Error = "fail", sanitize(err)
			}
			rmu.Lock()
			rep.Checks[c.Name] = res
			rmu.Unlock()
		}(c)
	}
	wg.Wait()

	status := StatusReady
	for _, r := range rep.Checks {
		if r.Status == "fail" {
			if r.Critical {
				status = StatusNotReady
				break
			}
			status = StatusDegraded
		}
	}
	switch {
	case s.draining.Get():
		status = StatusNotReady
		rep.Checks["draining"] = checkResult{Status: "fail", Critical: true, Error: "shutting_down"}
	case !s.startedFlag.Get():
		status = StatusNotReady
		rep.Checks["startup"] = checkResult{Status: "fail", Critical: true, Error: "starting"}
	}
	rep.Status = status
	if status == StatusNotReady {
		s.readyGauge.Set(0)
	} else {
		s.readyGauge.Set(1)
	}

	s.mu.Lock()
	s.cached, s.cachedAt = rep, time.Now()
	s.mu.Unlock()
	return rep
}

// sanitize maps an error to a short code so no addresses/credentials leak through /ready.
func sanitize(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return "refused"
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "password") || strings.Contains(msg, "auth"):
		return "auth"
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return strings.ToLower(st.Code().String())
	}
	return "error"
}

// ---- reusable checks --------------------------------------------------------------------

// SQLCheck pings a database/sql handle (e.g. gormDB.DB()).
func SQLCheck(name string, get func() (*sql.DB, error), critical bool) Check {
	return Check{Name: name, Critical: critical, Fn: func(ctx context.Context) error {
		db, err := get()
		if err != nil {
			return err
		}
		return db.PingContext(ctx)
	}}
}

// TCPCheck succeeds when any of the comma-separated host:port addresses accepts a connection (e.g. Kafka brokers).
func TCPCheck(name, addrs string, critical bool) Check {
	return Check{Name: name, Critical: critical, Fn: func(ctx context.Context) error {
		var last error
		for _, a := range strings.Split(addrs, ",") {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", a)
			if err == nil {
				_ = conn.Close()
				return nil
			}
			last = err
		}
		if last == nil {
			last = errors.New("no address configured")
		}
		return last
	}}
}

// GRPCHealthCheck asks another service's grpc.health.v1 whether it is serving.
func GRPCHealthCheck(name, addr string, critical bool) Check {
	return Check{Name: name, Critical: critical, Fn: func(ctx context.Context) error {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
		defer conn.Close()
		resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			return err
		}
		if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			return errors.New("not_serving")
		}
		return nil
	}}
}

// FuncCheck wraps any function (e.g. redisClient.Ping(ctx).Err()).
func FuncCheck(name string, critical bool, fn func(ctx context.Context) error) Check {
	return Check{Name: name, Critical: critical, Fn: fn}
}

// ---- helpers ----------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func version() string { return envOr("SERVICE_VERSION", "dev") }

func commit() string {
	if c := os.Getenv("GIT_COMMIT"); c != "" {
		return c
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}
	return "unknown"
}
