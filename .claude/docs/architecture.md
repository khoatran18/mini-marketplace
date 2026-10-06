# Architecture

```
Browser ──HTTPS──▶ Traefik ──▶ frontend (Next.js :3000)
                        └────▶ api-gateway (gin :8080) ──gRPC──▶ auth-service    :50051
                                   │  │                  ├────▶ user-service    :50054
                                   │  │                  ├────▶ product-service :50053
                                   │  │                  └────▶ order-service   :50052
                                   │  └─ Redis (rate limit, password-version cache)
                                   └──── Kafka (auth.change_password consumer)
Each service: its own gRPC server, shared Postgres instance (own tables), Redis, Kafka client.
order-service ─gRPC─▶ product-service (price/inventory lookup)      user-service ─gRPC─▶ auth-service
```

## Services
| Service | Responsibility | Key files |
|---|---|---|
| api-gateway | REST API, JWT validation (`Type=="access"`), RBAC, ownership checks, CORS allow-list, rate limiting, Swagger | `internal/router`, `internal/middleware`, `internal/handler` |
| auth-service | register/login/refresh/change-password, store lookup for accounts, JWT signing | `internal/service/*` |
| user-service | buyer and seller (store) profiles; creating a seller emits `user.create_seller` so auth links `store_id` | `internal/service/{buyer,seller}_service.go` |
| product-service | catalog CRUD (owner = store), inventory reserve/release driven by Kafka | `internal/service`, `internal/repository` |
| order-service | order creation (server-side pricing), status machine, cancel, outbox publishers | `internal/service/order_service.go`, `kafka_service.go` |

Layering inside a service: `server` (gRPC, protovalidate, error mapping) → `service` (business rules) → `repository` (GORM) ; `client/*` wraps outgoing gRPC; `config/messagequeue/kafkaimpl` wraps kafka-go.

## Identity model
- Account (`auth.accounts`) has `role` (`buyer`, `seller_admin`, `seller_employee`) and `store_id` (0 until the seller creates a store).
- A *store* is a `user-service` seller row. `store_id` on the account = seller id. Product `seller_id` = store id.
- The gateway resolves the caller's store through auth-service (`GetStoreIDRoleById`) when it must check product/store ownership.

## Order saga (happy path and cancel)
1. `POST /orders` → gateway forces `buyer_id` from the token, clears price/status/total → order-service.
2. order-service loads the products from product-service, merges duplicate lines, validates quantity and stock (advisory), computes prices/total, and in **one transaction** inserts the order (status `PENDING`) and a `create_order` outbox row.
3. Outbox worker publishes `order.create_order` (key = order id).
4. product-service consumes it, decrements inventory for all lines in one transaction and stores a validation result (`success` true/false); an outbox worker publishes `product.validate_order`.
5. order-service consumes the result: `PENDING → SUCCESS` or `PENDING → FAILED` (conditional update; replays are ignored).
6. `DELETE /orders/:id` is allowed only for `SUCCESS` orders: status → `CANCELED` and a `cancel_order` outbox row in the same transaction → `order.cancel_order` → product-service releases the inventory exactly once (`validate_order_events.restored`).

`PENDING` orders cannot be canceled (processing is in flight); that removes the reserve/cancel race without a timeout mechanism. A PENDING timeout/expiry is listed in [roadmap.md](roadmap.md).

## Where things run
Single Postgres database (`postgres`), tables owned by individual services (no cross-service SQL). Redis: rate-limit counters and `"<userId>:pwd_version"` keys. Kafka: single broker, 3 partitions per topic by default.
