package middleware

import (
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

const secret = "test-secret-with-enough-length"

func newRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()}), mr
}

func sign(t *testing.T, claims UserClaims, key string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func claims(typ string, pwdVersion int64, expires time.Duration) UserClaims {
	return UserClaims{
		UserID: 42, Username: "alice", Role: "buyer", PwdVersion: pwdVersion, Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(expires))},
	}
}

func authEngine(rdb *redis.Client, roles ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handlers := []gin.HandlerFunc{AuthMiddleware(zap.NewNop(), rdb, secret)}
	if len(roles) > 0 {
		handlers = append(handlers, AuthorizationMiddleware(roles, zap.NewNop()))
	}
	handlers = append(handlers, func(c *gin.Context) {
		c.JSON(200, gin.H{"user": c.GetUint64("userID"), "username": c.GetString("username"), "role": c.GetString("userRole")})
	})
	r.GET("/me", handlers...)
	return r
}

func get(r http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/me", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthMiddleware(t *testing.T) {
	rdb, mr := newRedis(t)
	r := authEngine(rdb)

	t.Run("valid access token", func(t *testing.T) {
		w := get(r, sign(t, claims("access", 0, time.Minute), secret))
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body)
		}
	})
	t.Run("refresh token is not accepted as access token", func(t *testing.T) {
		if w := get(r, sign(t, claims("refresh", 0, time.Minute), secret)); w.Code != 401 {
			t.Errorf("status %d, want 401", w.Code)
		}
	})
	t.Run("token without type", func(t *testing.T) {
		if w := get(r, sign(t, claims("", 0, time.Minute), secret)); w.Code != 401 {
			t.Errorf("status %d, want 401", w.Code)
		}
	})
	t.Run("expired token", func(t *testing.T) {
		if w := get(r, sign(t, claims("access", 0, -time.Minute), secret)); w.Code != 401 {
			t.Errorf("status %d, want 401", w.Code)
		}
	})
	t.Run("wrong signing key", func(t *testing.T) {
		if w := get(r, sign(t, claims("access", 0, time.Minute), "another-secret-another-secret")); w.Code != 401 {
			t.Errorf("status %d, want 401", w.Code)
		}
	})
	t.Run("alg none is rejected", func(t *testing.T) {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims("access", 0, time.Minute)).SignedString(jwt.UnsafeAllowNoneSignatureType)
		if w := get(r, tok); w.Code != 401 {
			t.Errorf("status %d, want 401", w.Code)
		}
	})
	t.Run("missing or malformed header", func(t *testing.T) {
		if w := get(r, ""); w.Code != 401 {
			t.Errorf("missing: status %d", w.Code)
		}
		req := httptest.NewRequest("GET", "/me", nil)
		req.Header.Set("Authorization", "Token abc")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Errorf("malformed: status %d", w.Code)
		}
	})
	t.Run("tokens issued before a password change are rejected", func(t *testing.T) {
		mr.Set("42:pwd_version", "3")
		if w := get(r, sign(t, claims("access", 2, time.Minute), secret)); w.Code != 401 {
			t.Errorf("old token: status %d, want 401", w.Code)
		}
		if w := get(r, sign(t, claims("access", 3, time.Minute), secret)); w.Code != 200 {
			t.Errorf("new token: status %d, want 200", w.Code)
		}
		mr.Del("42:pwd_version")
	})
	t.Run("identity is exposed to handlers", func(t *testing.T) {
		w := get(r, sign(t, claims("access", 0, time.Minute), secret))
		if got := w.Body.String(); got != `{"role":"buyer","user":42,"username":"alice"}` {
			t.Errorf("body %s", got)
		}
	})
}

func TestAuthorizationMiddleware(t *testing.T) {
	rdb, _ := newRedis(t)
	token := sign(t, claims("access", 0, time.Minute), secret)

	if w := get(authEngine(rdb, "buyer"), token); w.Code != 200 {
		t.Errorf("allowed role: status %d", w.Code)
	}
	if w := get(authEngine(rdb, "seller_admin", "seller_employee"), token); w.Code != 403 {
		t.Errorf("forbidden role: status %d, want 403", w.Code)
	}
}

func TestRateLimiting(t *testing.T) {
	rdb, mr := newRedis(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitingMiddleware("auth", 3, time.Minute, zap.NewNop(), rdb))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	call := func() int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 1; i <= 3; i++ {
		if code := call(); code != 200 {
			t.Fatalf("request %d: status %d", i, code)
		}
	}
	if code := call(); code != http.StatusTooManyRequests {
		t.Errorf("4th request: status %d, want 429", code)
	}

	// The counter always carries a TTL (no immortal keys) and is scoped
	key := "rate_limit:auth:10.0.0.1"
	if ttl := mr.TTL(key); ttl <= 0 || ttl > time.Minute {
		t.Errorf("ttl of %s = %v", key, ttl)
	}
	// After the window the client may call again
	mr.FastForward(time.Minute + time.Second)
	if code := call(); code != 200 {
		t.Errorf("after window: status %d, want 200", code)
	}
}

func TestRateLimitingFailsOpenWhenRedisIsDown(t *testing.T) {
	rdb, mr := newRedis(t)
	mr.Close()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitingMiddleware("global", 1, time.Minute, zap.NewNop(), rdb))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != 200 {
		t.Errorf("status %d, want 200 (fail open)", w.Code)
	}
}
