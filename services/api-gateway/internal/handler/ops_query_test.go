package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GET /ops/queries/:name is a read for the agent; upstream it must become ops-service's POST /queries/:name with only the
// known params in the body, the shared token, and the caller's identity for the audit log.
func TestOpsQueryTurnsTheQueryStringIntoTheUpstreamBody(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotAuth, gotActor string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotBody, gotAuth, gotActor = r.Method, r.URL.Path, string(b), r.Header.Get("Authorization"), r.Header.Get("X-Actor")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer up.Close()

	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/ops/queries/:name", func(c *gin.Context) { c.Set("userRole", "dev"); c.Set("userID", uint64(7)) }, NewOpsHandler(up.URL, "tok", zap.NewNop()).Query())

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", "/ops/queries/grpc_latency_p95?window=15m&service=order-service&evil=1", nil))
	if w.Code != 200 || gotMethod != "POST" || gotPath != "/queries/grpc_latency_p95" {
		t.Fatalf("%d %s %s", w.Code, gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"window":"15m"`) || !strings.Contains(gotBody, `"service":"order-service"`) || strings.Contains(gotBody, "evil") {
		t.Errorf("body %s", gotBody)
	}
	if gotAuth != "Bearer tok" || gotActor != "dev:7" {
		t.Errorf("auth=%q actor=%q", gotAuth, gotActor)
	}

	for _, bad := range []string{"/ops/queries/Bad-Name", "/ops/queries/a;b"} {
		w = httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", bad, nil))
		if w.Code != 400 {
			t.Errorf("%s: %d, want 400", bad, w.Code)
		}
	}
}
