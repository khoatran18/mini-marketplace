package client

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func callWith(b *Breaker, err error, calls *int) error {
	inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		*calls++
		return err
	}
	return BreakerInterceptor(b)(context.Background(), "/s/GetThing", nil, nil, nil, inv)
}

func newTestBreaker(th int) (*Breaker, *time.Time) {
	b := NewBreaker("svc", BreakerPolicy{Threshold: th, Cooldown: 10 * time.Second})
	now := time.Unix(1000, 0)
	b.now = func() time.Time { return now }
	return b, &now
}

func TestBreakerOpensAfterConsecutiveInfraFailuresAndFailsFast(t *testing.T) {
	b, _ := newTestBreaker(3)
	calls := 0
	down := status.Error(codes.Unavailable, "down")
	for i := 0; i < 3; i++ {
		_ = callWith(b, down, &calls)
	}
	if calls != 3 || b.state != circuitOpen {
		t.Fatalf("calls=%d state=%v", calls, b.state)
	}
	err := callWith(b, nil, &calls)
	if status.Code(err) != codes.Unavailable || calls != 3 {
		t.Fatalf("open circuit must reject without calling: err=%v calls=%d", err, calls)
	}
}

func TestBreakerBusinessErrorsDoNotCountAndSuccessResets(t *testing.T) {
	b, _ := newTestBreaker(3)
	calls := 0
	for i := 0; i < 10; i++ {
		_ = callWith(b, status.Error(codes.NotFound, "x"), &calls)
	}
	if b.state != circuitClosed {
		t.Fatalf("business errors opened the circuit")
	}
	down := status.Error(codes.DeadlineExceeded, "slow")
	_ = callWith(b, down, &calls)
	_ = callWith(b, down, &calls)
	_ = callWith(b, nil, &calls) // success resets the streak
	_ = callWith(b, down, &calls)
	_ = callWith(b, down, &calls)
	if b.state != circuitClosed {
		t.Fatal("non-consecutive failures opened the circuit")
	}
}

func TestBreakerHalfOpenProbe(t *testing.T) {
	b, now := newTestBreaker(2)
	calls := 0
	down := status.Error(codes.Unavailable, "down")
	_ = callWith(b, down, &calls)
	_ = callWith(b, down, &calls)
	*now = now.Add(11 * time.Second) // cooldown over
	// probe fails -> open again, and the cooldown restarts
	_ = callWith(b, down, &calls)
	if b.state != circuitOpen || calls != 3 {
		t.Fatalf("state=%v calls=%d", b.state, calls)
	}
	if err := callWith(b, nil, &calls); status.Code(err) != codes.Unavailable || calls != 3 {
		t.Fatalf("must be rejected again: %v calls=%d", err, calls)
	}
	*now = now.Add(11 * time.Second)
	// probe succeeds -> closed
	if err := callWith(b, nil, &calls); err != nil || b.state != circuitClosed || calls != 4 {
		t.Fatalf("err=%v state=%v calls=%d", err, b.state, calls)
	}
}

func TestBreakerOnlyOneProbeAtATime(t *testing.T) {
	b, now := newTestBreaker(1)
	calls := 0
	_ = callWith(b, status.Error(codes.Unavailable, "down"), &calls)
	*now = now.Add(11 * time.Second)
	ok1, probe1 := b.allow()
	ok2, _ := b.allow()
	if !ok1 || !probe1 || ok2 {
		t.Fatalf("first=%v/%v second=%v: only one probe may pass", ok1, probe1, ok2)
	}
}

func TestBreakerDisabledWhenThresholdZero(t *testing.T) {
	b, _ := newTestBreaker(0)
	calls := 0
	for i := 0; i < 20; i++ {
		_ = callWith(b, status.Error(codes.Unavailable, "down"), &calls)
	}
	if calls != 20 {
		t.Fatalf("calls=%d", calls)
	}
}
