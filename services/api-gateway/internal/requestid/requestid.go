// Package requestid mang X-Request-ID xuyên suốt: HTTP header -> context -> gRPC metadata `x-request-id`.
// Tự chứa, copy giống nhau ở mỗi service (không dùng module chung).
package requestid

import (
	"context"
	"regexp"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// HeaderName là tên HTTP header.
	HeaderName = "X-Request-ID"
	// MetadataKey là khoá gRPC metadata (luôn viết thường).
	MetadataKey = "x-request-id"
)

type ctxKey struct{}

var validID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// Valid báo id có đúng định dạng được chấp nhận hay không.
func Valid(id string) bool { return validID.MatchString(id) }

// New tạo id mới (UUID v4).
func New() string { return uuid.NewString() }

// FromContext trả id trong ctx ("" nếu không có).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// NewContext gắn id vào ctx.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// Logger trả logger có thêm field request_id (nếu ctx có id).
func Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if id := FromContext(ctx); id != "" {
		return base.With(zap.String("request_id", id))
	}
	return base
}

// UnaryServerInterceptor đọc id từ metadata (thiếu/không hợp lệ thì tạo mới), đặt vào ctx và trả lại qua header gRPC.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := ""
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get(MetadataKey); len(v) > 0 && Valid(v[0]) {
				id = v[0]
			}
		}
		if id == "" {
			id = New()
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs(MetadataKey, id))
		return handler(NewContext(ctx, id), req)
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
