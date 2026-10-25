package requestid

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func serve(t *testing.T, in context.Context) string {
	t.Helper()
	var got string
	_, err := UnaryServerInterceptor()(in, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		got = FromContext(ctx)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestServerKeepsExistingID(t *testing.T) {
	in := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "abc-12345678"))
	if got := serve(t, in); got != "abc-12345678" {
		t.Errorf("got %q", got)
	}
}

func TestServerGeneratesWhenMissing(t *testing.T) {
	a, b := serve(t, context.Background()), serve(t, context.Background())
	if len(a) != 36 || a == b {
		t.Errorf("expected distinct uuids, got %q %q", a, b)
	}
}

func TestClientToServerRoundtrip(t *testing.T) {
	ctx := NewContext(context.Background(), "rid-roundtrip-1")
	var out metadata.MD
	err := UnaryClientInterceptor()(ctx, "/x/y", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		out, _ = metadata.FromOutgoingContext(ctx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	in := metadata.NewIncomingContext(context.Background(), out)
	if got := serve(t, in); got != "rid-roundtrip-1" {
		t.Errorf("got %q", got)
	}
}

func TestClientNoIDNoMetadata(t *testing.T) {
	var out metadata.MD
	_ = UnaryClientInterceptor()(context.Background(), "/x/y", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		out, _ = metadata.FromOutgoingContext(ctx)
		return nil
	})
	if len(out.Get(MetadataKey)) != 0 {
		t.Errorf("unexpected metadata %v", out)
	}
}

func TestLogger(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger(NewContext(context.Background(), "rid-log-0001"), zap.New(core)).Info("hi")
	if v := logs.All()[0].ContextMap()["request_id"]; v != "rid-log-0001" {
		t.Errorf("request_id = %v", v)
	}
	Logger(context.Background(), nil).Info("no panic")
}
