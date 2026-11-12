// Package reqctx lets the shared libraries (outbox, Kafka producer/consumer) carry the request id without
// knowing each service's own internal/requestid package: a service registers how to read and write the id in its
// context once at start-up (Use), and the libraries call Get / Put. Without Use both are no-ops.
//
// Flow (ADR-36): HTTP X-Request-ID -> gateway ctx -> gRPC metadata -> service ctx -> events.Emit stores it in the
// outbox row (request_id) -> the publisher puts it in the Kafka header "x-request-id" -> the consumer puts it back
// in the handler's ctx, so one id follows an order across every service.
package reqctx

import (
	"context"
	"regexp"
	"sync/atomic"
)

// Header is the Kafka message header that carries the id (same spelling as the gRPC metadata key).
const Header = "x-request-id"

type hooks struct {
	get func(context.Context) string
	put func(context.Context, string) context.Context
}

var current atomic.Pointer[hooks]

// Use registers the service's own accessors (requestid.FromContext and requestid.NewContext). Call once at start-up.
func Use(get func(context.Context) string, put func(context.Context, string) context.Context) {
	current.Store(&hooks{get: get, put: put})
}

// Get returns the request id in ctx, or "".
func Get(ctx context.Context) string {
	if h := current.Load(); h != nil && h.get != nil && ctx != nil {
		return h.get(ctx)
	}
	return ""
}

var valid = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// Put returns ctx carrying id; an empty or malformed id (it may come from a Kafka header) leaves ctx unchanged.
func Put(ctx context.Context, id string) context.Context {
	if h := current.Load(); h != nil && h.put != nil && valid.MatchString(id) {
		return h.put(ctx, id)
	}
	return ctx
}
