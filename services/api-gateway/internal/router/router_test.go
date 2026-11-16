package router

import (
	"api-gateway/internal/client"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"encoding/json"
	"io"
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
	// Redis "đã chạy đủ lâu" (cửa sổ an toàn của middleware đã qua) nên đường nhanh được tin; không có auth-service trong test
	_ = mr.Set("pwd_version:safe_after", "1")
	logger := zap.NewNop()
	serviceConfig := &config.ServiceConfig{ZapLogger: logger, RedisClient: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	envConfig := &config.EnvConfig{JWTSecret: secret, AllowedOrigins: []string{"https://shop.example.com"}, RateLimit: 1000, AuthRateLimit: 1000}
	if mutate != nil {
		mutate(envConfig)
	}
	engine := gin.New()
	SetupRouter(engine, handler.NewHandlerManager(client.NewClientManager(), "test-secret", logger), serviceConfig, envConfig)
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

	if w := request(e, "GET", "/ops/system/health", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", w.Code)
	}
	for _, role := range []string{"buyer", "seller_admin", "seller_employee"} {
		if w := request(e, "GET", "/ops/system/health", token(t, role)); w.Code != http.StatusForbidden {
			t.Errorf("%s must be forbidden, got %d", role, w.Code)
		}
	}
	w := request(e, "GET", "/ops/system/health", token(t, "admin"))
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
		{"GET", "/admin/accounts", "seller_admin"},
		{"POST", "/admin/accounts", "buyer"},
		{"DELETE", "/admin/accounts/1", "seller_admin"},
		{"POST", "/admin/accounts/1/reset-password", "operator"},
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
		{"GET", "/payments/1", "seller_admin"},
		{"POST", "/payments/1/confirm", "admin"},
		{"POST", "/payments/1/cancel", "seller_employee"},
		{"GET", "/admin/payments", "buyer"},
		{"GET", "/admin/payments/1", "seller_admin"},
		{"POST", "/admin/payments/1/refund", "buyer"},
		{"POST", "/admin/dev/payments/1/force", "seller_admin"},
		{"GET", "/seller/analytics/summary", "buyer"},
		{"GET", "/seller/analytics/top-products", "admin"},
		{"GET", "/admin/analytics/summary", "seller_admin"},
		{"GET", "/admin/analytics/traffic", "seller_admin"},
		{"GET", "/admin/analytics/data-health", "buyer"},
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

// Regression: a global router.Use(authenticated) once made every "public" route answer 401.
func TestEventsRoutes(t *testing.T) {
	e := newEngine(t)
	if w := request(e, "POST", "/events/identify", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("identify anonymous: %d", w.Code)
	}
	// /events is public: an empty body is a 400 from the handler, not a 401
	if w := request(e, "POST", "/events", ""); w.Code != http.StatusBadRequest {
		t.Errorf("POST /events anonymous: %d", w.Code)
	}
	req := httptest.NewRequest("POST", "/events", strings.NewReader(`{"events":[{"event_type":"page_view"}]}`))
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("a malformed token is rejected, not treated as anonymous: %d", w.Code)
	}
}

func TestPublicRoutesNeedNoToken(t *testing.T) {
	e := newEngine(t)
	for _, path := range []string{"/search", "/categories", "/products", "/products/1", "/products/1/stock", "/payments/methods", "/shop/rules", "/health", "/healthz"} {
		if w := request(e, "GET", path, ""); w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
			t.Errorf("GET %s anonymous: %d, must be public", path, w.Code)
		}
	}
	for _, path := range []string{"/users/buyers/1", "/users/sellers/1"} {
		if w := request(e, "GET", path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous: %d, want 401", path, w.Code)
		}
	}
}

