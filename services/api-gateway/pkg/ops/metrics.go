package ops

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Gateway domain metrics. Package-level; New registers them with the service label on the Server registry.
var (
	rateLimited     = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_rate_limited_total", Help: "Requests rejected by rate limiting, by route group."}, []string{"group"})
	clientReqs      = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_grpc_client_requests_total", Help: "Outgoing gRPC calls."}, []string{"service", "method", "code"})
	clientRetry     = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_grpc_client_retries_total", Help: "Outgoing gRPC calls retried (read methods only)."}, []string{"service", "method"})
	circuitState    = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "mm_circuit_state", Help: "Circuit breaker state per target service: 0 closed, 1 open, 2 half-open (ADR-36)."}, []string{"service"})
	circuitRejected = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_circuit_rejected_total", Help: "Calls rejected without being sent because the circuit was open."}, []string{"service"})
	clientDur       = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "mm_grpc_client_duration_seconds", Help: "Outgoing gRPC call time.", Buckets: latencyBuckets}, []string{"service", "method"})
)

// IncRateLimited counts a 429. group is a route group name (auth, events, default), never an IP or path.
// The router's "global" scope is reported as "default"; unknown groups collapse to "default" to keep cardinality fixed.
func IncRateLimited(group string) {
	switch group {
	case "auth", "events", "default":
	default:
		group = "default"
	}
	rateLimited.WithLabelValues(group).Inc()
}

// SetCircuitState records the breaker state of a target service (0 closed, 1 open, 2 half-open).
func SetCircuitState(service string, state float64) { circuitState.WithLabelValues(service).Set(state) }

// IncCircuitRejected counts a call rejected by an open circuit.
func IncCircuitRejected(service string) { circuitRejected.WithLabelValues(service).Inc() }

// IncClientRetry counts one retry attempt (không tính lần gọi đầu) tới service đích.
func IncClientRetry(service, method string) { clientRetry.WithLabelValues(service, method).Inc() }

// ClientInterceptor records mm_grpc_client_* for every unary call made through a connection to the named service.
// method is the full gRPC method (bounded by the proto definitions).
func ClientInterceptor(service string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		clientReqs.WithLabelValues(service, method, status.Code(err).String()).Inc()
		clientDur.WithLabelValues(service, method).Observe(time.Since(start).Seconds())
		return err
	}
}

// registerDomain: the client metrics keep their own "service" label (the target service), so they are registered
// without the gateway's const service label.
func registerDomain(r, raw prometheus.Registerer) {
	r.MustRegister(rateLimited)
	raw.MustRegister(clientReqs, clientDur, clientRetry, circuitState, circuitRejected)
	for _, g := range []string{"auth", "events", "default"} {
		rateLimited.WithLabelValues(g)
	}
}
