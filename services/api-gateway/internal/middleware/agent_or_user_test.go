package middleware

import (
	"api-gateway/internal/agent"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func agentOrUserEngine(t *testing.T) (*gin.Engine, *agent.Signer, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	_ = mr.Set(pwdSafeAfterKey, "1")
	ring, err := agent.ParseKeyRing("k1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-A")
	if err != nil {
		t.Fatal(err)
	}
	signer := &agent.Signer{Keys: ring}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pwd := NewPwdVerifier(zap.NewNop(), rdb)
	userAuth := func(c *gin.Context) { c.JSON(401, gin.H{"error": "user path"}); c.Abort() }
	e := gin.New()
	e.GET("/orders/:id", AgentOrUser(signer, agent.DefaultScopes, pwd, userAuth, zap.NewNop()), func(c *gin.Context) {
		c.JSON(200, gin.H{"user": c.Value("userID"), "role": c.Value("userRole"), "run": c.Value("agentRunID")})
	})
	e.POST("/orders/:id", AgentOrUser(signer, agent.DefaultScopes, pwd, userAuth, zap.NewNop()), func(c *gin.Context) { c.Status(200) })
	return e, signer, mr
}

func getOrder(e *gin.Engine, method, tok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/orders/7", nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestAgentOrUser(t *testing.T) {
	e, s, mr := agentOrUserEngine(t)
	tok, jti, _ := s.IssueAgentToken(42, "u", "buyer", 0, time.Minute)

	w := getOrder(e, "GET", tok)
	if w.Code != 200 || w.Body.String() != `{"role":"buyer","run":"`+jti+`","user":42}` {
		t.Fatalf("agent token on a listed route: %d %s", w.Code, w.Body.String())
	}
	if w := getOrder(e, "POST", tok); w.Code != http.StatusForbidden {
		t.Errorf("a write method is not in the table: %d, want 403", w.Code)
	}
	other, _, _ := s.IssueAgentToken(42, "u", "admin", 0, time.Minute)
	if w := getOrder(e, "GET", other); w.Code != http.StatusForbidden {
		t.Errorf("role without rows: %d, want 403", w.Code)
	}
	for name, tk := range map[string]string{"none": "", "garbage": "abc"} {
		if w := getOrder(e, "GET", tk); w.Code != 401 || w.Body.String() != `{"error":"user path"}` {
			t.Errorf("%s goes to the user path: %d %s", name, w.Code, w.Body.String())
		}
	}
	// the proposal and a token for another audience are not agent tokens
	prop, _, _ := s.SignProposal(42, "run-12345678", "POST", "/orders/7/cancel", agent.BodyHash(nil), "x", time.Minute)
	if w := getOrder(e, "GET", prop); w.Code != 401 {
		t.Errorf("a proposal must not act as an agent token: %d", w.Code)
	}

	// password changed after the token was issued (version 3 now): refused like a JWT
	_ = mr.Set("42:pwd_version", "3")
	if w := getOrder(e, "GET", tok); w.Code != 401 {
		t.Errorf("stale pwd_version: %d, want 401", w.Code)
	}
	cur, _, _ := s.IssueAgentToken(42, "u", "buyer", 3, time.Minute)
	if w := getOrder(e, "GET", cur); w.Code != 200 {
		t.Errorf("current pwd_version: %d, want 200", w.Code)
	}
}

func TestRateLimitByKeyFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	for _, closed := range []bool{true, false} {
		e := gin.New()
		e.GET("/x", RateLimitByKey("t", 5, time.Minute, zap.NewNop(), dead, UserKey, closed), func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
		want := 200
		if closed {
			want = 503
		}
		if w.Code != want {
			t.Errorf("failClosed=%v with Redis down: %d, want %d", closed, w.Code, want)
		}
	}
}
