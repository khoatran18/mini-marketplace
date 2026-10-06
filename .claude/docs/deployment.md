# Deployment

Two compose files target Docker Swarm (they also work with plain `docker compose` for local use):
- `deploy/infra.yml` – `postgres:16-alpine`, `redis:8` (AOF), single-node Kafka (KRaft), named volumes, healthchecks, overlay network `marketplace-infra_marketplace`.
- `deploy/services.yml` – Traefik v3, frontend, api-gateway (2 replicas) and the four services. Env comes from the shell/`deploy/.env` by compose interpolation; **no `.env` is baked into images**.

## First deploy
```bash
cp deploy/.env.example deploy/.env      # set POSTGRES_PASSWORD, JWT_SECRET, TRAEFIK_DASHBOARD_USERS (htpasswd, $ → $$)
./scripts/gen-dev-certs.sh              # or place real certs in deploy/traefik/certs/{local.crt,local.key}
API_BASE_URL=https://api.marketplace.swarm.localhost ./scripts/build-images.sh   # frontend bakes the API URL at build time
docker swarm init
./scripts/deploy.sh            # infra stack "marketplace-infra" then app stack "marketplace"
```
Hosts: `https://marketplace.swarm.localhost` (UI), `https://api.marketplace.swarm.localhost`, `https://dashboard.marketplace.swarm.localhost`. Add them to `/etc/hosts` for local use.

## Environment variables
| Var | Used by | Default / note |
|---|---|---|
| `POSTGRES_USER/PASSWORD/DB` | infra, services | password required |
| `JWT_SECRET` | all | ≥ 16 chars, same everywhere |
| `JWT_EXPIRE_TIME` | auth, gateway | minutes, default 5 |
| `POSTGRES_DSN`, `REDIS_ADDR`, `KAFKA_BROKERS_ADDR` | services | built in `services.yml`; for local runs see `services/*/cmd/.env.example` |
| `KAFKA_PRODUCER_RETRY/BACKOFF`, `KAFKA_CONSUMER_BACKOFF` | services | 2 / 100 ms / 100 ms |
| `ALLOWED_ORIGINS`, `RATE_LIMIT_PER_MINUTE`, `AUTH_RATE_LIMIT_PER_MINUTE`, `TRUSTED_PROXIES` | gateway | see security.md |
| `ADMIN_BOOTSTRAP_USERNAME/PASSWORD` | auth | creates the platform admin once (see security.md); empty = none |
| `MAX_BODY_BYTES`, `SYSTEM_HEALTH_TARGETS` | gateway | body cap (default 1048576); `name=http://host:8081,…` list for `/admin/system/health` |
| `GRAFANA_ADMIN_USER/PASSWORD`, `PROMETHEUS_RETENTION` | observability.yml | password required |
| `SEED_DEMO_DATA` | auth, product | `true` creates `buyer1`/`seller1` (password `password`) and sample products |
| `PUBLIC_HOST`, `TRAEFIK_DASHBOARD_USERS` | services.yml | routing and dashboard auth |

## Running locally without Swarm
Start infra with `docker compose --env-file deploy/.env -f deploy/infra.yml up -d` (publish ports as needed), copy `services/<svc>/cmd/.env.example` to `.env`, then `go run ./cmd` in each service (start auth/product before order/user). The Kafka advertised listener is `broker1:9092`; for host access add an `/etc/hosts` entry `127.0.0.1 broker1` and publish port 9092.

## Notes from the real deployment
- Traefik must be ≥ v3.6 on Docker ≥ 29 (older Traefik uses Docker API 1.24, now rejected by the daemon).
- `docker stack deploy` needs the rendered compose without the top-level `name:` and with numeric ports; `scripts/deploy.sh` does that.
- The shared network is explicitly named `marketplace-net` (created by `infra.yml`, external in `services.yml`).
- The frontend image must listen on `0.0.0.0` (`HOSTNAME=0.0.0.0`), otherwise its healthcheck on `localhost` fails and Swarm kills it.
- Docker Hub rate limits (HTTP 429) can interrupt image pulls/builds on shared egress IPs; retry or use a registry mirror.

## Monitoring (Prometheus + Grafana)
`scripts/deploy.sh observability` deploys stack `marketplace-obs` (`deploy/observability.yml`): Prometheus, Grafana (`https://grafana.<PUBLIC_HOST>`, user/password from `GRAFANA_ADMIN_USER/PASSWORD`), cAdvisor and node-exporter on every node, and Postgres/Redis/Kafka exporters. It is view-only: there is no alerting. Dashboards (Overview, Services (RED), Containers & hosts, Kafka/Postgres/Redis/Traefik) are provisioned from `deploy/grafana/dashboards`. After the first deploy open Prometheus → Status → Targets (port-forward or `docker exec`) and check every job is UP. Details: [platform/05-analytics-monitoring.md](platform/05-analytics-monitoring.md).

## Operational endpoints
Every Go service listens on an extra **internal** HTTP port `ADMIN_PORT` (default `8081`, not published, not routed by Traefik): `/health` = `/healthz` (liveness), `/ready` = `/readyz` (readiness: Postgres, Kafka, downstream gRPC health are critical; Redis is non-critical), `/metrics` (Prometheus), `/version`. gRPC services also register `grpc.health.v1`. On SIGTERM a service reports not-ready for `DRAIN_SECONDS` (default 10) before stopping. Docker healthchecks use `/healthz`; Traefik routes to the gateway only while its `/ready` is 200. The frontend exposes `/api/health|healthz|ready|readyz`. Details: [platform/02-health-ready-metrics.md](platform/02-health-ready-metrics.md).
Quick check from any container on the network: `wget -qO- http://order-service:8081/ready`.

## Operations
- Scale: `docker service scale marketplace_api-gateway=3`. Services with Kafka consumers can run multiple replicas (consumer groups split partitions); outbox workers may then publish the same row twice – safe because consumers are idempotent.
- Kafka single broker and one Postgres are single points of failure. Back up the `postgres-data` volume (`pg_dump`). For HA use managed Postgres and a 3-broker Kafka (set RF 3, min ISR 2).
- Disable topic auto-creation (`KAFKA_AUTO_CREATE_TOPICS_ENABLE=false`) once topics exist; services create what they need explicitly.
- Images are not pushed anywhere by CI (build only). Add a registry push step before deploying to a multi-node swarm.
