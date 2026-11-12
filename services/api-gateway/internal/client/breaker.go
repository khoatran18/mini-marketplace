package client

import (
	"api-gateway/pkg/ops"
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BreakerPolicy cấu hình circuit breaker mỗi service đích (ADR-36).
type BreakerPolicy struct {
	Threshold int           // số lần lỗi hạ tầng LIÊN TIẾP thì mở mạch (CIRCUIT_FAILURE_THRESHOLD, mặc định 5; 0 = tắt)
	Cooldown  time.Duration // mở bao lâu trước khi thử lại một lần (CIRCUIT_COOLDOWN_MS, mặc định 10000)
}

func loadBreakerPolicy() BreakerPolicy {
	th := envPositiveInt("CIRCUIT_FAILURE_THRESHOLD", 5)
	if v := envPositiveInt("CIRCUIT_DISABLED", 0); v == 1 {
		th = 0
	}
	return BreakerPolicy{Threshold: th, Cooldown: time.Duration(envPositiveInt("CIRCUIT_COOLDOWN_MS", 10000)) * time.Millisecond}
}

const (
	circuitClosed   = 0.0
	circuitOpen     = 1.0
	circuitHalfOpen = 2.0
)

// Breaker: đóng -> (N lỗi liên tiếp) -> mở (từ chối ngay, không gọi) -> sau Cooldown nửa mở (cho ĐÚNG MỘT lời gọi thử)
// -> thành công thì đóng, lỗi thì mở lại. Chỉ lỗi hạ tầng (Unavailable, DeadlineExceeded) được tính; lỗi nghiệp vụ
// (NotFound, InvalidArgument, ...) chứng tỏ service vẫn trả lời nên coi như thành công.
type Breaker struct {
	service string
	p       BreakerPolicy
	now     func() time.Time

	mu       sync.Mutex
	fails    int
	state    float64
	openedAt time.Time
	probing  bool
}

// NewBreaker tạo breaker cho một service đích.
func NewBreaker(service string, p BreakerPolicy) *Breaker {
	ops.SetCircuitState(service, circuitClosed) // series exists from the start, so dashboards and alerts see 0
	return &Breaker{service: service, p: p, now: time.Now}
}

func infraFailure(err error) bool {
	c := status.Code(err)
	return c == codes.Unavailable || c == codes.DeadlineExceeded
}

func (b *Breaker) set(state float64) {
	b.state = state
	ops.SetCircuitState(b.service, state)
}

// allow báo có được gửi lời gọi không; probe = lời gọi này là lần thử của trạng thái nửa mở.
func (b *Breaker) allow() (ok, probe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case circuitOpen:
		if b.now().Sub(b.openedAt) < b.p.Cooldown {
			return false, false
		}
		b.set(circuitHalfOpen)
		b.probing = true
		return true, true
	case circuitHalfOpen:
		if b.probing {
			return false, false
		}
		b.probing = true
		return true, true
	}
	return true, false
}

func (b *Breaker) record(err error, probe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if probe {
		b.probing = false
	}
	if err == nil || !infraFailure(err) {
		b.fails = 0
		if b.state != circuitClosed {
			b.set(circuitClosed)
		}
		return
	}
	b.fails++
	if probe || b.fails >= b.p.Threshold {
		b.openedAt = b.now()
		b.set(circuitOpen)
	}
}

// BreakerInterceptor từ chối ngay (codes.Unavailable, gateway trả 503) khi mạch đang mở thay vì để mỗi request chờ hết
// deadline. Đặt ngoài RetryInterceptor: một lời gọi logic (gồm các lần retry) được tính một lần.
func BreakerInterceptor(b *Breaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if b.p.Threshold <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		ok, probe := b.allow()
		if !ok {
			ops.IncCircuitRejected(b.service)
			return status.Error(codes.Unavailable, "circuit open: "+b.service)
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		// ctx của chính client bị huỷ (người dùng bỏ đi) không nói gì về sức khoẻ service
		if ctx.Err() == context.Canceled {
			b.mu.Lock()
			if probe {
				b.probing = false
			}
			b.mu.Unlock()
			return err
		}
		b.record(err, probe)
		return err
	}
}
