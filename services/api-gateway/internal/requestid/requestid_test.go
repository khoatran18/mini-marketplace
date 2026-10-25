package requestid

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestClientInterceptorPushesID(t *testing.T) {
	ctx := NewContext(context.Background(), "abcdefgh-1")
	var got []string
	inv := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		got = md.Get(MetadataKey)
		return nil
	}
	if err := UnaryClientInterceptor()(ctx, "/s/M", nil, nil, nil, inv); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "abcdefgh-1" {
		t.Fatalf("metadata = %v", got)
	}
}

func TestClientInterceptorNoIDNoMetadata(t *testing.T) {
	inv := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		if md, ok := metadata.FromOutgoingContext(ctx); ok && len(md.Get(MetadataKey)) > 0 {
			t.Fatal("unexpected metadata")
		}
		return nil
	}
	_ = UnaryClientInterceptor()(context.Background(), "/s/M", nil, nil, nil, inv)
}

func TestServerInterceptorKeepsAndGenerates(t *testing.T) {
	h := func(ctx context.Context, _ any) (any, error) { return FromContext(ctx), nil }
	in := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "keep-this-id"))
	out, _ := UnaryServerInterceptor()(in, nil, &grpc.UnaryServerInfo{}, h)
	if out != "keep-this-id" {
		t.Fatalf("got %v", out)
	}
	out, _ = UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{}, h)
	if s, _ := out.(string); !Valid(s) {
		t.Fatalf("generated id invalid: %v", out)
	}
}
