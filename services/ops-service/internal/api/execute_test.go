package api

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakeRunner struct {
	calls []string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, id string, p map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, fmt.Sprint(id, p["service"]))
	return map[string]any{"requeued": 2}, f.err
}

func (e *env) withExec(enabled bool, r Runner) {
	e.h = (&API{Store: e.st, Token: "tok", Now: func() time.Time { return e.now }, ActionsEnabled: enabled, Runner: r}).Handler()
}

// accepted tạo đề xuất bởi admin:99 và để admin:1 chấp nhận.
func (e *env) accepted(t *testing.T, action string, params map[string]any) int {
	t.Helper()
	p := e.propose(t, map[string]any{"action_id": action, "params": params, "rationale": "r"})
	id := int(p["id"].(float64))
	tok := e.review(t, p["id"].(float64), "admin:1")
	code, out := e.call("POST", fmt.Sprintf("/proposals/%d/decision", id), "admin:1", map[string]any{"decision": "accept", "approval_token": tok, "reason": "đã đọc last_error"})
	if code != 200 {
		t.Fatalf("accept: %d %v", code, out)
	}
	return id
}

func TestExecuteDisabledByDefault(t *testing.T) {
	e := newEnv(t)
	fr := &fakeRunner{}
	e.withExec(false, fr)
	id := e.accepted(t, "requeue_outbox", map[string]any{"service": "order-service", "limit": 5})
	if code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/execute", id), "admin:2", nil); code != 403 || len(fr.calls) != 0 {
		t.Fatalf("công tắc tắt phải 403 và không chạy: %d %v", code, fr.calls)
	}
}

func TestExecuteHTTPAction(t *testing.T) {
	e := newEnv(t)
	fr := &fakeRunner{}
	e.withExec(true, fr)
	id := e.accepted(t, "requeue_outbox", map[string]any{"service": "order-service", "limit": 5})
	path := fmt.Sprintf("/proposals/%d/execute", id)
	if code, _ := e.call("POST", path, "admin:99", nil); code != 403 {
		t.Fatalf("người tạo không được tự thực thi: %d", code)
	}
	code, out := e.call("POST", path, "admin:2", nil)
	if code != 200 || out["exec_state"] != "EXECUTED" || len(fr.calls) != 1 {
		t.Fatalf("%d %v %v", code, out, fr.calls)
	}
	if code, _ := e.call("POST", path, "admin:2", nil); code != 409 || len(fr.calls) != 1 {
		t.Fatalf("không được chạy hai lần: %d %v", code, fr.calls)
	}
	_, got := e.call("GET", fmt.Sprintf("/proposals/%d", id), "admin:2", nil)
	trail := fmt.Sprint(got["audit"])
	if !contains(trail, "proposal_execution_started") || !contains(trail, "proposal_execution_succeeded") {
		t.Fatalf("audit thiếu: %s", trail)
	}
}

func TestExecuteFailureIsRecorded(t *testing.T) {
	e := newEnv(t)
	fr := &fakeRunner{err: errors.New("boom")}
	e.withExec(true, fr)
	id := e.accepted(t, "requeue_outbox", map[string]any{"service": "order-service", "limit": 5})
	code, out := e.call("POST", fmt.Sprintf("/proposals/%d/execute", id), "admin:2", nil)
	if code != 422 || out["exec_state"] != "FAILED" {
		t.Fatalf("%d %v", code, out)
	}
	if code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/execute", id), "admin:2", nil); code != 409 {
		t.Fatalf("FAILED không tự chạy lại: %d", code)
	}
}

func TestCooldownAndStaleApproval(t *testing.T) {
	e := newEnv(t)
	fr := &fakeRunner{}
	e.withExec(true, fr)
	a := e.accepted(t, "requeue_outbox", map[string]any{"service": "order-service", "limit": 5})
	b := e.accepted(t, "requeue_outbox", map[string]any{"service": "order-service", "limit": 6})
	if code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/execute", a), "admin:2", nil); code != 200 {
		t.Fatal("lần đầu phải chạy")
	}
	if code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/execute", b), "admin:2", nil); code != 429 {
		t.Fatalf("cùng hành động + service trong cooldown phải 429: %d", code)
	}
	e.now = e.now.Add(2 * time.Hour) // quyết định duyệt đã cũ hơn cửa sổ 1 giờ
	if code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/execute", b), "admin:2", nil); code != 409 {
		t.Fatalf("duyệt quá cũ phải 409: %d", code)
	}
}

func TestCommandModeIsHumanRecorded(t *testing.T) {
	e := newEnv(t)
	e.withExec(true, &fakeRunner{})
	id := e.accepted(t, "restart_service", map[string]any{"service": "order-service"})
	code, out := e.call("GET", fmt.Sprintf("/proposals/%d", id), "admin:1", nil)
	if code != 200 || out["command"] != "docker service update --force --detach=true marketplace_order-service" {
		t.Fatalf("command: %v", out["command"])
	}
	if code, _ := e.call("POST", fmt.Sprintf("/proposals/%d/execute", id), "admin:2", nil); code != 409 {
		t.Fatalf("mode=command không tự chạy: %d", code)
	}
	path := fmt.Sprintf("/proposals/%d/executed", id)
	if code, _ := e.call("POST", path, "admin:99", map[string]any{"outcome": "success"}); code != 403 {
		t.Fatalf("người tạo không tự ghi kết quả: %d", code)
	}
	if code, _ := e.call("POST", path, "admin:1", map[string]any{"outcome": "maybe"}); code != 400 {
		t.Fatalf("outcome sai: %d", code)
	}
	code, out = e.call("POST", path, "admin:1", map[string]any{"outcome": "success", "note": "đã chạy"})
	if code != 200 || out["proposal"].(map[string]any)["exec_state"] != "EXECUTED" {
		t.Fatalf("%d %v", code, out)
	}
	if code, _ := e.call("POST", path, "admin:1", map[string]any{"outcome": "success"}); code != 409 {
		t.Fatalf("ghi hai lần: %d", code)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
