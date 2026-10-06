# Data model

Schemas are created with GORM `AutoMigrate` at service start (no versioned migrations yet – see roadmap).

## auth-service
- `accounts(id, username UNIQUE, password bcrypt, role, pwd_version, store_id, deleted_at)`
- `pwd_version_events(user_id PK, pwd_version, status, created_at)` – outbox for `auth.change_password`

## user-service
- `buyers(user_id PK, name, gender, date_of_birth, phone, address, deleted_at)`
- `sellers(id PK, name, bank_account, tax_code, description, date_of_birth, phone, address, deleted_at)` – a seller is a store
- `create_seller_events(seller_id PK, user_id, status, created_at)` – outbox for `user.create_seller`

## product-service
- `products(id, name, price NUMERIC(18,2), seller_id, inventory, attributes JSONB, deleted_at)`
- `validate_order_events(order_id PK, success, status, processed, restored, created_at)` – inventory validation result **and** outbox row; `restored` makes inventory release idempotent

## order-service
- `orders(id, buyer_id, status, total_price NUMERIC(18,2), created_at, updated_at)` index `(buyer_id, status)`
- `order_items(id, order_id, product_id, quantity, price NUMERIC(18,2) unit-price snapshot, status ACTIVE|CANCELED)`
- `create_order_events(order_id PK, items JSON, status)` and `cancel_order_events(order_id PK, items JSON, status)` – outboxes

## Order status machine
```
PENDING ──▶ SUCCESS ──▶ CANCELED
   └──────▶ FAILED
```
FAILED and CANCELED are terminal. Transitions are conditional `UPDATE … WHERE status IN (…)` statements (`OrderRepository.TransitionStatus`, `CancelOrderByID`), so duplicates and out-of-order events are no-ops.

## Outbox status
`PENDING` (new) → `SUCCESS` (published) / `FAILED` (publish failed, retried). Publication happens at least once; consumers de-duplicate.

## Money
Postgres `NUMERIC(18,2)`; protobuf/JSON `double`; arithmetic in integer cents (`toMinor`/`fromMinor`). Prices are always taken from the catalog at order time and snapshotted on the item.
