package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
)

// MetricsMiddleware reports every request to observe using the matched route template
// (c.FullPath(), e.g. /orders/:id) so metric label cardinality stays bounded.
func MetricsMiddleware(observe func(method, route string, status int, d time.Duration)) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		observe(c.Request.Method, c.FullPath(), c.Writer.Status(), time.Since(start))
	}
}
