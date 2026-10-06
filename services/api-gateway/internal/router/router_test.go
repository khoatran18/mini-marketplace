package router

import (
	"api-gateway/internal/client"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const secret = "router-test-secret-long-enough"

func newEngine(t *testing.T) *gin.Engine {
	t.Helper()
	return newEngineEnv(t, nil)
}

// newEngineEnv builds the engine and lets the test adjust the environment config first.
func newEngineEnv(t *testing.T, mutate func(*config.EnvConfig)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	logger := zap.NewNop()
	serviceConfig := &config.ServiceConfig{ZapLogger: logger, RedisClient: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	envConfig := &config.EnvConfig{JWTSecret: secret, AllowedOrigins: []string{"https://shop.example.com"}, RateLimit: 1000, AuthRateLimit: 1000}
	if mutate != nil {
		mutate(envConfig)
	}
	engine := gin.New()
	SetupRouter(engine, handler.NewHandlerManager(client.NewClientManager(), logger), serviceConfig, envConfig)
	return engine
}

func token(t *testing.T, role string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.UserClaims{
		UserID: 1, Username: "u", Role: role, Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))},
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func request(e *gin.Engine, method, path, tok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestHealthIsPublic(t *testing.T) {
	if w := request(newEngine(t), "GET", "/healthz", ""); w.Code != 200 {
		t.Fatalf("/healthz must mirror /health, got %d", w.Code)
	}
	if w := request(newEngine(t), "GET", "/health", ""); w.Code != 200 {
		t.Errorf("status %d", w.Code)
	}
}

func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	e := newEngine(t)
	for _, route := range [][2]string{
		{"POST", "/auth/change-password"},
		{"POST", "/orders"},
		{"GET", "/orders"},
		{"GET", "/orders/1"},
		{"DELETE", "/orders/1"},
		{"POST", "/products"},
		{"PUT", "/products/1"},
	} {
		if w := request(e, route[0], route[1], ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: status %d, want 401", route[0], route[1], w.Code)
		}
	}
}

func TestOrderRoutesAreBuyerOnly(t *testing.T) {
	e := newEngine(t)
	for _, role := range []string{"seller_admin", "seller_employee"} {
		for _, route := range [][2]string{{"GET", "/orders"}, {"GET", "/orders/1"}, {"POST", "/orders"}, {"DELETE", "/orders/1"}} {
			if w := request(e, route[0], route[1], token(t, role)); w.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: status %d, want 403", route[0], route[1], role, w.Code)
			}
		}
	}
}

func TestProductWritesAreSellerOnly(t *testing.T) {
	e := newEngine(t)
	for _, route := range [][2]string{{"POST", "/products"}, {"PUT", "/products/1"}} {
		if w := request(e, route[0], route[1], token(t, "buyer")); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as buyer: status %d, want 403", route[0], route[1], w.Code)
		}
	}
}

func TestOrderUpdateEndpointIsNotExposed(t *testing.T) {
	// PUT /orders/:id let any user rewrite status, price and buyer; it must stay removed
	if w := request(newEngine(t), "PUT", "/orders/1", token(t, "buyer")); w.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", w.Code)
	}
}

func TestCORSAllowlist(t *testing.T) {
	e := newEngine(t)
	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("OPTIONS", "/products", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w
	}
	if got := preflight("https://shop.example.com").Header().Get("Access-Control-Allow-Origin"); got != "https://shop.example.com" {
		t.Errorf("allowed origin got %q", got)
	}
	if got := preflight("https://evil.example.com").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("foreign origin must not be allowed, got %q", got)
	}
}

