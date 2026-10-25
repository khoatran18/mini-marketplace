package requestid

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestServerKeepsIncomingID(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "abc-12345"))
	var got string
	_, _ = UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		got = FromContext(ctx)
		return nil, nil
	})
	if got != "abc-12345" {
		t.Errorf("got %q", got)
	}
}

func TestServerCreatesIDWhenMissing(t *testing.T) {
	var got string
	_, _ = UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		got = FromContext(ctx)
		return nil, nil
	})
	if len(got) < 8 {
		t.Errorf("expected generated id, got %q", got)
	}
}

func TestClientPropagatesID(t *testing.T) {
	ctx := NewContext(context.Background(), "req-98765")
	var md metadata.MD
	err := UnaryClientInterceptor()(ctx, "/m", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ = metadata.FromOutgoingContext(ctx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := md.Get(MetadataKey); len(v) != 1 || v[0] != "req-98765" {
		t.Errorf("outgoing md = %v", md)
	}
}

func TestLoggerAddsField(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger(NewContext(context.Background(), "rid-00001"), zap.New(core)).Info("x")
	if logs.All()[0].ContextMap()["request_id"] != "rid-00001" {
		t.Errorf("missing request_id field")
	}
}
