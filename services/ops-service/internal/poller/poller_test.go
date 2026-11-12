package poller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const body = `{"status":"success","data":{"alerts":[
 {"labels":{"alertname":"ServiceDown","swarm_service":"payment-service","severity":"critical"},"annotations":{"action":"low","runbook":"§1"},"state":"firing"},
 {"labels":{"alertname":"OutboxDeadRows","service":"order-service"},"annotations":{"action":"high"},"state":"pending"}]}}`

func TestFetchKeepsOnlyFiring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	p := &Poller{URL: srv.URL, Interval: time.Second}
	got, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "ServiceDown" || got[0].Annotations["action"] != "low" {
		t.Fatalf("got %+v", got)
	}
}

func TestFetchErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"http 500": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"bad json": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{")) },
		"status":   func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"status":"error"}`)) },
	} {
		srv := httptest.NewServer(h)
		p := &Poller{URL: srv.URL, Interval: time.Second}
		if _, err := p.Fetch(context.Background()); err == nil {
			t.Errorf("%s: phải lỗi", name)
		}
		srv.Close()
	}
}

func TestOnceFailureDoesNotResolve(t *testing.T) {
	// Lỗi đọc Prometheus phải tăng Failures và không chạm store (Store nil sẽ panic nếu bị gọi).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	p := &Poller{URL: srv.URL, Interval: time.Second}
	if err := p.Once(context.Background()); err == nil || p.Failures.Load() != 1 || p.LastSuccess.Load() != 0 {
		t.Fatalf("err=%v failures=%d", err, p.Failures.Load())
	}
}
