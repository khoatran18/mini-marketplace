# Security

## Authentication
- bcrypt password hashes; password ≥ 8 chars on register/change.
- HS256 JWT signed with `JWT_SECRET` (≥ 16 chars enforced at start in every service). Rotate by changing the secret everywhere (all sessions end).
- Gateway accepts only `Type=="access"` tokens. Refresh tokens are only accepted by auth-service, which also compares `PwdVersion` with the DB.
- Password change bumps `pwd_version`; the gateway caches the new version in Redis for `JWT_EXPIRE_TIME + 1 min` (= access token lifetime) so older access tokens are rejected immediately. A missing key is fine: tokens older than the TTL are expired anyway.

## Administrator role
`admin` is a fourth role that can **never** be self-registered: `Register` (gateway and auth-service) rejects it, and the protobuf `RegisterRequest` does not allow it. The only way to create one is `ADMIN_BOOTSTRAP_USERNAME` / `ADMIN_BOOTSTRAP_PASSWORD` read by auth-service at start-up (idempotent; an existing admin's password is never overwritten; credentials must satisfy the login contract: 3–16 characters `[a-zA-Z0-9_]`, password ≥ 8). Only `Login` and `ChangePassword` accept `admin`. Admin routes live under `/admin/*` (`AuthMiddleware` + role `admin`). Change the bootstrap password after the first login and remove the variables from the environment afterwards.

## Authorization rules (enforced in the gateway handlers, covered by tests)
- Identity (`userID`, username, role, and the seller's store via auth-service) comes from the token only; the route table is in `internal/router/router.go` and every row is covered by `router_test.go` ([testing.md](testing.md)). Authentication is attached **per route/group**, never globally (ADR-19).
- Orders, carts, addresses, payments: buyer role, scoped to own data; a foreign order/payment/address → 404. Sellers see and act only on orders of their own store (`store_id` from the token); a seller cannot `confirm_received`/`return`, a buyer cannot `ship`.
- Buyer profile: id must equal own user id. Store (seller) management: caller's store must equal the path id. Product writes, status, stock adjustments and ledger: owner = caller's store. Only an admin can ban a product.
- Public catalog: anonymous visitors only see `active` products; stock is capped at "20+" and `reserved`/thresholds are hidden unless you manage the product.
- Prices, totals, shipping fees, statuses, buyer/store ids are never read from clients; checkout requires an `Idempotency-Key`.
- Analytics: the report scope (platform vs one store) is derived from the token in the gateway **and** re-checked by analytics-service; query parameters can not widen it. Admin-only reports (`traffic`, `payments`, `search_terms`, `data_health`) are refused for a store scope.

## Payments (simulated) and tracking privacy
- Payments are a SIMULATION: no real card network. Card numbers only choose the simulated outcome; **only the last 4 digits** are stored, never the full number/CVV, and the gateway never echoes a request body that may contain card data.
- The simulated provider notifies `POST /internal/payments/webhook` on the internal admin port only (not routed by Traefik), signed `X-Signature: t=<unix>,v1=HMAC_SHA256(PAYMENT_WEBHOOK_SECRET, t + "." + body)`, 5-minute tolerance, idempotent per `provider_event_id`. `PAYMENT_WEBHOOK_SECRET` must be ≥ 16 characters or the service does not start. `POST /admin/dev/payments/:id/force` works only with `ENV=dev`.
- `POST /events` is public and rate limited (`EVENTS_RATE_LIMIT_PER_MINUTE`); the browser cannot set `user_id`/`role`/`store_id`/IP (the gateway attaches them). IPs are never stored: only `HMAC(IP_HASH_SECRET, UTC date | ip)`, which changes daily. Without analytics consent only anonymous `page_view`/`error_client` are kept (identifiers dropped). E-mail addresses/phone numbers in free text (search queries) are masked, URLs lose query strings. Raw events expire after `EVENTS_RETENTION_MONTHS` (13). **Not implemented**: user data erasure (`DELETE /users/me/data`), a `consents` store.

## Abuse protection
- Redis rate limiter (global `RATE_LIMIT_PER_MINUTE`, default 300; `/auth/*` `AUTH_RATE_LIMIT_PER_MINUTE`, default 20; `/events*` `EVENTS_RATE_LIMIT_PER_MINUTE`, default 120) per client IP, atomic Lua script, fails open if Redis is down.
- `TRUSTED_PROXIES` (comma list, default none) controls whether `X-Forwarded-For` is honored; set it to the Traefik network CIDR in production or every client appears as the proxy IP.
- CORS allow-list `ALLOWED_ORIGINS` (default `http://localhost:3000`).

## Secrets
Nothing sensitive is committed. Use `.env.example` templates; real values live in `deploy/.env` (ignored) or Docker secrets. TLS certs are generated locally (`scripts/gen-dev-certs.sh`) and ignored. **Anything committed before this cleanup (old JWT secret, DB/Elastic passwords, Traefik htpasswd, TLS key) is still in git history – rotate all of it.**

## Known gaps / follow-ups
- **Credential format is capped by the protobuf contract**: usernames and passwords must match `^[a-zA-Z0-9_]{3,16}$` (`auth.proto`); the gateway/auth additionally require ≥ 8 password characters. A 16-character alphanumeric maximum is weak; relax it to e.g. 8–72 chars (bcrypt limit) by editing `services/auth-service/pkg/proto/auth.proto` and re-running `buf generate` (needs access to buf.build).
- Tokens are stored in `localStorage` by the frontend (XSS exposure); prefer httpOnly cookies.
- gRPC between services and Postgres connections are plaintext inside the overlay network (`sslmode=disable`).
- No account lockout, `audit_log`, email verification, password reset or logout/deny-list (not implemented, see roadmap). `accounts.username` is unique globally, so the same username under two roles is a DB error.
- ClickHouse, Prometheus and the admin port `:8081` have no authentication; they are reachable only inside the overlay network (never publish them). Grafana uses a password (`GRAFANA_ADMIN_PASSWORD`, no anonymous access).
- A manual admin refund (`POST /admin/payments/:id/refund`) is not linked to an order, so `orders.payment_status` is not updated by it.
- Request bodies are capped by `MAX_BODY_BYTES` (default 1 MiB, HTTP 413 when `Content-Length` is larger; undeclared/chunked bodies fail while reading).
- Traefik dashboard is protected only by basic auth (`TRAEFIK_DASHBOARD_USERS`).
