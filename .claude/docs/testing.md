# Testing

```bash
cd services/<name> && go test -race ./...          # unit tests; DB tests skip without TEST_POSTGRES_DSN
TEST_POSTGRES_DSN="host=localhost port=5432 user=postgres password=postgres sslmode=disable" go test -race ./...
cd apps/frontend && npx tsc --noEmit && npm run build
```
DB tests create a throw-away database (`CREATE DATABASE …_test_<rand>`) per test and drop it afterward; they need a role allowed to create databases. CI runs everything against a Postgres service container (`.github/workflows/ci.yml`).

## What is covered
| Area | File | Highlights |
|---|---|---|
| order pricing & lifecycle | `order-service/internal/service/order_service_test.go` | server-side prices, duplicate merging, validation codes, outbox carries the real order id, idempotent status events, cancel state machine & single cancel event, status-only update |
| inventory | `product-service/internal/service/product_service_test.go` | all-or-nothing reserve, idempotent/concurrent duplicate delivery, no overselling under concurrency, release exactly once, owner checks, zero-value updates, pagination bounds |
| auth | `auth-service/internal/service/auth_service_test.go` | role whitelist, bcrypt storage, uniform login errors, token types, password change invalidation, employee-only registration |
| gateway authz | `api-gateway/internal/handler/*_test.go` | ownership for orders/buyers/stores/products, client-controlled fields ignored, error mapping |
| gateway middleware & routes | `internal/middleware`, `internal/router` | JWT type/expiry/alg, pwd-version revocation, role checks, atomic rate limiting, auth required on every protected route, CORS allow-list |

Shared ops package (`services/*/pkg/ops`): `go test -race ./pkg/ops/` – liveness ignores dependencies, ready/degraded/not-ready, sanitized errors, timeouts, caching, draining, metrics labels, and a real-socket test of the admin port + gRPC health. The five copies must stay byte-identical (`md5sum services/*/pkg/ops/*.go`).

Admin role: `auth-service/internal/service/admin_test.go` (bootstrap rules, idempotency, no password takeover, admin can't be registered, admin credentials don't work for other roles; needs `TEST_POSTGRES_DSN`); gateway `router_test.go` (`/admin/system/health` is admin-only and aggregates ready/not_ready/unreachable; body limit returns 413) and `middleware/bodylimit_test.go`.
Local Postgres for DB tests: `initdb`/`pg_ctl` from `/usr/lib/postgresql/16/bin` as the `postgres` OS user (data dir outside `/tmp/claude-*`), then `export TEST_POSTGRES_DSN="host=localhost port=5433 user=postgres sslmode=disable"`.

## Writing tests
- Fake a gRPC dependency by embedding the generated client interface in a struct and overriding the methods you need (see `fakeProductClient`, `fakeOrderClient`).
- Gateway tests build gin engines with a stub middleware that sets `userID`/`userRole`; Redis is `miniredis`.
- Prefer testing the service layer against real Postgres (row locks, conditional updates and NUMERIC behave differently in mocks).

## Tests on the real infrastructure (Docker Swarm)
```bash
./scripts/deploy.sh                                   # after build-images.sh
scripts/e2e/run-in-swarm.sh                           # 58 checks: Traefik/TLS/UI/dashboard/CORS, auth, store+catalog, order saga, concurrency, token revocation
RATE_LIMIT_TEST=1 scripts/e2e/run-in-swarm.sh         # needs AUTH_RATE_LIMIT_PER_MINUTE=20 (default); the main suite needs it raised (e.g. 1000)
scripts/e2e/resilience.sh                             # failure injection: Kafka down, product/order-service down, rolling restart, Postgres restart
```
Both run from a throw-away container on the swarm network (`marketplace-net`), so they work where the host cannot reach the ingress ports; point `API_URL`/`UI_URL` at a public URL and run `node scripts/e2e/e2e.mjs` directly to test a remote deployment.
`e2e.mjs` exercises: cross-service store linking through Kafka, server-side pricing, inventory reserve/release, 12 buyers racing for 5 units (exactly 5 succeed), IDOR attempts, password-change revocation through Redis, throttling.

## Not covered yet
user-service unit tests, gRPC server adapters, and browser-level frontend tests (the UI is only checked for being served and for CORS).
