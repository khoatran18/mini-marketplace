# REST API (api-gateway)

Base URL: `https://api.<PUBLIC_HOST>` (dev: `http://localhost:8080`). Swagger: `services/api-gateway/docs/swagger.yaml`.
Auth: `Authorization: Bearer <access_token>`. Errors: `{"error": "message"}`.

| Method & path | Who | Notes |
|---|---|---|
| GET `/health` | public | liveness |
| POST `/auth/register` | public | roles `buyer` or `seller_admin`; password ≥ 8 chars; 20 req/min/IP |
| POST `/auth/login` | public | `{username,password,role}` → access + refresh token |
| POST `/auth/refresh-token` | public | refresh token only (access tokens are rejected) |
| POST `/auth/change-password` | authenticated | account taken from the token; invalidates older tokens |
| POST `/auth/register-seller-roles` | `seller_admin` | creates a `seller_employee` in the admin's store |
| POST/GET/PUT/DELETE `/users/buyers[/:id]` | `buyer` | `:id` must equal the caller's user id (403 otherwise); create is always for the caller |
| POST `/users/sellers` | `seller_admin` | binds a new store to the caller (only if the account has none) |
| GET `/users/sellers/:id` | authenticated | owner sees everything; others only `id,name,description` |
| PUT/DELETE `/users/sellers/:id` | `seller_admin` | only for the caller's own store |
| POST `/products` | seller roles | product belongs to the caller's store; name, price>0, inventory≥0 |
| PUT `/products/:id` | seller roles | caller's store must own the product; replaces name/price/inventory/attributes |
| GET `/products`, `/products/:id`, `/products/seller/:seller_id` | authenticated | `page_size` capped at 100 |
| POST `/orders` | `buyer` | body `{order:{order_items:[{product_id,quantity}]}}`; any price/status/buyer is ignored |
| GET `/orders?status=` | `buyer` | caller's orders (`PENDING|SUCCESS|FAILED|CANCELED`); `buyer_id` query is ignored |
| GET `/orders/:id` | `buyer` | 404 for orders of other buyers |
| DELETE `/orders/:id` | `buyer` | cancel; only `SUCCESS` orders, 422 otherwise |

There is deliberately no `PUT /orders/:id` (it allowed rewriting status/price/buyer).

## Error mapping (`handler/helpers.go`)
InvalidArgument→400, Unauthenticated→401, PermissionDenied→403, NotFound→404, AlreadyExists/Aborted→409, FailedPrecondition→422, Unavailable/DeadlineExceeded→503, else 500.

## Tokens
Access token lifetime `JWT_EXPIRE_TIME` minutes (default 5); refresh token 2×. Claims: `UserID, Username, Role, PwdVersion, Type(access|refresh)`.
