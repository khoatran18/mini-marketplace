package client

import (
	"api-gateway/pkg/ops"
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CallPolicy là deadline/retry mặc định cho lời gọi gRPC từ gateway.
type CallPolicy struct {
	ReadTimeout  time.Duration // GRPC_TIMEOUT_READ_MS (mặc định 3000)
	WriteTimeout time.Duration // GRPC_TIMEOUT_WRITE_MS (mặc định 10000)
	MaxAttempts  int           // GRPC_RETRY_MAX_ATTEMPTS, tổng số lần gọi gồm lần đầu (mặc định 3)
	BaseBackoff  time.Duration // 100ms, nhân đôi mỗi lần, có jitter
}

func envPositiveInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func loadCallPolicy() CallPolicy {
	return CallPolicy{
		ReadTimeout:  time.Duration(envPositiveInt("GRPC_TIMEOUT_READ_MS", 3000)) * time.Millisecond,
		WriteTimeout: time.Duration(envPositiveInt("GRPC_TIMEOUT_WRITE_MS", 10000)) * time.Millisecond,
		MaxAttempts:  envPositiveInt("GRPC_RETRY_MAX_ATTEMPTS", 3),
		BaseBackoff:  100 * time.Millisecond,
	}
}

// Không dùng tiền tố "Check": nó khớp nhầm Checkout (ghi). Proto hiện chưa có method Check*/Validate*.
var readPrefixes = []string{"Get", "List", "Search", "Preview", "Query", "Count", "Validate"}

// writeOverrides: tên bắt đầu bằng tiền tố đọc nhưng thực tế có tác dụng phụ => luôn là ghi, không retry.
var writeOverrides = map[string]bool{
	"GetAndDecreaseInventoryByID": true, // trừ tồn kho
}

// IsReadMethod phân loại method gRPC đầy đủ ("/pkg.Service/Name") là đọc (an toàn để retry) hay ghi.
// Mặc định là ghi: tên không nhận ra thì coi là ghi để không bao giờ retry nhầm lệnh có tác dụng phụ.
// Login/Register/RefreshToken/IngestEvents/MergeCart... đều là ghi.
func IsReadMethod(fullMethod string) bool {
	name := fullMethod
	if i := strings.LastIndex(fullMethod, "/"); i >= 0 {
		name = fullMethod[i+1:]
	}
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

// DeadlineInterceptor đặt deadline mặc định theo loại method; nếu ctx đã có deadline sớm hơn thì giữ nguyên.
func DeadlineInterceptor(p CallPolicy) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		d := p.WriteTimeout
		if IsReadMethod(method) {
			d = p.ReadTimeout
		}
		if d > 0 {
			if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > d {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, d)
				defer cancel()
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// RetryInterceptor retry CHỈ method đọc khi gặp codes.Unavailable (backoff mũ 100ms, 200ms... + jitter).
// Method ghi KHÔNG retry: không biết request đã được server xử lý hay chưa nên retry có thể nhân đôi tác dụng phụ
// (tạo order/payment, trừ kho...). POST /orders có Idempotency-Key nhưng việc retry ghi để dành đợt sau.
// DeadlineExceeded không retry; ngừng ngay khi ctx cha hết hạn/bị huỷ.
func RetryInterceptor(service string, p CallPolicy) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if !IsReadMethod(method) {
			return err
		}
		backoff := p.BaseBackoff
		for attempt := 1; attempt < p.MaxAttempts; attempt++ {
			if err == nil || status.Code(err) != codes.Unavailable || ctx.Err() != nil {
				return err
			}
			// jitter: ngủ trong [backoff/2, backoff)
			sleep := backoff/2 + time.Duration(rand.Int64N(int64(backoff/2)+1))
			t := time.NewTimer(sleep)
			select {
			case <-ctx.Done():
				t.Stop()
				return err
			case <-t.C:
			}
			ops.IncClientRetry(service, method)
			err = invoker(ctx, method, req, reply, cc, opts...)
			backoff *= 2
		}
		return err
	}
}
