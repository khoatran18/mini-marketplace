// Package tracing wires OpenTelemetry for the services (ADR-36). It is OFF until OTEL_EXPORTER_OTLP_ENDPOINT is set
// (for example http://tempo:4318); even when off the W3C trace context is still passed through gRPC, so enabling it
// on one service later never breaks a trace.
//
//	gateway HTTP root span (middleware) -> gRPC client span -> gRPC server span of each service -> ...
//
// Sampling: parent-based, the root samples OTEL_TRACES_SAMPLER_ARG of requests (default 0.1).
// Kafka hops are not traced as spans; the request id (reqctx) links them in the logs.
package tracing

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// Enabled reports whether spans are exported.
func Enabled() bool { return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" }

// Init installs the global propagator and, when enabled, the exporter and sampler. The returned function flushes
// pending spans; call it on shutdown (it is never nil).
func Init(ctx context.Context, service string) (shutdown func(context.Context) error, err error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if !Enabled() {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx) // endpoint and headers come from the standard OTEL_EXPORTER_OTLP_* variables
	if err != nil {
		return func(context.Context) error { return nil }, err
	}
	ratio := 0.1
	if v, perr := strconv.ParseFloat(os.Getenv("OTEL_TRACES_SAMPLER_ARG"), 64); perr == nil && v >= 0 && v <= 1 {
		ratio = v
	}
	res, _ := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", service),
		attribute.String("service.version", os.Getenv("SERVICE_VERSION")),
		attribute.String("deployment.environment", os.Getenv("ENV")),
	))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// ServerOption traces incoming gRPC calls (and continues the caller's trace).
func ServerOption() grpc.ServerOption { return grpc.StatsHandler(otelgrpc.NewServerHandler()) }

// ClientOption traces outgoing gRPC calls and passes the trace context on.
func ClientOption() grpc.DialOption { return grpc.WithStatsHandler(otelgrpc.NewClientHandler()) }

// Tracer is the tracer for hand-made spans (the gateway's HTTP root span).
func Tracer() trace.Tracer { return otel.Tracer("mini-marketplace") }

// Extract returns ctx carrying the trace context found in HTTP headers (a caller-supplied traceparent).
func Extract(ctx context.Context, h propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, h)
}
