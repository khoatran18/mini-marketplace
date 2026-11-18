package api

import (
	"net/http"
	"net/http/httptest"
	"ops-service/internal/playbooks"
	"ops-service/internal/queries"
	"strings"
	"testing"
)

// Store nil: các ca dưới đây phải bị chặn trước khi chạm DB.
func do(h http.Handler, method, path, auth string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAuthRequired(t *testing.T) {
	h := (&API{Token: "secret-token"}).Handler()
	for _, p := range []string{"/incidents", "/incidents/1", "/audit"} {
		if w := do(h, "GET", p, ""); w.Code != 401 {
			t.Errorf("%s không token: %d", p, w.Code)
		}
		if w := do(h, "GET", p, "Bearer wrong"); w.Code != 401 {
			t.Errorf("%s sai token: %d", p, w.Code)
		}
	}
	if w := do(h, "POST", "/incidents/1/investigate", ""); w.Code != 401 {
		t.Errorf("investigate không token: %d", w.Code)
	}
}

func TestDisabledWithoutToken(t *testing.T) {
	h := (&API{}).Handler()
	if w := do(h, "GET", "/incidents", ""); w.Code != 503 {
		t.Fatalf("không cấu hình token phải 503, được %d", w.Code)
	}
	if w := do(h, "GET", "/incidents", "Bearer "); w.Code != 503 {
		t.Fatalf("token rỗng không được lọt: %d", w.Code)
	}
}

func TestValidation(t *testing.T) {
	h := (&API{Token: "t"}).Handler()
	for path, want := range map[string]int{
		"/incidents?state=bogus":       400,
		"/incidents/abc":               400,
		"/incidents/0":                 400,
		"/audit?subject_type=incident": 400,
	} {
		if w := do(h, "GET", path, "Bearer t"); w.Code != want {
			t.Errorf("%s: %d, mong %d", path, w.Code, want)
		}
	}
	if w := do(h, "POST", "/incidents/abc/investigate", "Bearer t"); w.Code != 400 {
		t.Errorf("investigate id sai: %d", w.Code)
	}
}

func TestQueriesAuthAndValidation(t *testing.T) {
	h := (&API{Token: "t"}).Handler()
	if w := do(h, "GET", "/queries", ""); w.Code != 401 {
		t.Errorf("catalog không token: %d", w.Code)
	}
	if w := do(h, "GET", "/queries", "Bearer t"); w.Code != 200 {
		t.Errorf("catalog: %d", w.Code)
	}
	if w := do(h, "POST", "/queries/service_up", "Bearer t"); w.Code != 503 { // Queries nil
		t.Errorf("chưa cấu hình phải 503: %d", w.Code)
	}
	h2 := (&API{Token: "t", Queries: &queries.Runner{}}).Handler()
	if w := do(h2, "POST", "/queries/drop_all", "Bearer t"); w.Code != 400 {
		t.Errorf("truy vấn lạ phải 400: %d", w.Code)
	}
	if w := do(h2, "POST", "/queries/service_up", "Bearer t"); w.Code != 400 {
		t.Errorf("thiếu tham số phải 400: %d", w.Code)
	}
}

// Token agent (ADR-40): vào được đường đọc/đề xuất nhưng bị chặn ở mọi đường duyệt và thực thi, trước khi chạm DB.
func TestAgentTokenCannotApproveOrExecute(t *testing.T) {
	h := (&API{Token: "full-token", AgentToken: "agent-token"}).Handler()
	for _, p := range []string{"/proposals/1/review", "/proposals/1/decision", "/proposals/1/execute", "/proposals/1/executed"} {
		if w := do(h, "POST", p, "Bearer agent-token"); w.Code != 403 {
			t.Errorf("%s với token agent: %d, muốn 403", p, w.Code)
		}
		// token đầy đủ không bị chặn ở lớp này (id không hợp lệ/DB nil xử lý ở sau; chỉ cần không phải 403 do phạm vi)
		if w := do(h, "POST", "/proposals/abc/review", "Bearer full-token"); w.Code == 403 || w.Code == 401 {
			t.Errorf("token đầy đủ bị chặn nhầm: %d", w.Code)
		}
	}
	if w := do(h, "GET", "/actions", "Bearer agent-token"); w.Code != 200 {
		t.Errorf("agent đọc /actions: %d", w.Code)
	}
	if w := do(h, "GET", "/incidents?state=bogus", "Bearer agent-token"); w.Code != 400 {
		t.Errorf("agent phải vượt auth (400 do state sai), được %d", w.Code)
	}
	if w := do(h, "GET", "/actions", "Bearer nope"); w.Code != 401 {
		t.Errorf("sai token: %d", w.Code)
	}
}

func TestAgentActorIsForcedAndIgnoresHeader(t *testing.T) {
	var got string
	h := (&API{Token: "full-token", AgentToken: "agent-token"}).auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = actor(r) }))
	for _, c := range []struct{ token, header, want string }{
		{"agent-token", "admin:1", "agent"}, {"full-token", "admin:7", "admin:7"}, {"full-token", "", "unknown"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer "+c.token)
		if c.header != "" {
			r.Header.Set("X-Actor", c.header)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got != c.want {
			t.Errorf("token %s header %q: actor %q, muốn %q", c.token, c.header, got, c.want)
		}
	}
}

func TestPlaybooksEndpoints(t *testing.T) {
	lib, err := playbooks.Load("../../../../docs/playbooks")
	if err != nil {
		t.Fatal(err)
	}
	h := (&API{Token: "t", AgentToken: "agent", Playbooks: lib}).Handler()
	if w := do(h, "GET", "/playbooks", ""); w.Code != 401 {
		t.Fatalf("playbooks không token: %d", w.Code)
	}
	w := do(h, "GET", "/playbooks?alert=OutboxDeadRows", "Bearer agent") // token phạm vi agent đọc được
	if w.Code != 200 || !strings.Contains(w.Body.String(), "outbox-stuck") || strings.Contains(w.Body.String(), "service-unavailable") {
		t.Fatalf("lọc theo alert: %d %s", w.Code, w.Body.String())
	}
	w = do(h, "GET", "/playbooks/outbox-stuck", "Bearer agent")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "poison-row") || strings.Contains(w.Body.String(), "ground_truth") {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	for path, want := range map[string]int{"/playbooks/nope": 404, "/playbooks/..%2Fetc": 400, "/playbooks/Bad_ID": 400} {
		if w := do(h, "GET", path, "Bearer agent"); w.Code != want {
			t.Errorf("%s: %d want %d", path, w.Code, want)
		}
	}
	if w := do((&API{Token: "t"}).Handler(), "GET", "/playbooks", "Bearer t"); w.Code != 200 || !strings.Contains(w.Body.String(), `"playbooks":[]`) {
		t.Fatalf("thiếu thư viện = danh sách rỗng: %d %s", w.Code, w.Body.String())
	}
}

// POST /audit/events (gateway, kill switch): only the full token, a closed set of events, validated before the DB is touched.
func TestAIKillSwitchAuditValidationAndScope(t *testing.T) {
	h := (&API{Token: "full-token", AgentToken: "agent-token"}).Handler()
	post := func(token, body string) int {
		r := httptest.NewRequest("POST", "/audit/events", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := post("agent-token", `{"paused":true}`); c != 403 {
		t.Errorf("a token with agent scope must not write audit events: %d", c)
	}
	if c := post("nope", `{"paused":true}`); c != 401 {
		t.Errorf("wrong token: %d", c)
	}
	for name, body := range map[string]string{"missing paused": `{}`, "not json": `x`, "reason too long": `{"paused":true,"reason":"` + strings.Repeat("x", 201) + `"}`} {
		if c := post("full-token", body); c != 400 {
			t.Errorf("%s: %d, want 400", name, c)
		}
	}
}
