# Kafka events

Broker: `broker1:9092` (single node). Topics are created by the services at start (`KafkaClient.EnsureTopicExist`, 3 partitions, RF 1).

| Topic | Producer → Consumer (group) | Key | Payload |
|---|---|---|---|
| `order.create_order` | order-service → product-service (`product-service-validate-order`) | order id | `{order_id, items:[{product_id,quantity}]}` |
| `product.validate_order` | product-service → order-service (`order-service-validate-result`) | order id | `{order_id, success}` |
| `order.cancel_order` | order-service → product-service (`product-service-cancel-order`) | order id | `{order_id, items:[…]}` |
| `auth.change_password` | auth-service → api-gateway (`api-gateway-group-3`) | user id | `{user_id, pwd_version}` |
| `user.create_seller` | user-service → auth-service (`auth-service`) | seller id | `{seller_id, user_id}` |

## Guarantees
- **Producer**: transactional outbox. Rows are written in the same DB transaction as the business change; a worker (every 3 s, batch 100, oldest first) publishes with `acks=all` and marks the row `SUCCESS`/`FAILED`. A crash between publish and mark re-publishes → **at-least-once**.
- **Consumer** (`kafkaimpl.KafkaConsumer`): commits only after the handler succeeds; failures are retried up to 5 times with growing backoff, then the message is logged and skipped so a poison message cannot block a partition. Malformed JSON is dropped immediately (handlers return nil).
- **Idempotency**: order status uses conditional updates; inventory reserve is guarded by the primary key of `validate_order_events` (duplicates roll back); release by `restored`; password version is a "latest wins" SET.
- **Ordering**: same key → same partition (`kafka.Hash`), so events of one order/user stay ordered. Different topics have no relative ordering (the cancel flow avoids needing one by only canceling `SUCCESS` orders).

## Adding an event
1. Add the outbox table + `Get…NotPublish`/`Update…Status` in the producer's repository.
2. Write the outbox row in the same transaction as the change.
3. Start a worker (`runOutboxWorker` in order-service is the template) and `EnsureTopicExist` in `main.go`.
4. Consumer handler must be idempotent and return nil for unrecoverable input.
