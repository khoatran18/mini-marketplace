package requestid

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestClientToServerRoundtrip(t *testing.T) {
	ctx := NewContext(context.Background(), "req-12345678")
	var out metadata.MD
	err := UnaryClientInterceptor()(ctx, "/x", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		out, _ = metadata.FromOutgoingContext(ctx)
		return nil
	})
	if err != nil || len(out.Get(MetadataKey)) != 1 || out.Get(MetadataKey)[0] != "req-12345678" {
		t.Fatalf("outgoing md = %v err=%v", out, err)
	}
	in := metadata.NewIncomingContext(context.Background(), out)
	var got string
	_, _ = UnaryServerInterceptor()(in, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		got = FromContext(ctx)
		return nil, nil
	})
	if got != "req-12345678" {
		t.Fatalf("server id = %q", got)
	}
}

func TestServerGeneratesWhenMissing(t *testing.T) {
	var got string
	_, _ = UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		got = FromContext(ctx)
		return nil, nil
	})
	if len(got) < 8 {
		t.Fatalf("expected generated id, got %q", got)
	}
}

func TestClientWithoutIDAddsNothing(t *testing.T) {
	_ = UnaryClientInterceptor()(context.Background(), "/x", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		if md, ok := metadata.FromOutgoingContext(ctx); ok && len(md.Get(MetadataKey)) > 0 {
			t.Error("unexpected metadata")
		}
		return nil
	})
}

func TestLogger(t *testing.T) {
	if Logger(context.Background(), nil) == nil {
		t.Fatal("nil logger")
	}
	if Logger(NewContext(context.Background(), "abc12345"), nil) == nil {
		t.Fatal("nil logger")
	}
}
