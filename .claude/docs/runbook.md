# Runbook

## Orders stuck in PENDING
1. `docker service logs marketplace_product-service | grep -i "reserve\|validate"` – is the consumer running? Group lag: `kafka-consumer-groups.sh --bootstrap-server broker1:9092 --describe --group product-service-validate-order`.
2. Outbox backlog: `SELECT status, count(*) FROM create_order_events GROUP BY 1;` (order DB) and `… FROM validate_order_events WHERE processed GROUP BY status;` (product). Rows stuck in `FAILED` are retried every 3 s; persistent failures mean Kafka is unreachable.
3. A consumer that exhausts its 5 attempts logs `giving up on message … offset N`; fix the cause and re-publish by setting the outbox row back to `PENDING` (`UPDATE create_order_events SET status='PENDING' WHERE order_id=…`). Product-side validation is idempotent.

## Inventory looks wrong
- Reserved quantities: `validate_order_events(success=true, restored=false)` joined with the order items. Canceled orders must have `restored=true` once the `order.cancel_order` event was consumed; if not, check `cancel_order_events.status`.

## Users get 401 right after changing password
Expected for tokens issued before the change. If *new* tokens fail, check Redis key `<userId>:pwd_version` and the gateway consumer (`auth.change_password`).

## 429 responses
Counters live in Redis (`rate_limit:<scope>:<ip>`). Behind Traefik set `TRUSTED_PROXIES`, otherwise all clients share one IP and hit the limit together.

## Kafka topic missing / consumer idle
Services call `EnsureTopicExist` at start; restart the service. For host-side tools remember the advertised listener is `broker1:9092`.

## Rotate JWT secret
Update `JWT_SECRET` in `deploy/.env`, redeploy all stacks. Every user must log in again.

## Seed accounts
`SEED_DEMO_DATA=true` (auth: `buyer1`, `seller1` / `password`; product: 20 sample products owned by store id 2). Disable in production.
