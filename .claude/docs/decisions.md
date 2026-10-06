# Architecture decision records

## ADR-1 Replace Patroni stack with stock Postgres (2026-10)
The repo vendored the whole Patroni project (+ etcd, haproxy, custom image) for HA. It was unmaintained here, not needed by the code, and dominated the repo. `deploy/infra.yml` now runs `postgres:16-alpine` with a volume and healthcheck. For HA use a managed Postgres or re-introduce Patroni as an external image, not vendored source.

## ADR-2 Secrets never in git
`.env`, keys, certs and binaries are ignored; `.env.example` files document the settings; images contain no configuration; compose injects env at deploy time.

## ADR-3 Authorization lives in the gateway, identity from the JWT
Protobuf contracts could not change without regenerating code in five modules, and the gateway is the only public entry point. Handlers therefore derive buyer/seller/store from the token (and the auth service for the store) and ignore client-supplied identity. Services still validate business rules (owner of product, order transitions).

## ADR-4 Price and state are server-side
order-service re-prices every order from product-service and owns the status state machine. The removed `PUT /orders/:id` allowed arbitrary rewrites.

## ADR-5 Cancel only confirmed orders, release stock via outbox event
Canceling a `PENDING` order would race with the in-flight inventory reservation across two topics with no ordering. Allowing cancel only after the result arrived (`SUCCESS`) makes release safe, and `restored` makes it exactly-once.

## ADR-6 At-least-once delivery with idempotent consumers
Transactional outbox + `acks=all` + commit-after-handle + bounded retries. Every handler is a conditional/idempotent write; there is no distributed transaction.

## ADR-7 NUMERIC money, integer-cent arithmetic
Changing the protobuf `double` fields to integer minor units needs regenerated code in five modules and a frontend change; for now precision is protected in storage and arithmetic, and the wire format stays `double`. Revisit with proto regeneration.

## ADR-8 One Go module per service (kept)
A shared module for generated protobuf and `kafkaimpl` would remove the copies in each service but requires building Docker images from the repository root and a `go.work`/`replace` setup. Deferred; the copies are kept identical (`kafkaimpl` was normalized in all five).
