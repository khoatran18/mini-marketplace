package middleware

import (
	"api-gateway/internal/requestid"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"mini-marketplace/shared/tracing"
)

// TracingMiddleware mở span gốc cho mỗi request HTTP (ADR-36) và đưa nó vào c.Request.Context(), nên các span gRPC client
// phía sau nằm cùng một trace. Tên span là "METHOD route-mẫu" (không chứa id => số lượng tên bị chặn).
// Không phải auth middleware nên được phép router.Use; đặt SAU RequestIDMiddleware để gắn được request_id.
// Khi tracing tắt (không có OTEL_EXPORTER_OTLP_ENDPOINT) span là no-op, chi phí gần bằng 0.
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := tracing.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := tracing.Tracer().Start(ctx, c.Request.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		if id := requestid.FromContext(ctx); id != "" {
			span.SetAttributes(attribute.String("request_id", id))
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		code := c.Writer.Status()
		span.SetName(c.Request.Method + " " + route)
		span.SetAttributes(attribute.String("http.request.method", c.Request.Method), attribute.String("http.route", route), attribute.Int("http.response.status_code", code))
		if code >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(code))
		}
	}
}
