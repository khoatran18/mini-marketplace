package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ops-service/internal/store"
)

type fake struct{ rows []store.AuditEntry }

func (f *fake) MaxAuditID(context.Context) (int64, error) { return 5, nil }
func (f *fake) AuditSince(_ context.Context, after int64, _ []string, _ int) ([]store.AuditEntry, error) {
	var out []store.AuditEntry
	for _, r := range f.rows {
		if r.ID > after {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestTickSendsOnlyNewAndRetriesOnFailure(t *testing.T) {
	var bodies []string
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if fail {
			w.WriteHeader(500)
			return
		}
		bodies = append(bodies, string(b))
	}))
	defer srv.Close()
	f := &fake{rows: []store.AuditEntry{
		{ID: 4, Event: "incident_opened", SubjectID: "1", Details: json.RawMessage(`{"alert":"OLD"}`)},
		{ID: 6, Event: "incident_opened", SubjectID: "2", Details: json.RawMessage(`{"alert":"ServiceDown","dedupe_key":"swarm_service=order-service"}`)},
		{ID: 7, Event: "proposal_created", SubjectID: "9", Actor: "agent:triage"},
	}}
	n := &Notifier{Source: f, URL: srv.URL}
	ctx := context.Background()
	if err := n.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if sent, err := n.Tick(ctx); err == nil || sent != 0 {
		t.Fatalf("lỗi webhook phải trả lỗi: sent=%d err=%v", sent, err)
	}
	fail = false
	sent, err := n.Tick(ctx)
	if err != nil || sent != 2 {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], "ServiceDown") || strings.Contains(strings.Join(bodies, ""), "OLD") || !strings.Contains(bodies[1], "agent:triage") {
		t.Fatalf("nội dung: %v", bodies)
	}
	if sent, _ := n.Tick(ctx); sent != 0 {
		t.Fatal("không được gửi lại")
	}
}
