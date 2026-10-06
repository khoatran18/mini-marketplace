package router

import (
	"api-gateway/internal/client"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"net/http"
	"net/http/httptest"
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
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	logger := zap.NewNop()
	serviceConfig := &config.ServiceConfig{ZapLogger: logger, RedisClient: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	envConfig := &config.EnvConfig{JWTSecret: secret, AllowedOrigins: []string{"https://shop.example.com"}, RateLimit: 1000, AuthRateLimit: 1000}
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
