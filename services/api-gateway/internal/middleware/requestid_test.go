package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/requestid"

	"github.com/gin-gonic/gin"
)

func ridRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/x", func(c *gin.Context) { c.String(200, requestid.FromContext(c.Request.Context())) })
	return r
}

func TestRequestIDKeepsValidHeader(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", "client-id-12345")
	ridRouter().ServeHTTP(w, req)
	if w.Header().Get("X-Request-ID") != "client-id-12345" || w.Body.String() != "client-id-12345" {
		t.Fatalf("header=%q body=%q", w.Header().Get("X-Request-ID"), w.Body.String())
	}
}

func TestRequestIDGeneratesWhenMissingOrInvalid(t *testing.T) {
	for _, in := range []string{"", "short", "bad id with spaces!!"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if in != "" {
			req.Header.Set("X-Request-ID", in)
		}
		ridRouter().ServeHTTP(w, req)
		got := w.Header().Get("X-Request-ID")
		if got == in || !requestid.Valid(got) || w.Body.String() != got {
			t.Fatalf("in=%q got=%q body=%q", in, got, w.Body.String())
		}
	}
}
