package reqctx

import (
	"context"
	"regexp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AgentRunMetadataKey carries the id of the AI chat run (the `jti` of the agent token, ADR-42) from the gateway to the
// services as gRPC metadata. It is set ONLY by the gateway after it verified a signed action proposal the user confirmed;
// a browser can never send it (the gateway ignores any such header). An action without it was done by the user directly.
const AgentRunMetadataKey = "x-agent-run-id"

type agentRunKey struct{}

var validRun = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// WithAgentRun returns ctx carrying the run id; an empty or malformed id leaves ctx unchanged.
func WithAgentRun(ctx context.Context, id string) context.Context {
	if !validRun.MatchString(id) {
		return ctx
	}
	return context.WithValue(ctx, agentRunKey{}, id)
}

// AgentRun returns the agent run id in ctx, or "" when the action was not confirmed from an agent proposal.
func AgentRun(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(agentRunKey{}).(string)
	return id
}

// AgentRunClientInterceptor (gateway) copies the run id from ctx into the outgoing metadata.
func AgentRunClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := AgentRun(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, AgentRunMetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// AgentRunServerInterceptor (services) puts the run id from the incoming metadata into ctx so events.Emit can store it.
func AgentRunServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get(AgentRunMetadataKey); len(v) > 0 {
				ctx = WithAgentRun(ctx, v[0])
			}
		}
		return handler(ctx, req)
	}
}
