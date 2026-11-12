package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"ops-service/internal/migrate"
	"ops-service/internal/store"
)

type env struct {
	h   http.Handler
	st  *store.Store
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("ops_api_%d", rand.Int63())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); admin.Close() })
	if err := migrate.Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	e := &env{st: &store.Store{DB: db}, now: time.Now()}
	e.h = (&API{Store: e.st, Token: "tok", Now: func() time.Time { return e.now }}).Handler()
	return e
}

func (e *env) call(method, path, actor string, body any) (int, map[string]any) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rd)
	r.Header.Set("Authorization", "Bearer tok")
	r.Header.Set("X-Actor", actor)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *env) propose(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	code, out := e.call("POST", "/proposals", "admin:99", body)
	if code != 201 {
		t.Fatalf("tạo đề xuất: %d %v", code, out)
	}
	return out["proposal"].(map[string]any)
}

func (e *env) review(t *testing.T, id float64, actor string) string {
	t.Helper()
	code, out := e.call("POST", fmt.Sprintf("/proposals/%d/review", int(id)), actor, nil)
	if code != 200 {
		t.Fatalf("review: %d %v", code, out)
	}
	return out["approval_token"].(string)
}

func TestFullAcceptFlow(t *testing.T) {
	e := newEnv(t)
	p := e.propose(t, map[string]any{"action_id": "restart_service", "params": map[string]any{"service": "order-service"}, "evidence": []string{"mm_ready=0"}, "rationale": "treo"})
	if p["risk"] != "low" || p["state"] != "PENDING" || p["created_by"] != "admin:99" {
		t.Fatalf("%v", p)
	}
	id := p["id"].(float64)
	tok := e.review(t, id, "admin:1")
	code, out := e.call("POST", fmt.Sprintf("/proposals/%d/decision", int(id)), "admin:1", map[string]any{"decision": "accept", "approval_token": tok})
	if code != 200 || out["proposal"].(map[string]any)["state"] != "ACCEPTED" {
		t.Fatalf("%d %v", code, out)
	}
	code, out = e.call("GET", fmt.Sprintf("/proposals/%d", int(id)), "admin:1", nil)
	if code != 200 || len(out["audit"].([]any)) != 3 {
		t.Fatalf("%d audit=%v", code, out["audit"])
	}
}

func TestRiskComesFromCatalogNotFromCaller(t *testing.T) {
	e := newEnv(t)
	// người tạo khai risk=observe cho hành động high: trường lạ bị từ chối ngay
	code, _ := e.call("POST", "/proposals", "admin:99", map[string]any{"action_id": "requeue_outbox", "risk": "observe", "params": map[string]any{"service": "order-service", "limit": 5}})
	if code != 400 {
		t.Fatalf("trường risk lạ phải 400, được %d", code)
	}
	p := e.propose(t, map[string]any{"action_id": "requeue_outbox", "params": map[string]any{"service": "order-service", "limit": 5}})
	if p["risk"] != "high" {
		t.Fatalf("risk phải lấy từ sổ: %v", p["risk"])
	}
}

func TestOutsideCatalogAndBadParams(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]map[string]any{
		"ngoài sổ":     {"action_id": "docker_exec", "params": map[string]any{"cmd": "rm -rf /"}},
		"scale về 0":   {"action_id": "scale_service", "params": map[string]any{"service": "order-service", "replicas": 0}},
		"restart db":   {"action_id": "restart_service", "params": map[string]any{"service": "postgres"}},
		"thiếu action": {"params": map[string]any{}},
	} {
		if code, out := e.call("POST", "/proposals", "admin:99", body); code != 400 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if code, _ := e.call("POST", "/proposals", "admin:99", map[string]any{"action_id": "escalate_to_human", "evidence": make([]string, 21)}); code != 400 {
		t.Errorf("evidence quá nhiều: %d", code)
	}
	var n int
	e.st.DB.QueryRow(`SELECT count(*) FROM ops_proposals`).Scan(&n)
	if n != 0 {
		t.Fatalf("đề xuất bị từ chối vẫn được lưu: %d", n)
	}
}

func TestDecisionNeedsReviewTokenFromSamePerson(t *testing.T) {
	e := newEnv(t)
	p := e.propose(t, map[string]any{"action_id": "restart_service", "params": map[string]any{"service": "order-service"}})
	id := int(p["id"].(float64))
	path := fmt.Sprintf("/proposals/%d/decision", id)
	// không mã, mã rác
	for _, tok := range []string{"", "abc.def"} {
		if code, _ := e.call("POST", path, "admin:1", map[string]any{"decision": "accept", "approval_token": tok}); code != 403 {
			t.Errorf("mã %q phải 403, được %d", tok, code)
		}
	}
	// mã của admin:1 không dùng được bởi admin:2
	tok := e.review(t, p["id"].(float64), "admin:1")
	if code, _ := e.call("POST", path, "admin:2", map[string]any{"decision": "accept", "approval_token": tok}); code != 403 {
		t.Errorf("mã của người khác phải 403, được %d", code)
	}
	// mã hết hạn (5 phút)
	e.now = e.now.Add(6 * time.Minute)
	if code, _ := e.call("POST", path, "admin:1", map[string]any{"decision": "accept", "approval_token": tok}); code != 403 {
		t.Errorf("mã hết hạn phải 403, được %d", code)
	}
	// người tạo (admin:99) xem rồi vẫn không tự duyệt được
	tok99 := e.review(t, p["id"].(float64), "admin:99")
	if code, _ := e.call("POST", path, "admin:99", map[string]any{"decision": "accept", "approval_token": tok99}); code != 403 {
		t.Errorf("tự duyệt phải 403, được %d", code)
	}
	if _, out := e.call("GET", fmt.Sprintf("/proposals/%d", id), "admin:1", nil); out["proposal"].(map[string]any)["state"] != "PENDING" {
		t.Fatal("đề xuất phải còn PENDING")
	}
}

