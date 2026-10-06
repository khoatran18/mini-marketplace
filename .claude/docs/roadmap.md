# Roadmap

## Technical debt
- Shared module for generated protobuf + `kafkaimpl` (ADR-8); `buf` module registry instead of five `buf.gen.yaml` copy targets.
- Versioned migrations (goose/atlas) instead of `AutoMigrate`; unique constraint on `accounts.username` is global although login is by (username, role).
- Integer minor units on the wire (ADR-7).
- Observability: request IDs, Prometheus metrics, tracing; structured logging everywhere (`fmt.Println` remains in config bootstrap code).
- Graceful shutdown (signal handling) for gRPC servers and outbox workers; the gateway uses `engine.Run`.
- gRPC deadlines/retries from gateway and order-service; circuit breaker around product-service.
- TLS for gRPC/Postgres; frontend tokens in httpOnly cookies; request body size limits.
- Kafka: 3 brokers, RF 3, schema/versioning of event payloads, dead-letter topic instead of "log and skip".
- User-service tests and an end-to-end docker-compose suite.

## Business logic worth adding (priority order)
1. **Fuller order lifecycle**: `PAID → SHIPPED → DELIVERED`, `REFUNDED`; who may move which transition (seller confirms/ships, buyer cancels before shipping); persisted status history. The state machine in `order-service` (`allowedPredecessors`) is the extension point.
2. **Reservation with expiry**: reserve stock on order creation, auto-cancel and release if unpaid after N minutes (scheduled worker); also lets a `PENDING` order time out instead of waiting forever.
3. **Idempotency key on `POST /orders`** (header → unique column) so client retries cannot create duplicates.
4. **Payment**: mock/COD first, then a provider with webhooks and refunds; `PAID` transition driven by an event.
5. **Cart and multi-seller checkout**: split one checkout into one order per store; shipping address and fee; snapshot product name on the order item.
6. **Seller tools**: order inbox and revenue dashboard for a store, employee management, product visibility (hide/out of stock).
7. **Reviews** limited to buyers of a delivered order; **search/filter** (the old Elasticsearch experiment was dropped, Postgres full-text is enough to start).
8. **Account safety**: email verification, password reset, lockout after repeated failures, explicit logout (token deny-list).
9. **Coupons, notifications over Kafka (email/in-app), audit log, soft-delete consistency**.