func TestOpsRoutesAdminOnlyAndProxied(t *testing.T) {
	var gotAuth, gotActor, gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotActor, gotPath = r.Header.Get("Authorization"), r.Header.Get("X-Actor"), r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"incidents":[]}`))
	}))
	defer up.Close()
	e := newEngineEnv(t, func(c *config.EnvConfig) { c.OpsServiceURL, c.OpsAPIToken = up.URL, "ops-secret" })
	for _, route := range []struct{ method, path string }{
		{"GET", "/ops/incidents"}, {"GET", "/ops/incidents/1"}, {"POST", "/ops/incidents/1/investigate"}, {"GET", "/ops/audit"},
		{"GET", "/ops/actions"}, {"GET", "/ops/proposals"}, {"POST", "/ops/proposals"}, {"GET", "/ops/proposals/1"},
		{"POST", "/ops/proposals/1/review"}, {"POST", "/ops/proposals/1/decision"}, {"POST", "/ops/proposals/1/withdraw"},
		{"POST", "/ops/proposals/1/execute"}, {"POST", "/ops/proposals/1/executed"}, {"GET", "/ops/queries"}, {"GET", "/ops/queries/service_up"}, {"POST", "/ops/queries/service_up"}, {"GET", "/ops/outbox/order-service/dead"}, {"GET", "/ops/playbooks"}, {"GET", "/ops/playbooks/outbox-stuck"}, {"POST", "/ops/queries/gateway_latency_p95"},
	} {
		if w := request(e, route.method, route.path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", route.method, route.path, w.Code)
		}
		for _, role := range []string{"buyer", "seller_admin", "seller_employee"} {
			if w := request(e, route.method, route.path, token(t, role)); w.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403", route.method, route.path, role, w.Code)
			}
		}
	}
	// an "operator" runs the whole ops workflow (read, propose, review, decide, execute) but never reports/revenue/other admin routes (ADR-40)
	for _, route := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/ops/incidents", 200}, {"GET", "/ops/proposals", 200}, {"POST", "/ops/proposals", 200},
		{"POST", "/ops/queries/service_up", 200}, {"GET", "/ops/queries/service_up", 200}, {"POST", "/ops/incidents/1/investigate", 200}, {"GET", "/ops/system/health", -1},
		{"POST", "/ops/proposals/1/review", 200}, {"POST", "/ops/proposals/1/decision", 200}, {"POST", "/ops/proposals/1/withdraw", 200},
		{"POST", "/ops/proposals/1/execute", 200}, {"POST", "/ops/proposals/1/executed", 200}, {"GET", "/admin/analytics/data-health", -1},
		{"GET", "/admin/analytics/summary", 403}, {"GET", "/admin/analytics/traffic", 403}, {"GET", "/admin/payments", 403}, {"GET", "/admin/orders", 403},
		{"GET", "/admin/categories", 403}, {"GET", "/admin/accounts", 403}, {"POST", "/admin/accounts", 403}, {"DELETE", "/admin/accounts/1", 403}, {"POST", "/admin/orders/1/cancel", 403}, {"POST", "/admin/payments/1/refund", 403},
	} {
		w := request(e, route.method, route.path, token(t, "operator"))
		if route.want == -1 {
			if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
				t.Errorf("operator %s %s: %d, must be allowed", route.method, route.path, w.Code)
			}
		} else if w.Code != route.want {
			t.Errorf("operator %s %s: %d, want %d", route.method, route.path, w.Code, route.want)
		}
	}
	request(e, "GET", "/ops/incidents", token(t, "operator"))
	if !strings.HasPrefix(gotActor, "operator:") {
		t.Errorf("actor for operator: %q", gotActor)
	}
	if w := request(e, "GET", "/ops/incidents?state=open", token(t, "admin")); w.Code != 200 || gotPath != "/incidents?state=open" || gotAuth != "Bearer ops-secret" || !strings.HasPrefix(gotActor, "admin:") {
		t.Fatalf("proxy: %d path=%q auth=%q actor=%q", w.Code, gotPath, gotAuth, gotActor)
	}
	// POST body is forwarded as JSON
	var gotBody, gotCT string
	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotCT = string(b), r.Header.Get("Content-Type")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"proposal":{}}`))
	}))
	defer up2.Close()
	e3 := newEngineEnv(t, func(c *config.EnvConfig) { c.OpsServiceURL, c.OpsAPIToken = up2.URL, "ops-secret" })
	req := httptest.NewRequest("POST", "/ops/proposals/5/decision", strings.NewReader(`{"decision":"accept"}`))
	req.Header.Set("Authorization", "Bearer "+token(t, "admin"))
	rec := httptest.NewRecorder()
	e3.ServeHTTP(rec, req)
	if rec.Code != 201 || gotBody != `{"decision":"accept"}` || gotCT != "application/json" {
		t.Errorf("body forward: %d body=%q ct=%q", rec.Code, gotBody, gotCT)
	}
	big := httptest.NewRequest("POST", "/ops/proposals", strings.NewReader(strings.Repeat("x", 70<<10)))
	big.Header.Set("Authorization", "Bearer "+token(t, "admin"))
	rec = httptest.NewRecorder()
	e3.ServeHTTP(rec, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize body: %d, want 413", rec.Code)
	}
	// query names may contain digits (p95); anything outside [a-z0-9_] is rejected before reaching the upstream
	gotPath = ""
	if w := request(e, "POST", "/ops/queries/gateway_latency_p95", token(t, "admin")); w.Code != 200 || gotPath != "/queries/gateway_latency_p95" {
		t.Errorf("query name with digits: %d path=%q", w.Code, gotPath)
	}
	if w := request(e, "POST", "/ops/queries/Bad-Name", token(t, "admin")); w.Code != http.StatusBadRequest {
		t.Errorf("bad query name: %d, want 400", w.Code)
	}
	if w := request(e, "GET", "/ops/incidents/abc", token(t, "admin")); w.Code != http.StatusBadRequest {
		t.Errorf("non numeric id: %d, want 400", w.Code)
	}
	// without a token the proxy is off: the upstream is never called
	e2 := newEngineEnv(t, func(c *config.EnvConfig) { c.OpsServiceURL, c.OpsAPIToken = up.URL, "" })
	gotPath = ""
	if w := request(e2, "GET", "/ops/incidents", token(t, "admin")); w.Code != http.StatusServiceUnavailable || gotPath != "" {
		t.Errorf("disabled proxy: %d path=%q", w.Code, gotPath)
	}
}

