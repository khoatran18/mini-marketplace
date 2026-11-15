package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

// ProposalTTL is how long the user has to confirm; short, because the numbers in it (stock, price, status) go stale.
const ProposalTTL = 5 * time.Minute

// proposable is the closed list of write routes an agent may propose (method + path). Everything else is refused when the
// proposal is created, so the signed gate never covers a route nobody reviewed. Roles are still enforced by the route itself.
var proposable = []struct {
	Method string
	Path   *regexp.Regexp
}{
	{"POST", regexp.MustCompile(`^/orders/\d+/cancel$`)},
	{"POST", regexp.MustCompile(`^/orders/\d+/return$`)},
	{"POST", regexp.MustCompile(`^/seller/orders/\d+/cancel$`)},
	{"POST", regexp.MustCompile(`^/seller/orders/\d+/return/approve$`)},
	{"POST", regexp.MustCompile(`^/seller/orders/\d+/return/reject$`)},
	{"POST", regexp.MustCompile(`^/seller/products/\d+/inventory/adjust$`)}, // body {delta, reason} is bound by hash
	{"POST", regexp.MustCompile(`^/admin/orders/\d+/cancel$`)},
	{"POST", regexp.MustCompile(`^/admin/payments/\d+/refund$`)},
	{"POST", regexp.MustCompile(`^/ops/proposals$`)}, // operator only: records an ops proposal; review, approval and execution stay with other humans
}

// Proposable reports whether method+path may be proposed.
func Proposable(method, path string) bool {
	for _, p := range proposable {
		if p.Method == method && p.Path.MatchString(path) {
			return true
		}
	}
	return false
}

// BodyHash is the hex SHA-256 of the canonical JSON of body (keys sorted, no whitespace), so the same request hashes the
// same however the browser formats it. An empty body hashes the empty string. Non-JSON bodies hash as raw bytes.
func BodyHash(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) > 0 {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil && !dec.More() {
			if canon, err := json.Marshal(v); err == nil {
				body = canon
			}
		}
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Once makes a proposal single-use.
type Once interface {
	// Take returns true only the first time id is taken within ttl.
	Take(ctx context.Context, id string, ttl time.Duration) (bool, error)
}

// RedisOnce is Once on Redis SET NX; an unavailable Redis returns an error (the caller must refuse, never allow).
type RedisOnce struct{ RDB *redis.Client }

func (r RedisOnce) Take(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	return r.RDB.SetNX(ctx, "agent_proposal:"+id, 1, ttl).Result()
}
