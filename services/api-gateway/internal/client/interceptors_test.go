package client

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var testPolicy = CallPolicy{ReadTimeout: 3 * time.Second, WriteTimeout: 10 * time.Second, MaxAttempts: 3, BaseBackoff: time.Millisecond}

func TestIsReadMethod(t *testing.T) {
	cases := map[string]bool{
		"/product.ProductService/GetProductByID":              true,
		"/product.ProductService/SearchProducts":              true,
		"/order.OrderService/ListOrders":                      true,
		"/order.OrderService/PreviewCheckout":                 true,
		"/analytics.AnalyticsService/Query":                   true,
		"/product.ProductService/GetAndDecreaseInventoryByID": false,
		"/order.OrderService/Checkout":                        false,
		"/auth.AuthService/Login":                             false,
		"/analytics.AnalyticsService/IngestEvents":            false,
		"/order.OrderService/MergeCart":                       false,
	}
	for m, want := range cases {
		if got := IsReadMethod(m); got != want {
			t.Errorf("%s: got %v want %v", m, got, want)
		}
	}
}

func TestDeadlineDefaultsByKind(t *testing.T) {
	check := func(method string, want time.Duration) {
		var got time.Duration
		inv := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			dl, ok := ctx.Deadline()
			if !ok {
				t.Fatal("no deadline")
			}
			got = time.Until(dl)
			return nil
		}
		_ = DeadlineInterceptor(testPolicy)(context.Background(), method, nil, nil, nil, inv)
		if got > want || got < want-time.Second {
			t.Errorf("%s: deadline %v want ~%v", method, got, want)
		}
	}
	check("/s/GetThing", 3*time.Second)
	check("/s/CreateThing", 10*time.Second)
}

func TestDeadlineKeepsSmallerParent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	inv := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		dl, _ := ctx.Deadline()
		if time.Until(dl) > 600*time.Millisecond {
			t.Errorf("deadline widened: %v", time.Until(dl))
		}
		return nil
	}
	_ = DeadlineInterceptor(testPolicy)(ctx, "/s/GetThing", nil, nil, nil, inv)
}

func TestRetryReadUnavailableThenOK(t *testing.T) {
	calls := 0
	inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		if calls <= 2 {
			return status.Error(codes.Unavailable, "down")
		}
		return nil
	}
	if err := RetryInterceptor("t", testPolicy)(context.Background(), "/s/GetThing", nil, nil, nil, inv); err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestRetryGivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		return status.Error(codes.Unavailable, "down")
	}
	err := RetryInterceptor("t", testPolicy)(context.Background(), "/s/ListThings", nil, nil, nil, inv)
	if status.Code(err) != codes.Unavailable || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestNoRetryForWriteOrOtherCodes(t *testing.T) {
	for _, tc := range []struct {
		method string
		code   codes.Code
	}{{"/s/CreateThing", codes.Unavailable}, {"/s/GetThing", codes.DeadlineExceeded}, {"/s/GetThing", codes.NotFound}} {
		calls := 0
		inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			calls++
			return status.Error(tc.code, "x")
		}
		_ = RetryInterceptor("t", testPolicy)(context.Background(), tc.method, nil, nil, nil, inv)
		if calls != 1 {
			t.Errorf("%s/%v: calls=%d", tc.method, tc.code, calls)
		}
	}
}

func TestNoRetryWhenParentCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	inv := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		cancel()
		return status.Error(codes.Unavailable, "down")
	}
	_ = RetryInterceptor("t", testPolicy)(ctx, "/s/GetThing", nil, nil, nil, inv)
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
