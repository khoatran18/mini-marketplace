package clientmanager

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryReadsOnlyRetriesReadsOnUnavailable(t *testing.T) {
	for _, tc := range []struct {
		method string
		err    error
		want   int
	}{
		{"/product.ProductService/GetProductsByIDs", status.Error(codes.Unavailable, "x"), 3},
		{"/product.ProductService/GetAndDecreaseInventoryByID", status.Error(codes.Unavailable, "x"), 1},
		{"/order.X/CreateThing", status.Error(codes.Unavailable, "x"), 1},
		{"/product.ProductService/GetProductsByIDs", status.Error(codes.NotFound, "x"), 1},
		{"/product.ProductService/GetProductsByIDs", status.Error(codes.DeadlineExceeded, "x"), 1},
		{"/product.ProductService/GetProductsByIDs", nil, 1},
	} {
		calls := 0
		inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			calls++
			return tc.err
		}
		_ = retryReads(3, time.Millisecond)(context.Background(), tc.method, nil, nil, nil, inv)
		if calls != tc.want {
			t.Errorf("%s (%v): %d calls, want %d", tc.method, tc.err, calls, tc.want)
		}
	}
}

func TestRetryReadsRecoversAndStopsOnCancel(t *testing.T) {
	calls := 0
	inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		if calls < 3 {
			return status.Error(codes.Unavailable, "restarting")
		}
		return nil
	}
	if err := retryReads(4, time.Millisecond)(context.Background(), "/p.S/GetX", nil, nil, nil, inv); err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	always := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		return status.Error(codes.Unavailable, "x")
	}
	_ = retryReads(4, time.Millisecond)(ctx, "/p.S/GetX", nil, nil, nil, always)
	if calls != 1 {
		t.Fatalf("cancelled ctx must not retry: %d", calls)
	}
}
