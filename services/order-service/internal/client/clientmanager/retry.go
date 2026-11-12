package clientmanager

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Cùng quy ước với gateway (internal/client/interceptors.go): chỉ method ĐỌC mới được retry, mặc định là ghi.
var readPrefixes = []string{"Get", "List", "Search", "Preview", "Query", "Count", "Validate"}

// Tên bắt đầu bằng tiền tố đọc nhưng có tác dụng phụ.
var writeOverrides = map[string]bool{"GetAndDecreaseInventoryByID": true}

func isRead(fullMethod string) bool {
	name := fullMethod[strings.LastIndex(fullMethod, "/")+1:]
	if writeOverrides[name] {
		return false
	}
	for _, p := range readPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// retryReads thử lại lời gọi đọc khi gặp codes.Unavailable (service đích vừa khởi động lại, kênh chưa nối xong):
// tối đa maxAttempts lần gồm lần đầu, backoff base, 2×base... có jitter. Không retry lời gọi ghi, DeadlineExceeded,
// hay khi ctx đã hết hạn / bị huỷ.
func retryReads(maxAttempts int, base time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if !isRead(method) {
			return err
		}
		backoff := base
		for attempt := 1; attempt < maxAttempts; attempt++ {
			if err == nil || status.Code(err) != codes.Unavailable || ctx.Err() != nil {
				return err
			}
			t := time.NewTimer(backoff/2 + time.Duration(rand.Int64N(int64(backoff/2)+1)))
			select {
			case <-ctx.Done():
				t.Stop()
				return err
			case <-t.C:
			}
			err = invoker(ctx, method, req, reply, cc, opts...)
			backoff *= 2
		}
		return err
	}
}
