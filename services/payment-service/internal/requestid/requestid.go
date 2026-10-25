// Package requestid mang mã yêu cầu (request-id) xuyên suốt HTTP -> gRPC giữa các service.
// Header HTTP là X-Request-ID, khoá metadata gRPC là x-request-id. Bản sao giống hệt nhau nằm trong
// mỗi service (không dùng module chung, xem ADR-8).
package requestid

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataKey là khoá metadata gRPC; HTTPHeader là tên header HTTP tương ứng.
const (
	MetadataKey = "x-request-id"
	HTTPHeader  = "X-Request-ID"
)

type ctxKey struct{}

// NewContext trả về ctx mang id.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext trả về id trong ctx ("" nếu chưa có).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Logger thêm field request_id vào logger (giữ nguyên nếu ctx chưa có id).
func Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		return zap.NewNop()
	}
	if id := FromContext(ctx); id != "" {
		return base.With(zap.String("request_id", id))
	}
	return base
}

// UnaryServerInterceptor đọc id từ metadata đến (thiếu thì tạo mới), đặt vào ctx và trả lại qua header gRPC.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
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
		return h(ctx, req)
	}
}

// UnaryClientInterceptor đẩy id từ ctx vào metadata đi ra (nếu có).
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := FromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
