// Package requestid chuyển request-id qua gRPC metadata (key `x-request-id`) và ctx.
package requestid

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataKey là key gRPC metadata mang request-id (HTTP header tương ứng: X-Request-ID).
const MetadataKey = "x-request-id"

type ctxKey struct{}

// NewContext trả ctx mới chứa request-id.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext lấy request-id từ ctx ("" nếu chưa có).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Logger thêm field request_id vào logger (nil-safe: base nil -> zap.NewNop).
func Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	if id := FromContext(ctx); id != "" {
		return base.With(zap.String("request_id", id))
	}
	return base
}

// UnaryServerInterceptor đọc request-id từ metadata (thiếu thì tạo UUID v4), đặt vào ctx
// và trả lại qua header response `x-request-id`.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := ""
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get(MetadataKey); len(v) > 0 {
				id = v[0]
			}
		}
		if id == "" {
			id = uuid.NewString()
		}
		ctx = NewContext(ctx, id)
		_ = grpc.SetHeader(ctx, metadata.Pairs(MetadataKey, id))
		return handler(ctx, req)
	}
}

// UnaryClientInterceptor đẩy request-id từ ctx vào metadata đi ra (nếu có).
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := FromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
