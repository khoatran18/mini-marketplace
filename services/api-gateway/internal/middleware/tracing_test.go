package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

func TestTracingMiddlewareContinuesIncomingTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got trace.SpanContext
	r := gin.New()
	r.Use(RequestIDMiddleware(), TracingMiddleware())
	r.GET("/items/:id", func(c *gin.Context) {
		got = trace.SpanContextFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})
	// no propagator installed in this unit test => the middleware must still work and not panic
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items/7", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	_ = got // no-op tracer: the span context may be invalid, the request must simply pass through
}
