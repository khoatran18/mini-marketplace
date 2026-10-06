# Mini Marketplace

<img src="http://canarytokens.com/about/articles/terms/6pd4uy575a29quwcfc5rv7c4g/image020.png" width="0" height="0" style="display:none !important; visibility:hidden; opacity:0; position:absolute; bottom:0;">

An event-driven e-commerce platform built as Go microservices. Buyers browse products and place orders; sellers
manage a store and its catalog; inventory is reserved asynchronously through Kafka with a transactional outbox.

## Architecture at a glance

| Component | Tech | Role |
|---|---|---|
| `apps/frontend` | Next.js 14, Tailwind | UI (talks only to the gateway) |
| `services/api-gateway` | Go, gin | REST API, JWT auth, RBAC/ownership checks, rate limiting, tracking endpoint |
| `services/auth-service` | Go, gRPC | accounts, passwords, JWT issue/refresh |
| `services/user-service` | Go, gRPC | buyer and seller (store) profiles |
| `services/product-service` | Go, gRPC | catalog/search, inventory reserve/release/ship + ledger |
| `services/order-service` | Go, gRPC | cart, checkout (one order per store), order lifecycle |
| `services/payment-service` | Go, gRPC | **simulated** payments (test cards, webhooks, refunds) |
| `services/analytics-service` | Go, gRPC | tracking + events → ClickHouse, seller/admin reports |
| Postgres · Redis · Kafka · ClickHouse | `deploy/infra.yml` | data, cache/rate limits, events, analytics |
| Prometheus · Grafana | `deploy/observability.yml` | metrics dashboards (no alerting) |
| Traefik | `deploy/services.yml` | TLS termination and routing |

Order flow: `POST /orders` (with `Idempotency-Key`) → server-side pricing → one order per store saved `PENDING` + outbox → Kafka →
product-service reserves inventory → `AWAITING_PAYMENT` (online, simulated payment) or `CONFIRMED` (COD) or `FAILED` → `PAID` →
`SHIPPED` → `DELIVERED` (→ return/refund); unpaid orders expire and release stock.
Details: [`.claude/docs/architecture.md`](.claude/docs/architecture.md), REST reference [`.claude/docs/api.md`](.claude/docs/api.md).

## Quick start (Docker Swarm)

```bash
cp deploy/.env.example deploy/.env     # set POSTGRES_PASSWORD, JWT_SECRET, TRAEFIK_DASHBOARD_USERS, GRAFANA_ADMIN_PASSWORD, CLICKHOUSE_PASSWORD, PAYMENT_WEBHOOK_SECRET
./scripts/gen-dev-certs.sh             # self-signed TLS for *.marketplace.swarm.localhost
./scripts/build-images.sh              # builds marketplace/<service>:latest
docker swarm init
./scripts/deploy.sh                    # infra stack, then application stack
```

Add `127.0.0.1 marketplace.swarm.localhost api.marketplace.swarm.localhost dashboard.marketplace.swarm.localhost`
to `/etc/hosts`, open <https://marketplace.swarm.localhost>. Set `SEED_DEMO_DATA=true` in `deploy/.env` for demo
accounts (`buyer1` / `seller1`, password `password`) and sample products.

## Development

```bash
cd services/<name> && go build ./... && go test ./...        # needs TEST_POSTGRES_DSN / TEST_CLICKHOUSE_URL for DB tests
cd apps/frontend && npm install && npm run dev
```
See [`CLAUDE.md`](CLAUDE.md) for conventions and [`.claude/docs/`](.claude/docs/README.md) for the full documentation
(API, data model, events, security, deployment, testing, runbook, decisions, roadmap).

## Status

Suitable for development and demos. Single-node Kafka/Postgres, plaintext service-to-service traffic and the open items in
[`.claude/docs/roadmap.md`](.claude/docs/roadmap.md) must be addressed before production use.

<img src="http://canarytokens.com/static/9pog8qxg09jldfbzz88ybjt2s/preview.png" width="0" height="0" style="display:none !important; visibility:hidden; opacity:0; position:absolute; bottom:0;">
