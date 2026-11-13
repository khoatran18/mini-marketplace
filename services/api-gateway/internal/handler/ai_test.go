package handler

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func aiEngine(h *AIHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.POST("/ai/chat", func(c *gin.Context) { c.Set("userID", uint64(7)); c.Set("userRole", "buyer"); c.Next() }, h.Chat)
	return e
}

func newAI(t *testing.T, up *httptest.Server, max time.Duration) *AIHandler {
	t.Helper()
	url := ""
	if up != nil {
		url = up.URL
	}
	return NewAIHandler(true, url, max, zap.NewNop())
}

func TestAIChatDisabledAndUnconfigured(t *testing.T) {
	off := aiEngine(NewAIHandler(false, "http://x", time.Second, zap.NewNop()))
	w := httptest.NewRecorder()
	off.ServeHTTP(w, httptest.NewRequest("POST", "/ai/chat", strings.NewReader(`{}`)))
	if w.Code != 404 {
		t.Errorf("flag off: %d, want 404", w.Code)
	}
	noURL := aiEngine(NewAIHandler(true, "", time.Second, zap.NewNop()))
	w = httptest.NewRecorder()
	noURL.ServeHTTP(w, httptest.NewRequest("POST", "/ai/chat", strings.NewReader(`{}`)))
	if w.Code != 503 {
		t.Errorf("no upstream: %d, want 503", w.Code)
	}
}

// The core requirement: chunks reach the client as the upstream produces them (no buffering in the gateway), and identity
// is forwarded from the verified JWT, not from the request.
func TestAIChatStreamsWithoutBufferingAndForwardsIdentity(t *testing.T) {
	release := make(chan struct{})
	var gotAuth, gotUser, gotRole, gotBody, gotAccept string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotAuth, gotUser, gotRole, gotBody, gotAccept = r.Header.Get("Authorization"), r.Header.Get("X-User-ID"), r.Header.Get("X-User-Role"), string(b), r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: one\n\n"))
		w.(http.Flusher).Flush()
		<-release // hold the second event until the client has seen the first
		_, _ = w.Write([]byte("data: two\n\n"))
	}))
	defer up.Close()
	gw := httptest.NewServer(aiEngine(newAI(t, up, 10*time.Second)))
	defer gw.Close()

	req, _ := http.NewRequest("POST", gw.URL+"/ai/chat", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-User-Role", "admin") // a spoofed header must not reach the upstream
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" || !strings.Contains(resp.Header.Get("Cache-Control"), "no-cache") {
		t.Errorf("missing anti-buffering headers: %v", resp.Header)
	}
	rd := bufio.NewReader(resp.Body)
	first := make(chan string, 1)
	go func() { l, _ := rd.ReadString('\n'); first <- l }()
	select {
	case l := <-first:
		if l != "data: one\n" {
			t.Fatalf("first line %q", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first event not delivered while the upstream is still open: the gateway is buffering")
	}
	close(release)
	rest, _ := io.ReadAll(rd)
	if !strings.Contains(string(rest), "data: two") {
		t.Errorf("second event missing: %q", rest)
	}
	if gotAuth != "Bearer tok" || gotUser != "7" || gotRole != "buyer" || gotBody != `{"message":"hi"}` || gotAccept != "text/event-stream" {
		t.Errorf("upstream saw auth=%q user=%q role=%q body=%q accept=%q", gotAuth, gotUser, gotRole, gotBody, gotAccept)
	}
}

// Closing the browser tab must stop the upstream call (the model call costs money).
func TestAIChatClientDisconnectCancelsUpstream(t *testing.T) {
	cancelled := make(chan struct{})
	started := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(": hello\n\n"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer up.Close()
	gw := httptest.NewServer(aiEngine(newAI(t, up, 30*time.Second)))
	defer gw.Close()
	ctx, stop := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", gw.URL+"/ai/chat", strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	stop()
	resp.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream was not cancelled after the client disconnected")
	}
}

func TestAIChatStreamTimeLimitEmitsErrorEvent(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: start\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer up.Close()
	gw := httptest.NewServer(aiEngine(newAI(t, up, 300*time.Millisecond)))
	defer gw.Close()
	resp, err := http.Post(gw.URL+"/ai/chat", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "data: start") || !strings.Contains(string(b), "event: error") || !strings.Contains(string(b), "time limit") {
		t.Errorf("body %q", b)
	}
}

func TestAIChatUpstreamErrorsAndLimits(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer up.Close()
	e := aiEngine(newAI(t, up, time.Second))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("POST", "/ai/chat", strings.NewReader(`{}`)))
	if w.Code != 429 || !strings.Contains(w.Body.String(), "busy") {
		t.Errorf("non-stream upstream reply must pass through: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("POST", "/ai/chat", strings.NewReader(strings.Repeat("x", 70<<10))))
	if w.Code != 413 {
		t.Errorf("oversize: %d, want 413", w.Code)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	e2 := aiEngine(NewAIHandler(true, dead.URL, time.Second, zap.NewNop()))
	w = httptest.NewRecorder()
	e2.ServeHTTP(w, httptest.NewRequest("POST", "/ai/chat", strings.NewReader(`{}`)))
	if w.Code != 503 {
		t.Errorf("upstream down: %d, want 503", w.Code)
	}
}

func TestFeaturesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/config/features", FeaturesHandler{AIChat: true}.Get)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", "/config/features", nil))
	if w.Code != 200 || w.Body.String() != `{"features":{"ai_chat":true,"ai_ops":false}}` || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("%d %s %v", w.Code, w.Body.String(), w.Header())
	}
}
