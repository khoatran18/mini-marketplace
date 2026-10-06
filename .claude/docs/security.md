# Security

## Authentication
- bcrypt password hashes; password ≥ 8 chars on register/change.
- HS256 JWT signed with `JWT_SECRET` (≥ 16 chars enforced at start in every service). Rotate by changing the secret everywhere (all sessions end).
- Gateway accepts only `Type=="access"` tokens. Refresh tokens are only accepted by auth-service, which also compares `PwdVersion` with the DB.
- Password change bumps `pwd_version`; the gateway caches the new version in Redis for `JWT_EXPIRE_TIME + 1 min` (= access token lifetime) so older access tokens are rejected immediately. A missing key is fine: tokens older than the TTL are expired anyway.

## Authorization rules (enforced in the gateway handlers, covered by tests)
- Identity (`userID`, username, role) comes from the token only.
- Orders: buyer role, scoped to own orders; foreign order → 404.
- Buyer profile: id must equal own user id. Store (seller) management: caller's store must equal the path id. Product writes: owner = caller's store.
- Prices/totals/status/buyer are never read from clients.

## Abuse protection
- Redis rate limiter (global `RATE_LIMIT_PER_MINUTE`, default 300; `/auth/*` `AUTH_RATE_LIMIT_PER_MINUTE`, default 20) per client IP, atomic Lua script, fails open if Redis is down.
- `TRUSTED_PROXIES` (comma list, default none) controls whether `X-Forwarded-For` is honored; set it to the Traefik network CIDR in production or every client appears as the proxy IP.
- CORS allow-list `ALLOWED_ORIGINS` (default `http://localhost:3000`).

## Secrets
Nothing sensitive is committed. Use `.env.example` templates; real values live in `deploy/.env` (ignored) or Docker secrets. TLS certs are generated locally (`scripts/gen-dev-certs.sh`) and ignored. **Anything committed before this cleanup (old JWT secret, DB/Elastic passwords, Traefik htpasswd, TLS key) is still in git history – rotate all of it.**

## Known gaps / follow-ups
- **Credential format is capped by the protobuf contract**: usernames and passwords must match `^[a-zA-Z0-9_]{3,16}$` (`auth.proto`); the gateway/auth additionally require ≥ 8 password characters. A 16-character alphanumeric maximum is weak; relax it to e.g. 8–72 chars (bcrypt limit) by editing `services/auth-service/pkg/proto/auth.proto` and re-running `buf generate` (needs access to buf.build).
- Tokens are stored in `localStorage` by the frontend (XSS exposure); prefer httpOnly cookies.
- gRPC between services and Postgres connections are plaintext inside the overlay network (`sslmode=disable`).
- No account lockout / email verification / password reset (see roadmap).
- The gateway accepts JSON bodies of unbounded size; add `http.MaxBytesReader` or a body-limit middleware.
- Traefik dashboard is protected only by basic auth (`TRAEFIK_DASHBOARD_USERS`).
