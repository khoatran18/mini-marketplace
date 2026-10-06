package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func bodyLimitEngine(max int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimitMiddleware(max))
	r.POST("/echo", func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read failed"})
			return
		}
		c.String(http.StatusOK, "%d", len(b))
	})
	return r
}

func TestBodyLimitAllowsSmallBodies(t *testing.T) {
	w := httptest.NewRecorder()
	bodyLimitEngine(10).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("0123456789")))
	if w.Code != 200 || w.Body.String() != "10" {
		t.Fatalf("body at the limit must pass: %d %s", w.Code, w.Body.String())
	}
}

func TestBodyLimitRejectsDeclaredLargeBodiesWith413(t *testing.T) {
	w := httptest.NewRecorder()
	bodyLimitEngine(10).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("0123456789X")))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", w.Code)
	}
}

func TestBodyLimitCapsUndeclaredBodies(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/echo", io.NopCloser(strings.NewReader(strings.Repeat("x", 100))))
	req.ContentLength = -1 // chunked: length unknown up front
	w := httptest.NewRecorder()
	bodyLimitEngine(10).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("reading beyond the cap must fail, got %d", w.Code)
	}
}
