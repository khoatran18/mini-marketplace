package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetricsMiddlewareUsesRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type obs struct {
		method, route string
		status        int
	}
	var got []obs
	r := gin.New()
	r.Use(MetricsMiddleware(func(m, route string, s int, _ time.Duration) { got = append(got, obs{m, route, s}) }))
	r.GET("/orders/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orders/42", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope/123", nil))

	if len(got) != 2 {
		t.Fatalf("want 2 observations, got %d", len(got))
	}
	if got[0] != (obs{"GET", "/orders/:id", 200}) {
		t.Fatalf("route template expected, got %+v", got[0])
	}
	if got[1].route != "" || got[1].status != 404 {
		t.Fatalf("unmatched routes must report an empty route (mapped to 'unmatched'), got %+v", got[1])
	}
}
