package requestid

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

func TestContextRoundtrip(t *testing.T) {
	if FromContext(context.Background()) != "" {
		t.Fatal("ctx rỗng phải trả chuỗi rỗng")
	}
	if got := FromContext(NewContext(context.Background(), "abc")); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

// captureServer ghi lại id mà handler thấy.
type captureServer struct {
	healthpb.UnimplementedHealthServer
	seen string
}

func (c *captureServer) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	c.seen = FromContext(ctx)
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func call(t *testing.T, ctx context.Context) (seen string, hdr metadata.MD) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cs := &captureServer{}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(UnaryServerInterceptor()))
	healthpb.RegisterHealthServer(s, cs)
	go s.Serve(lis)
	defer s.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithChainUnaryInterceptor(UnaryClientInterceptor()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}, grpc.Header(&hdr)); err != nil {
		t.Fatal(err)
	}
	return cs.seen, hdr
}

func TestKeepsIncomingID(t *testing.T) {
	seen, hdr := call(t, NewContext(context.Background(), "req-12345678"))
	if seen != "req-12345678" || hdr.Get(MetadataKey)[0] != "req-12345678" {
		t.Fatalf("seen=%q hdr=%v", seen, hdr)
	}
}

func TestGeneratesWhenMissing(t *testing.T) {
	seen, hdr := call(t, context.Background())
	if len(seen) < 8 || len(hdr.Get(MetadataKey)) != 1 || hdr.Get(MetadataKey)[0] != seen {
		t.Fatalf("seen=%q hdr=%v", seen, hdr)
	}
}