func TestReasonRules(t *testing.T) {
	e := newEnv(t)
	low := e.propose(t, map[string]any{"action_id": "restart_service", "params": map[string]any{"service": "order-service"}})
	high := e.propose(t, map[string]any{"action_id": "requeue_outbox", "params": map[string]any{"service": "order-service", "limit": 5}})
	d := func(p map[string]any, body map[string]any) int {
		body["approval_token"] = e.review(t, p["id"].(float64), "admin:1")
		code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/decision", int(p["id"].(float64))), "admin:1", body)
		return code
	}
	if c := d(high, map[string]any{"decision": "accept"}); c != 400 {
		t.Errorf("accept hành động high thiếu lý do: %d, muốn 400", c)
	}
	if c := d(low, map[string]any{"decision": "reject"}); c != 400 {
		t.Errorf("reject thiếu lý do: %d, muốn 400", c)
	}
	if c := d(low, map[string]any{"decision": "accept"}); c != 200 {
		t.Errorf("accept hành động low không cần lý do: %d", c)
	}
	if c := d(high, map[string]any{"decision": "accept", "reason": "đã đọc last_error"}); c != 200 {
		t.Errorf("accept high có lý do: %d", c)
	}
}

func TestAlternativeFlow(t *testing.T) {
	e := newEnv(t)
	p := e.propose(t, map[string]any{"action_id": "restart_service", "params": map[string]any{"service": "order-service"}})
	path := fmt.Sprintf("/proposals/%d/decision", int(p["id"].(float64)))
	body := func(alt map[string]any) map[string]any {
		return map[string]any{"decision": "alternative", "reason": "đổi cách", "alternative": alt, "approval_token": e.review(t, p["id"].(float64), "admin:1")}
	}
	// thay thế ngoài sổ, giống hệt gốc: bị chặn
	if c, _ := e.call("POST", path, "admin:1", body(map[string]any{"action_id": "drop", "params": map[string]any{}})); c != 400 {
		t.Errorf("ngoài sổ: %d", c)
	}
	if c, _ := e.call("POST", path, "admin:1", body(map[string]any{"action_id": "restart_service", "params": map[string]any{"service": "order-service"}})); c != 400 {
		t.Errorf("giống hệt gốc: %d", c)
	}
	c, out := e.call("POST", path, "admin:1", body(map[string]any{"action_id": "scale_service", "params": map[string]any{"service": "order-service", "replicas": 2}}))
	got := out["proposal"].(map[string]any)
	if c != 200 || got["state"] != "ALTERNATIVE" || got["chosen_action_id"] != "scale_service" || got["action_id"] != "restart_service" {
		t.Fatalf("%d %v", c, out)
	}
}

func TestStateRulesAndListing(t *testing.T) {
	e := newEnv(t)
	p := e.propose(t, map[string]any{"action_id": "escalate_to_human", "rationale": "không khớp playbook"})
	id := int(p["id"].(float64))
	// trùng nội dung
	if c, _ := e.call("POST", "/proposals", "admin:99", map[string]any{"action_id": "escalate_to_human", "rationale": "lần hai"}); c != 409 {
		t.Errorf("trùng phải 409, được %d", c)
	}
	tok := e.review(t, p["id"].(float64), "admin:1")
	e.call("POST", fmt.Sprintf("/proposals/%d/decision", id), "admin:1", map[string]any{"decision": "reject", "reason": "không cần", "approval_token": tok})
	// đã quyết thì không review / quyết lại
	if c, _ := e.call("POST", fmt.Sprintf("/proposals/%d/review", id), "admin:1", nil); c != 409 {
		t.Errorf("review sau quyết định phải 409, được %d", c)
	}
	if c, _ := e.call("POST", fmt.Sprintf("/proposals/%d/decision", id), "admin:1", map[string]any{"decision": "accept", "approval_token": tok}); c != 409 {
		t.Errorf("quyết lại phải 409, được %d", c)
	}
	if c, out := e.call("GET", "/proposals?state=rejected", "admin:1", nil); c != 200 || len(out["proposals"].([]any)) != 1 {
		t.Errorf("list rejected: %d %v", c, out)
	}
	if c, _ := e.call("GET", "/proposals?state=bogus", "admin:1", nil); c != 400 {
		t.Errorf("state sai: %d", c)
	}
	if c, _ := e.call("GET", "/proposals/9999", "admin:1", nil); c != 404 {
		t.Errorf("không có: %d", c)
	}
	// hết hạn
	p2 := e.propose(t, map[string]any{"action_id": "restart_service", "params": map[string]any{"service": "order-service"}})
	e.now = e.now.Add(25 * time.Hour)
	if c, _ := e.call("POST", fmt.Sprintf("/proposals/%d/review", int(p2["id"].(float64))), "admin:1", nil); c != 409 {
		t.Errorf("review sau hạn: %d", c)
	}
}

func TestBodyLimitAndCatalogEndpoint(t *testing.T) {
	e := newEnv(t)
	big := strings.Repeat("x", 70<<10)
	if c, _ := e.call("POST", "/proposals", "admin:99", map[string]any{"action_id": "escalate_to_human", "rationale": big}); c != 400 {
		t.Errorf("body quá lớn: %d", c)
	}
	c, out := e.call("GET", "/actions", "admin:1", nil)
	if c != 200 || len(out["actions"].([]any)) != 5 {
		t.Errorf("catalog: %d %v", c, out)
	}
}
