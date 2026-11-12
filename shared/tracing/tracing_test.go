package tracing

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestInitDisabledStillPropagatesTraceContext(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := Init(context.Background(), "svc")
	if err != nil || shutdown == nil || Enabled() {
		t.Fatalf("err=%v shutdown=%v enabled=%v", err, shutdown != nil, Enabled())
	}
	h := http.Header{}
	h.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	ctx := Extract(context.Background(), propagation.HeaderCarrier(h))
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() || sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace context not extracted: %+v", sc)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInitEnabledBuildsProvider(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1") // nothing listens: spans are dropped, Init must still succeed
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "1")
	shutdown, err := Init(context.Background(), "svc")
	if err != nil || !Enabled() {
		t.Fatalf("err=%v enabled=%v", err, Enabled())
	}
	_, span := Tracer().Start(context.Background(), "x")
	if !span.SpanContext().IsSampled() {
		t.Fatal("ratio 1 must sample")
	}
	span.End()
	_ = shutdown(context.Background())
}