func TestFeaturesPublicAndAIChatAuthenticatedAndGated(t *testing.T) {
	var gotPath, gotRole string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRole = r.URL.Path, r.Header.Get("X-User-Role")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	defer up.Close()
	// flag off: features say so (public), chat 404 for a signed-in user, 401 anonymous
	off := newEngine(t)
	w := request(off, "GET", "/config/features", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ai_chat":false`) {
		t.Errorf("features (off): %d %s", w.Code, w.Body.String())
	}
	if w := request(off, "POST", "/ai/chat", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous chat: %d, want 401", w.Code)
	}
	if w := request(off, "POST", "/ai/chat", token(t, "buyer")); w.Code != http.StatusNotFound {
		t.Errorf("chat with flag off: %d, want 404", w.Code)
	}
	// flag on + upstream: allowed for buyer/seller/admin/operator, stream reaches the client
	on := newEngineEnv(t, func(c *config.EnvConfig) {
		c.FeatureAIChat, c.AIServiceURL, c.AIStreamTimeout = true, up.URL, 5*time.Second
	})
	if w := request(on, "GET", "/config/features", ""); !strings.Contains(w.Body.String(), `"ai_chat":true`) {
		t.Errorf("features (on): %s", w.Body.String())
	}
	for _, role := range []string{"buyer", "seller_admin", "seller_employee", "admin", "operator"} {
		w := request(on, "POST", "/ai/chat", token(t, role))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "data: ok") || gotPath != "/chat" || gotRole != role {
			t.Errorf("chat as %s: %d %q path=%q role=%q", role, w.Code, w.Body.String(), gotPath, gotRole)
		}
	}
}

func TestAIHistoryRoutes(t *testing.T) {
	var gotPath, gotQuery string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer up.Close()
	off := newEngine(t)
	if w := request(off, "GET", "/ai/conversations", token(t, "buyer")); w.Code != http.StatusNotFound {
		t.Errorf("history with flag off: %d, want 404", w.Code)
	}
	on := newEngineEnv(t, func(c *config.EnvConfig) { c.FeatureAIChat, c.AIServiceURL = true, up.URL })
	if w := request(on, "GET", "/ai/conversations", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous history: %d, want 401", w.Code)
	}
	for _, role := range []string{"buyer", "seller_admin", "admin", "operator"} {
		if w := request(on, "GET", "/ai/conversations?limit=5&user_id=1", token(t, role)); w.Code != 200 || gotPath != "/conversations" || gotQuery != "limit=5" {
			t.Errorf("own history as %s: %d path=%q query=%q (only limit may pass through)", role, w.Code, gotPath, gotQuery)
		}
	}
	if w := request(on, "GET", "/ai/conversations/abc-123", token(t, "buyer")); w.Code != 200 || gotPath != "/conversations/abc-123" {
		t.Errorf("one conversation: %d path=%q", w.Code, gotPath)
	}
	// everyone's history is a platform-admin route
	for _, role := range []string{"buyer", "seller_admin", "operator"} {
		if w := request(on, "GET", "/admin/ai/conversations", token(t, role)); w.Code != http.StatusForbidden {
			t.Errorf("admin history as %s: %d, want 403", role, w.Code)
		}
	}
	if w := request(on, "GET", "/admin/ai/conversations?role=buyer&user_id=9&x=1", token(t, "admin")); w.Code != 200 || gotPath != "/admin/conversations" || gotQuery != "role=buyer&user_id=9" {
		t.Errorf("admin history: %d path=%q query=%q", w.Code, gotPath, gotQuery)
	}
	if w := request(on, "GET", "/admin/ai/conversations/abc-123", token(t, "admin")); w.Code != 200 || gotPath != "/admin/conversations/abc-123" {
		t.Errorf("admin one: %d path=%q", w.Code, gotPath)
	}
}