func TestAdminSystemHealthIsAdminOnly(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Errorf("probe must call /ready, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready","service":"order-service","version":"1.0.0","uptime_s":5,"checks":{"postgres":{"status":"ok"}}}`))
	}))
	defer ready.Close()
	degraded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready","checks":{"kafka":{"status":"fail","error":"refused"}}}`))
	}))
	defer degraded.Close()
	e := newEngineEnv(t, func(c *config.EnvConfig) {
		c.SystemTargets = map[string]string{"order-service": ready.URL, "product-service": degraded.URL, "ghost": "http://127.0.0.1:1"}
	})

	if w := request(e, "GET", "/admin/system/health", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", w.Code)
	}
	for _, role := range []string{"buyer", "seller_admin", "seller_employee"} {
		if w := request(e, "GET", "/admin/system/health", token(t, role)); w.Code != http.StatusForbidden {
			t.Errorf("%s must be forbidden, got %d", role, w.Code)
		}
	}
	w := request(e, "GET", "/admin/system/health", token(t, "admin"))
	if w.Code != 200 {
		t.Fatalf("admin: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Status   string `json:"status"`
		Services []struct {
			Name, Status, Version string
		} `json:"services"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range out.Services {
		got[s.Name] = s.Status
	}
	if got["order-service"] != "ready" || got["product-service"] != "not_ready" || got["ghost"] != "unreachable" {
		t.Errorf("unexpected service statuses: %v", got)
	}
	if out.Status != "unreachable" || len(out.Services) != 3 || out.Services[0].Name != "ghost" {
		t.Errorf("worst status / ordering wrong: %+v", out)
	}
}

func TestBodyLimitAppliesToEveryRoute(t *testing.T) {
	e := newEngineEnv(t, func(c *config.EnvConfig) { c.MaxBodyBytes = 64 })
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(strings.Repeat("a", 65)))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", w.Code)
	}
}

func TestCatalogRouteAccessRules(t *testing.T) {
	e := newEngine(t)
	// admin-only and seller-only routes: anonymous -> 401, wrong role -> 403
	for _, route := range []struct{ method, path, wrongRole string }{
		{"GET", "/admin/categories", "seller_admin"},
		{"POST", "/admin/categories", "buyer"},
		{"PUT", "/admin/categories/1", "seller_admin"},
		{"PATCH", "/admin/products/1/status", "seller_admin"},
		{"GET", "/seller/products", "buyer"},
		{"GET", "/seller/inventory/low-stock", "buyer"},
		{"POST", "/seller/products/1/inventory/adjust", "buyer"},
		{"GET", "/seller/products/1/inventory/ledger", "admin"},
		{"PATCH", "/products/1/status", "buyer"},
	} {
		if w := request(e, route.method, route.path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", route.method, route.path, w.Code)
		}
		if w := request(e, route.method, route.path, token(t, route.wrongRole)); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as %s: %d, want 403", route.method, route.path, route.wrongRole, w.Code)
		}
	}
	// a malformed token on a public route is rejected rather than treated as anonymous
	for _, path := range []string{"/search", "/products", "/products/1", "/products/seller/1"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer not-a-jwt")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with a bad token: %d, want 401", path, w.Code)
		}
	}
}

func TestOrderRouteAccessRules(t *testing.T) {
	e := newEngine(t)
	for _, route := range []struct{ method, path, wrongRole string }{
		{"GET", "/cart", "seller_admin"},
		{"PUT", "/cart/items/1", "admin"},
		{"POST", "/cart/merge", "seller_employee"},
		{"POST", "/checkout/preview", "admin"},
		{"GET", "/checkouts/co_1", "seller_admin"},
		{"POST", "/orders", "seller_admin"},
		{"GET", "/orders", "admin"},
		{"GET", "/orders/summary", "admin"},
		{"POST", "/orders/1/cancel", "seller_admin"},
		{"POST", "/orders/1/confirm-received", "admin"},
		{"POST", "/orders/1/return", "seller_admin"},
		{"GET", "/users/me/addresses", "seller_admin"},
		{"POST", "/users/me/addresses", "admin"},
		{"GET", "/seller/orders", "buyer"},
		{"GET", "/seller/orders/summary", "admin"},
		{"POST", "/seller/orders/1/ship", "buyer"},
		{"POST", "/seller/orders/1/return/approve", "buyer"},
		{"GET", "/admin/orders", "seller_admin"},
		{"POST", "/admin/orders/1/cancel", "buyer"},
		{"POST", "/admin/orders/1/return/approve", "seller_admin"},
	} {
		if w := request(e, route.method, route.path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", route.method, route.path, w.Code)
		}
		if w := request(e, route.method, route.path, token(t, route.wrongRole)); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as %s: %d, want 403", route.method, route.path, route.wrongRole, w.Code)
		}
	}
}
