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

## Writing tests
- Fake a gRPC dependency by embedding the generated client interface in a struct and overriding the methods you need (see `fakeProductClient`, `fakeOrderClient`).
- Gateway tests build gin engines with a stub middleware that sets `userID`/`userRole`; Redis is `miniredis`.
- Prefer testing the service layer against real Postgres (row locks, conditional updates and NUMERIC behave differently in mocks).

## Not covered yet
Kafka wiring end-to-end (no broker in the test environment), the user-service, gRPC server adapters, and the frontend. An integration suite on `docker compose` that runs register → order → inventory → cancel is the next useful addition.