const sysKey = "system-key-system-key-system-key-0"

func requestWith(e *gin.Engine, method, path, tok string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

// ADR-47: POST /ai/system-token exists only with AI_SYSTEM_KEY, needs that exact key, and the token it returns is a read-only /ops pass.
func TestSystemTokenRoute(t *testing.T) {
	keys := "k1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-A"
	off := newEngineEnv(t, func(c *config.EnvConfig) { c.AgentKeys = keys })
	if w := requestWith(off, "POST", "/ai/system-token", "", map[string]string{"X-System-Key": sysKey}, ""); w.Code != http.StatusNotFound {
		t.Errorf("no AI_SYSTEM_KEY: %d, want 404", w.Code)
	}
	var gotPath, gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.RequestURI(), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"incidents":[]}`))
	}))
	defer up.Close()
	e := newEngineEnv(t, func(c *config.EnvConfig) {
		c.AgentKeys, c.AISystemKey, c.OpsServiceURL, c.OpsAPIToken = keys, sysKey, up.URL, "ops-secret"
	})
	for _, hdr := range []map[string]string{nil, {"X-System-Key": "wrong"}, {"X-System-Key": sysKey + "x"}} {
		if w := requestWith(e, "POST", "/ai/system-token", "", hdr, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("headers %v: %d, want 401", hdr, w.Code)
		}
	}
	w := requestWith(e, "POST", "/ai/system-token", "", map[string]string{"X-System-Key": sysKey}, "")
	if w.Code != 200 {
		t.Fatalf("right key: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" || out.ExpiresIn != 900 {
		t.Fatalf("body %s", w.Body.String())
	}
	// the token reads the ops view...
	if w := requestWith(e, "GET", "/ops/incidents?state=open", out.Token, nil, ""); w.Code != 200 || gotPath != "/incidents?state=open" || gotAuth != "Bearer ops-secret" {
		t.Errorf("ops read with the system token: %d path=%q auth=%q", w.Code, gotPath, gotAuth)
	}
	// ...and nothing else: no write, no other area, no proposal
	for _, r := range []struct{ method, path string }{
		{"POST", "/ops/proposals"}, {"POST", "/ops/proposals/1/decision"}, {"POST", "/ops/incidents/1/investigate"}, {"GET", "/admin/orders"},
		{"GET", "/admin/accounts"}, {"GET", "/orders"}, {"POST", "/ai/proposals"}, {"POST", "/ai/chat"}, {"POST", "/ops/rca/runs"},
	} {
		if w := requestWith(e, r.method, r.path, out.Token, nil, `{}`); w.Code < 400 {
			t.Errorf("%s %s with a system token: %d, must be refused", r.method, r.path, w.Code)
		}
	}
}

// The RCA routes forward to ai-agent for admin/operator only; confirming a draft is the operator's alone.
func TestRCARoutes(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotTok string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotBody, gotTok = r.Method, r.URL.RequestURI(), string(b), r.Header.Get("X-Agent-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer up.Close()
	keys := "k1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-A"
	off := newEngineEnv(t, func(c *config.EnvConfig) { c.AgentKeys, c.AIServiceURL = keys, up.URL })
	if w := request(off, "GET", "/ops/rca/runs", token(t, "operator")); w.Code != http.StatusNotFound {
		t.Errorf("FEATURE_AI_OPS off: %d, want 404", w.Code)
	}
	on := newEngineEnv(t, func(c *config.EnvConfig) { c.AgentKeys, c.AIServiceURL, c.FeatureAIOps = keys, up.URL, true })
	routes := []struct{ method, path string }{
		{"GET", "/ops/rca/runs"}, {"GET", "/ops/rca/runs/7"}, {"GET", "/ops/rca/runs/7/events"}, {"POST", "/ops/rca/runs"}, {"POST", "/ops/rca/runs/7/recheck"}, {"POST", "/ops/rca/runs/7/cancel"}, {"POST", "/ops/rca/runs/7/proposal"},
	}
	for _, r := range routes {
		if w := request(on, r.method, r.path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", r.method, r.path, w.Code)
		}
		for _, role := range []string{"buyer", "seller_admin", "seller_employee"} {
			if w := request(on, r.method, r.path, token(t, role)); w.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403", r.method, r.path, role, w.Code)
			}
		}
	}
	for _, role := range []string{"admin", "operator"} {
		if w := requestWith(on, "GET", "/ops/rca/runs?incident_id=5&limit=3&x=1", token(t, role), nil, ""); w.Code != 200 || gotPath != "/rca/runs?incident_id=5&limit=3" || gotTok == "" {
			t.Errorf("list as %s: %d path=%q token=%q", role, w.Code, gotPath, gotTok)
		}
		if w := requestWith(on, "POST", "/ops/rca/runs", token(t, role), nil, `{"incident_id":5}`); w.Code != 200 || gotMethod != "POST" || gotPath != "/rca/runs" || gotBody != `{"incident_id":5}` {
			t.Errorf("start as %s: %d %s %s %s", role, w.Code, gotMethod, gotPath, gotBody)
		}
		if w := requestWith(on, "GET", "/ops/rca/runs/7/events?after=12&x=1", token(t, role), nil, ""); w.Code != 200 || gotPath != "/rca/runs/7/events?after=12" {
			t.Errorf("%s events: %d path %q", role, w.Code, gotPath)
		}
		if w := request(on, "POST", "/ops/rca/runs/7/cancel", token(t, role)); w.Code != 200 || gotMethod != "POST" || gotPath != "/rca/runs/7/cancel" {
			t.Errorf("%s cancel: %d %s %s", role, w.Code, gotMethod, gotPath)
		}
		if w := request(on, "POST", "/ops/rca/runs/7/recheck", token(t, role)); w.Code != 200 || gotPath != "/rca/runs/7/recheck" {
			t.Errorf("recheck as %s: %d path=%q", role, w.Code, gotPath)
		}
	}
	if w := request(on, "POST", "/ops/rca/runs/7/proposal", token(t, "admin")); w.Code != http.StatusForbidden {
		t.Errorf("an admin must not sign a draft: %d, want 403", w.Code)
	}
	if w := request(on, "POST", "/ops/rca/runs/7/proposal", token(t, "operator")); w.Code != 200 || gotPath != "/rca/runs/7/proposal" {
		t.Errorf("operator draft: %d path=%q", w.Code, gotPath)
	}
	if w := request(on, "GET", "/ops/rca/runs/a%2F..%2Fb", token(t, "operator")); w.Code == 200 && strings.Contains(gotPath, "/../") {
		t.Errorf("path traversal reached the upstream: %q", gotPath)
	}
}

// Playbook ids are slugs ("analytics-data"), not numbers: the gateway used to answer 400 "invalid id" for every one (RCA tool errors, 16.32).
func TestOpsPlaybookSlugIsForwarded(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer up.Close()
	e := newEngineEnv(t, func(c *config.EnvConfig) { c.OpsServiceURL, c.OpsAPIToken = up.URL, "ops-secret" })
	for _, id := range []string{"analytics-data", "outbox-stuck", "db-pool-2"} {
		gotPath = ""
		if w := request(e, "GET", "/ops/playbooks/"+id, token(t, "operator")); w.Code != 200 || gotPath != "/playbooks/"+id {
			t.Errorf("playbook %s: %d path=%q", id, w.Code, gotPath)
		}
	}
	for _, id := range []string{"Bad_ID", "a.b", "%2e%2e", strings.Repeat("a", 65)} {
		gotPath = ""
		if w := request(e, "GET", "/ops/playbooks/"+id, token(t, "operator")); w.Code != 400 || gotPath != "" {
			t.Errorf("playbook %q: %d path=%q, want 400 and no upstream call", id, w.Code, gotPath)
		}
	}
}
