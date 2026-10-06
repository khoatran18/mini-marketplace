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
| `SEED_DEMO_DATA` | auth, product | `true` creates `buyer1`/`seller1` (password `password`) and sample products |
| `PUBLIC_HOST`, `TRAEFIK_DASHBOARD_USERS` | services.yml | routing and dashboard auth |

## Running locally without Swarm
Start infra with `docker compose --env-file deploy/.env -f deploy/infra.yml up -d` (publish ports as needed), copy `services/<svc>/cmd/.env.example` to `.env`, then `go run ./cmd` in each service (start auth/product before order/user). The Kafka advertised listener is `broker1:9092`; for host access add an `/etc/hosts` entry `127.0.0.1 broker1` and publish port 9092.

## Operations
- Scale: `docker service scale marketplace_api-gateway=3`. Services with Kafka consumers can run multiple replicas (consumer groups split partitions); outbox workers may then publish the same row twice – safe because consumers are idempotent.
- Kafka single broker and one Postgres are single points of failure. Back up the `postgres-data` volume (`pg_dump`). For HA use managed Postgres and a 3-broker Kafka (set RF 3, min ISR 2).
- Disable topic auto-creation (`KAFKA_AUTO_CREATE_TOPICS_ENABLE=false`) once topics exist; services create what they need explicitly.
- Images are not pushed anywhere by CI (build only). Add a registry push step before deploying to a multi-node swarm.
