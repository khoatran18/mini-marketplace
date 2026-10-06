package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimitMiddleware rejects request bodies larger than max bytes. Requests that declare a
// Content-Length above the limit get 413 immediately; for chunked/undeclared bodies the reader is
// capped, so reading beyond max fails (handlers answer 400) and the excess is never buffered.
func BodyLimitMiddleware(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > max {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
