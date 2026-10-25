// Package requestid truyền X-Request-ID qua gRPC (metadata key "x-request-id").
package requestid

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataKey là key metadata gRPC (và tên header trả về).
const MetadataKey = "x-request-id"

type ctxKey struct{}

// NewContext gắn request id vào ctx.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext trả về request id trong ctx ("" nếu chưa có).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Logger trả về logger kèm field request_id (giữ nguyên base nếu ctx không có id).
func Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		return zap.NewNop()
	}
	if id := FromContext(ctx); id != "" {
		return base.With(zap.String("request_id", id))
	}
	return base
}

// UnaryServerInterceptor đọc id từ metadata đến (thiếu thì tạo UUID v4), đặt vào ctx
// và gửi lại qua header phản hồi.
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

// UnaryClientInterceptor đẩy id từ ctx vào metadata đi ra.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := FromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
