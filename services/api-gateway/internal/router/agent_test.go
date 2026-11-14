package router

import (
	"api-gateway/internal/agent"
	"api-gateway/internal/config"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const agentKeys = "k1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-A"

func agentEngine(t *testing.T) (*gin.Engine, *agent.Signer) {
	t.Helper()
	ring, err := agent.ParseKeyRing(agentKeys)
	if err != nil {
		t.Fatal(err)
	}
	return newEngineEnv(t, func(e *config.EnvConfig) { e.AgentKeys = agentKeys }), &agent.Signer{Keys: ring}
}

func do(e *gin.Engine, method, path, tok, proposal, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if proposal != "" {
		req.Header.Set("X-Agent-Proposal", proposal)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

// propose asks POST /ai/proposals as the agent of user 1 and returns the signed proposal.
func propose(t *testing.T, e *gin.Engine, s *agent.Signer, uid uint64, method, path, body string) string {
	t.Helper()
	return proposeAs(t, e, s, "buyer", uid, method, path, body)
}

func proposeAs(t *testing.T, e *gin.Engine, s *agent.Signer, role string, uid uint64, method, path, body string) string {
	t.Helper()
	at, _, err := s.IssueAgentToken(uid, "u", role, 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"method":"` + method + `","path":"` + path + `","summary":"Huỷ đơn","body":` + body + `}`
	w := do(e, "POST", "/ai/proposals", at, "", payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("propose: %d %s", w.Code, w.Body.String())
	}
	var out struct{ Proposal string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Proposal
}

func TestProposalEndpointNeedsAnAgentToken(t *testing.T) {
	e, s := agentEngine(t)
	if w := do(e, "POST", "/ai/proposals", "", "", `{}`); w.Code != 401 {
		t.Errorf("no token: %d", w.Code)
	}
	if w := do(e, "POST", "/ai/proposals", token(t, "buyer"), "", `{}`); w.Code != 401 {
		t.Errorf("a user JWT must not create proposals: %d", w.Code)
	}
	at, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	for _, p := range []string{
		`{"method":"POST","path":"/orders","summary":"x"}`,             // not on the list
		`{"method":"DELETE","path":"/admin/accounts/3","summary":"x"}`, // not on the list
		`{"method":"POST","path":"/orders/5/cancel","summary":""}`,     // no summary
	} {
		if w := do(e, "POST", "/ai/proposals", at, "", p); w.Code == http.StatusCreated {
			t.Errorf("must refuse %s", p)
		}
	}
	if w := do(newEngine(t), "POST", "/ai/proposals", at, "", `{}`); w.Code != 503 {
		t.Errorf("without keys the endpoint answers 503, got %d", w.Code)
	}
}

func TestAgentTokenCannotCallNormalRoutes(t *testing.T) {
	e, s := agentEngine(t)
	at, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	for _, r := range [][2]string{
		{"POST", "/orders/5/cancel"}, {"POST", "/orders"}, {"DELETE", "/cart"}, {"POST", "/auth/change-password"},
		{"GET", "/admin/accounts"}, {"GET", "/users/me/addresses"}, {"GET", "/payments/5"}, {"POST", "/seller/orders/5/ship"},
		{"POST", "/ai/chat"},
	} {
		if w := do(e, r[0], r[1], at, "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with an agent token: %d, want 401 (not signed with JWT_SECRET)", r[0], r[1], w.Code)
		}
	}
}

// pathFor fills the :params of a route pattern.
func pathFor(p string) string {
	parts := strings.Split(p, "/")
	for i, x := range parts {
		if strings.HasPrefix(x, ":") {
			parts[i] = "1"
		}
	}
	return strings.Join(parts, "/")
}

func TestScopeTableRowsAreRealRoutesAndAcceptTheAgentToken(t *testing.T) {
	e, s := agentEngine(t)
	have := map[string]bool{}
	for _, r := range e.Routes() {
		have[r.Method+" "+r.Path] = true
	}
	for role, tools := range agent.DefaultScopes {
		at, _, _ := s.IssueAgentToken(1, "u", role, 0, time.Minute)
		for _, tool := range tools {
			if tool.Method != "GET" {
				t.Errorf("%s: agent tools are read-only, %s is %s", role, tool.Name, tool.Method)
			}
			if !have[tool.Method+" "+tool.Path] {
				t.Errorf("tool %s points at %s %s, which is not a route", tool.Name, tool.Method, tool.Path)
				continue
			}
			// the gRPC backends are absent in tests, so the handler may fail later; the gate itself must not answer 401/403
			if w := do(e, tool.Method, pathFor(tool.Path), at, "", ""); w.Code == 401 || w.Code == 403 {
				t.Errorf("tool %s (%s %s) refused its own role: %d %s", tool.Name, tool.Method, tool.Path, w.Code, w.Body.String())
			}
		}
	}
}

func TestAgentTokenOfAnotherRoleIsRefusedOnScopedRoutes(t *testing.T) {
	e, s := agentEngine(t)
	for _, role := range []string{"seller_admin", "seller_employee", "admin", "operator", "nobody"} {
		at, _, _ := s.IssueAgentToken(1, "u", role, 0, time.Minute)
		for _, p := range []string{"/cart", "/orders", "/orders/1", "/orders/summary"} {
			if w := do(e, "GET", p, at, "", ""); w.Code != http.StatusForbidden {
				t.Errorf("GET %s with a %s agent token: %d, want 403", p, role, w.Code)
			}
		}
	}
	// a buyer agent token has no seller rows; a seller one has no buyer rows
	buyerTok, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	for _, p := range []string{"/seller/orders", "/seller/products", "/seller/analytics/summary", "/seller/inventory/low-stock"} {
		if w := do(e, "GET", p, buyerTok, "", ""); w.Code != http.StatusForbidden {
			t.Errorf("GET %s with a buyer agent token: %d, want 403", p, w.Code)
		}
	}
	// writes on the same paths are not wired for the agent at all
	at, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	if w := do(e, "PUT", "/cart/items/1", at, "", `{}`); w.Code != http.StatusUnauthorized {
		t.Errorf("PUT /cart/items/1 with an agent token: %d, want 401", w.Code)
	}
}

func TestUserJWTStillWorksOnTheScopedRoutes(t *testing.T) {
	e, _ := agentEngine(t)
	for _, p := range []string{"/cart", "/orders", "/orders/1", "/orders/summary", "/search", "/products/1"} {
		if w := do(e, "GET", p, token(t, "buyer"), "", ""); w.Code == 401 || w.Code == 403 {
			t.Errorf("GET %s with a buyer JWT: %d", p, w.Code)
		}
	}
	if w := do(e, "GET", "/orders", token(t, "admin"), "", ""); w.Code != http.StatusForbidden {
		t.Errorf("GET /orders as admin: %d, want 403 (role check unchanged)", w.Code)
	}
	if w := do(e, "GET", "/orders", "garbage", "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("garbage token: %d, want 401", w.Code)
	}
}

func TestToolsEndpoint(t *testing.T) {
	e, s := agentEngine(t)
	if w := do(e, "GET", "/ai/tools", "", "", ""); w.Code != 401 {
		t.Errorf("no token: %d", w.Code)
	}
	if w := do(e, "GET", "/ai/tools", token(t, "buyer"), "", ""); w.Code != 401 {
		t.Errorf("a user JWT must not read the tool list: %d", w.Code)
	}
	at, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	w := do(e, "GET", "/ai/tools", at, "", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var out struct {
		Role  string
		Tools []agent.Tool
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Role != "buyer" || len(out.Tools) != len(agent.DefaultScopes["buyer"]) {
		t.Errorf("unexpected list: %+v", out)
	}
	// a role with no row gets an empty list (never null), no access
	at2, _, _ := s.IssueAgentToken(1, "u", "nobody", 0, time.Minute)
	w = do(e, "GET", "/ai/tools", at2, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"tools":[]`) {
		t.Errorf("unknown role: %d %s", w.Code, w.Body.String())
	}
	// admin and operator get read tools and no propose_ actions
	for _, role := range []string{"admin", "operator"} {
		at3, _, _ := s.IssueAgentToken(1, "u", role, 0, time.Minute)
		w = do(e, "GET", "/ai/tools", at3, "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"actions":[]`) || strings.Contains(w.Body.String(), `"tools":[]`) {
			t.Errorf("%s: want tools and no actions, got %d", role, w.Code)
		}
	}
}

// operator sees the system only; admin sees business data too; nobody reaches accounts, and /ops writes stay out of reach.
func TestAdminAndDevAgentScopes(t *testing.T) {
	e, s := agentEngine(t)
	tok := func(role string) string { at, _, _ := s.IssueAgentToken(1, "u", role, 0, time.Minute); return at }
	for _, c := range []struct {
		role, method, path string
		allowed            bool
	}{
		{"admin", "GET", "/admin/orders", true}, {"admin", "GET", "/admin/payments/1", true}, {"admin", "GET", "/ops/incidents", true},
		{"operator", "GET", "/ops/incidents", true}, {"operator", "GET", "/ops/system/health", true}, {"operator", "GET", "/ops/queries/gateway_latency_p95", true},
		{"admin", "GET", "/ops/proposals", true}, {"operator", "GET", "/ops/proposals/1", true}, {"operator", "POST", "/ops/queries/gateway_latency_p95", false},
		{"operator", "POST", "/ops/proposals", false}, {"admin", "POST", "/ops/proposals/1/decision", false}, {"operator", "GET", "/admin/analytics/data-health", true},
		{"operator", "GET", "/admin/orders", false}, {"operator", "GET", "/admin/analytics/summary", false}, {"operator", "GET", "/admin/payments", false},
		{"admin", "GET", "/admin/accounts", false}, {"operator", "GET", "/admin/accounts", false},
		{"admin", "POST", "/ops/incidents/1/investigate", false}, {"operator", "POST", "/ops/proposals/1/execute", false},
		{"admin", "POST", "/admin/orders/1/cancel", false}, {"admin", "POST", "/admin/payments/1/refund", false},
		{"admin", "GET", "/orders", false}, {"admin", "GET", "/seller/orders", false},
	} {
		w := do(e, c.method, c.path, tok(c.role), "", "")
		refused := w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden
		if c.allowed && refused {
			t.Errorf("%s %s %s: refused (%d), want allowed", c.role, c.method, c.path, w.Code)
		}
		if !c.allowed && !refused {
			t.Errorf("%s %s %s: %d, want refused", c.role, c.method, c.path, w.Code)
		}
	}
	// the read-only roles cannot even get a proposal signed
	for _, role := range []string{"admin", "operator"} {
		w := do(e, "POST", "/ai/proposals", tok(role), "", `{"method":"POST","path":"/admin/orders/5/cancel","summary":"x","body":null}`)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s proposal: %d, want 403", role, w.Code)
		}
	}
}

func TestAgentRoutesAreRateLimitedPerUser(t *testing.T) {
	e := newEngineEnv(t, func(c *config.EnvConfig) { c.AgentKeys = agentKeys; c.AIProposalRateLimit = 2 })
	ring, _ := agent.ParseKeyRing(agentKeys)
	s := &agent.Signer{Keys: ring}
	a1, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	a2, _, _ := s.IssueAgentToken(2, "v", "buyer", 0, time.Minute)
	for i, want := range []int{200, 200, 429} {
		if w := do(e, "GET", "/ai/tools", a1, "", ""); w.Code != want {
			t.Errorf("call %d: %d, want %d", i+1, w.Code, want)
		}
	}
	if w := do(e, "GET", "/ai/tools", a2, "", ""); w.Code != 200 {
		t.Errorf("another user has their own counter: %d", w.Code)
	}
}

func TestConfirmProposalGate(t *testing.T) {
	e, s := agentEngine(t)
	const path, body = "/orders/5/cancel", `{"reason":"changed mind"}`
	user := token(t, "buyer") // user id 1

	// match: passes the gate (the gRPC backend is absent in tests, so the handler itself fails later: not a gate status)
	p := propose(t, e, s, 1, "POST", path, body)
	w := do(e, "POST", path, user, p, `{ "reason": "changed mind" }`) // different layout, same JSON
	if w.Code == 401 || w.Code == 403 || w.Code == 409 {
		t.Fatalf("a matching proposal must pass the gate, got %d %s", w.Code, w.Body.String())
	}
	// single use
	if w := do(e, "POST", path, user, p, body); w.Code != http.StatusConflict {
		t.Errorf("second use: %d, want 409", w.Code)
	}
	// tampered body, other path, other user's proposal, garbage, wrong method are all refused
	p2 := propose(t, e, s, 1, "POST", path, body)
	if w := do(e, "POST", path, user, p2, `{"reason":"other"}`); w.Code != 403 {
		t.Errorf("changed body: %d, want 403", w.Code)
	}
	p3 := propose(t, e, s, 1, "POST", path, body)
	if w := do(e, "POST", "/orders/6/cancel", user, p3, body); w.Code != 403 {
		t.Errorf("other order: %d, want 403", w.Code)
	}
	pOther := propose(t, e, s, 2, "POST", path, body)
	if w := do(e, "POST", path, user, pOther, body); w.Code != 403 {
		t.Errorf("proposal of another user: %d, want 403", w.Code)
	}
	if w := do(e, "POST", path, user, "garbage", body); w.Code != 401 {
		t.Errorf("garbage proposal: %d, want 401", w.Code)
	}
	// an agent token is not a proposal
	at, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	if w := do(e, "POST", path, user, at, body); w.Code != 401 {
		t.Errorf("agent token as proposal: %d, want 401", w.Code)
	}
	// the gate runs after auth/role: a seller cannot use a buyer route even with a valid proposal
	p4 := propose(t, e, s, 1, "POST", path, body)
	if w := do(e, "POST", path, token(t, "seller_admin"), p4, body); w.Code != 403 || !strings.Contains(w.Body.String(), "permission") {
		t.Errorf("role check must still apply: %d %s", w.Code, w.Body.String())
	}
}

// Every route the agent may propose must really carry the confirm gate: a proposal for body A replayed with body B is refused
// (403) before the handler runs. A route listed as proposable but missing `confirm` would answer something else here.
func TestEveryProposableRouteHasTheConfirmGate(t *testing.T) {
	e, s := agentEngine(t)
	for _, c := range []struct{ role, path string }{
		{"buyer", "/orders/5/cancel"}, {"buyer", "/orders/5/return"},
		{"seller_admin", "/seller/orders/5/cancel"}, {"seller_admin", "/seller/orders/5/return/approve"},
		{"seller_admin", "/seller/orders/5/return/reject"}, {"seller_admin", "/seller/products/5/inventory/adjust"},
	} {
		p := proposeAs(t, e, s, c.role, 1, "POST", c.path, `{"n":1}`)
		if w := do(e, "POST", c.path, token(t, c.role), p, `{"n":2}`); w.Code != http.StatusForbidden {
			t.Errorf("%s as %s: tampered body got %d, want 403 (is `confirm` attached?)", c.path, c.role, w.Code)
		}
	}
}

func TestProposalIsRefusedForAnActionOfAnotherRole(t *testing.T) {
	e, s := agentEngine(t)
	buyer, _, _ := s.IssueAgentToken(1, "u", "buyer", 0, time.Minute)
	seller, _, _ := s.IssueAgentToken(2, "u", "seller_admin", 0, time.Minute)
	for _, c := range []struct{ tok, path string }{
		{buyer, "/seller/orders/5/cancel"}, {buyer, "/admin/orders/5/cancel"}, {seller, "/orders/5/cancel"},
	} {
		w := do(e, "POST", "/ai/proposals", c.tok, "", `{"method":"POST","path":"`+c.path+`","summary":"x","body":null}`)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403", c.path, w.Code)
		}
	}
	if w := do(e, "POST", "/ai/proposals", seller, "", `{"method":"POST","path":"/seller/orders/5/cancel","summary":"Huỷ đơn","body":null}`); w.Code != http.StatusCreated {
		t.Errorf("a seller's own action: %d %s", w.Code, w.Body.String())
	}
}
