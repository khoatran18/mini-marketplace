// Package requestid chuyển X-Request-ID qua gRPC (metadata "x-request-id") và ctx để log các service
// nối được với nhau. Bản sao giống hệt nhau nằm trong từng service (không dùng module chung).
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

// NewContext gắn id vào ctx.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext trả về id trong ctx ("" nếu chưa có).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Logger trả về logger kèm field request_id (giữ nguyên base nếu ctx chưa có id).
func Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	if id := FromContext(ctx); id != "" {
		return base.With(zap.String("request_id", id))
	}
	return base
}

// UnaryServerInterceptor đọc id từ metadata đến (thiếu thì tạo mới), đặt vào ctx và trả lại qua header.
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

// UnaryClientInterceptor đẩy id từ ctx vào metadata gửi đi (không ghi đè nếu đã có).
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := FromContext(ctx); id != "" {
			if md, ok := metadata.FromOutgoingContext(ctx); !ok || len(md.Get(MetadataKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
