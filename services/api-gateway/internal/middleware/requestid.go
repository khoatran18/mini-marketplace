package middleware

import (
	"api-gateway/internal/requestid"

	"github.com/gin-gonic/gin"
)

// RequestIDMiddleware nhận X-Request-ID hợp lệ (không thì tạo UUID v4), trả lại trong response header,
// lưu vào gin context ("requestID") và vào c.Request.Context() để interceptor gRPC client đẩy sang metadata.
// Không phải auth middleware nên được phép gắn bằng router.Use (đặt đầu chuỗi).
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestid.HeaderName)
		if !requestid.Valid(id) {
			id = requestid.New()
		}
		c.Set("requestID", id)
		c.Header(requestid.HeaderName, id)
		c.Request = c.Request.WithContext(requestid.NewContext(c.Request.Context(), id))
		c.Next()
	}
}
