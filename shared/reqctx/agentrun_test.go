package reqctx

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestAgentRunContext(t *testing.T) {
	if AgentRun(context.Background()) != "" {
		t.Fatal("empty by default")
	}
	if AgentRun(WithAgentRun(context.Background(), "short")) != "" {
		t.Fatal("malformed id must be ignored")
	}
	if got := AgentRun(WithAgentRun(context.Background(), "run-0123456789")); got != "run-0123456789" {
		t.Fatalf("got %q", got)
	}
}

func TestAgentRunServerInterceptorReadsMetadataOnly(t *testing.T) {
	var seen string
	h := func(ctx context.Context, _ any) (any, error) { seen = AgentRun(ctx); return nil, nil }
	in := metadata.NewIncomingContext(context.Background(), metadata.Pairs(AgentRunMetadataKey, "run-0123456789"))
	if _, err := AgentRunServerInterceptor()(in, nil, nil, h); err != nil || seen != "run-0123456789" {
		t.Fatalf("seen=%q err=%v", seen, err)
	}
	bad := metadata.NewIncomingContext(context.Background(), metadata.Pairs(AgentRunMetadataKey, "x"))
	_, _ = AgentRunServerInterceptor()(bad, nil, nil, h)
	if seen != "" {
		t.Fatalf("invalid id must be dropped, got %q", seen)
	}
}
