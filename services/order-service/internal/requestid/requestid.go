// Package requestid mang X-Request-ID qua gRPC (metadata "x-request-id") để nối log giữa các service.
package requestid

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataKey là khoá gRPC metadata mang request id.
const MetadataKey = "x-request-id"

type ctxKey struct{}

// NewContext trả ctx mang id.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext trả id trong ctx ("" nếu chưa có).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// UnaryServerInterceptor đọc id từ metadata đến (thiếu thì tạo mới), đặt vào ctx và trả lại qua header phản hồi.
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

// UnaryClientInterceptor đẩy id trong ctx vào metadata gửi đi.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := FromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// Logger thêm field request_id vào logger.
func Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if id := FromContext(ctx); id != "" {
		return base.With(zap.String("request_id", id))
	}
	return base
}
