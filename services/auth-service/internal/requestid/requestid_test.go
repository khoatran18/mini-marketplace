package requestid

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestServerGeneratesWhenMissing(t *testing.T) {
	var got string
	_, err := UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, _ any) (any, error) { got = FromContext(ctx); return nil, nil })
	if err != nil || len(got) < 8 {
		t.Fatalf("expected generated id, got %q err=%v", got, err)
	}
}

func TestServerKeepsIncoming(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "abc-12345"))
	var got string
	_, _ = UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, _ any) (any, error) { got = FromContext(ctx); return nil, nil })
	if got != "abc-12345" {
		t.Fatalf("got %q", got)
	}
}

func TestClientRoundtrip(t *testing.T) {
	ctx := NewContext(context.Background(), "rid-98765")
	var out metadata.MD
	err := UnaryClientInterceptor()(ctx, "/m", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			out, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
	if err != nil || len(out.Get(MetadataKey)) != 1 || out.Get(MetadataKey)[0] != "rid-98765" {
		t.Fatalf("md=%v err=%v", out, err)
	}
}

func TestLogger(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger(NewContext(context.Background(), "rid-1"), zap.New(core)).Info("x")
	if logs.All()[0].ContextMap()["request_id"] != "rid-1" {
		t.Fatal("missing request_id field")
	}
}
