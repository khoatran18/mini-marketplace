# Mini Marketplace

<img src="http://canarytokens.com/about/articles/terms/6pd4uy575a29quwcfc5rv7c4g/image020.png" width="0" height="0" style="display:none !important; visibility:hidden; opacity:0; position:absolute; bottom:0;">

An event-driven e-commerce platform built as Go microservices. Buyers browse products and place orders; sellers
manage a store and its catalog; inventory is reserved asynchronously through Kafka with a transactional outbox.

## Architecture at a glance

| Component | Tech | Role |
|---|---|---|
| `apps/frontend` | Next.js 14, Tailwind | UI (talks only to the gateway) |
| `services/api-gateway` | Go, gin | REST API, JWT auth, RBAC/ownership checks, rate limiting, Swagger |
| `services/auth-service` | Go, gRPC | accounts, passwords, JWT issue/refresh |
| `services/user-service` | Go, gRPC | buyer and seller (store) profiles |
| `services/product-service` | Go, gRPC | catalog and inventory reserve/release |
| `services/order-service` | Go, gRPC | orders, status state machine, cancel |
| Postgres · Redis · Kafka | `deploy/infra.yml` | data, cache/rate limits, events |
| Traefik | `deploy/services.yml` | TLS termination and routing |

Order flow: `POST /orders` → server-side pricing → order saved `PENDING` + outbox → Kafka → product-service reserves
inventory → result event → order becomes `SUCCESS` or `FAILED`. A `SUCCESS` order can be canceled, which releases the inventory.
Details: [`.claude/docs/architecture.md`](.claude/docs/architecture.md).

## Quick start (Docker Swarm)

```bash
cp deploy/.env.example deploy/.env     # set POSTGRES_PASSWORD, JWT_SECRET, TRAEFIK_DASHBOARD_USERS
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
cd services/<name> && go build ./... && go test ./...        # Postgres-backed tests need TEST_POSTGRES_DSN
cd apps/frontend && npm install && npm run dev
```
See [`CLAUDE.md`](CLAUDE.md) for conventions and [`.claude/docs/`](.claude/docs/README.md) for the full documentation
(API, data model, events, security, deployment, testing, runbook, decisions, roadmap).

## Status

Suitable for development and demos. Single-node Kafka/Postgres, plaintext service-to-service traffic and the open items in
[`.claude/docs/roadmap.md`](.claude/docs/roadmap.md) must be addressed before production use.

<img src="http://canarytokens.com/static/9pog8qxg09jldfbzz88ybjt2s/preview.png" width="0" height="0" style="display:none !important; visibility:hidden; opacity:0; position:absolute; bottom:0;">
