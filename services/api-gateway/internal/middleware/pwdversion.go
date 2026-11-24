package middleware

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Access tokens cannot be revoked, so a password change is enforced by comparing the token's password version with the
// current one. The current version lives in the database (auth-service, source of truth). Redis is only a fast path: after
// a change the auth-service outbox -> Kafka -> gateway consumer writes `<userID>:pwd_version` with a TTL of one access
// token lifetime (+1 min); a missing key therefore means "no change in the last token lifetime".
//
// That shortcut is only valid while Redis is trustworthy. Redis is NOT trusted (the database is asked instead) when
//   - the call to Redis fails, or
//   - Redis may have lost data: a sentinel key (pwd_version:safe_after) is missing (fresh or flushed Redis) or still in the
//     future. The sentinel is created without TTL as "now + window"; for one window after (re)creation the deny entries of
//     the last window may have been lost, so the database decides.
//
// Rebuilding instead of asking on every request: when the sentinel is missing and the database source can list recent
// changes (PwdRecentSource), the first request takes a short lock, asks auth-service ONCE for every account whose password
// changed in the last window, writes their `<userID>:pwd_version` entries back (SetNX: never overwrites a newer value the
// Kafka consumer wrote) and sets the sentinel to "now", so Redis is trusted again at once. Only when that call fails does
// the old behaviour apply: sentinel = now + window and the database decides every request until it passes.
//
// If neither Redis nor the database can answer, the request is refused (fail closed) with 503, not 401, so clients retry
// instead of treating it as "logged out".

const (
	pwdSafeAfterKey    = "pwd_version:safe_after"
	pwdRebuildLockKey  = "pwd_version:rebuild_lock"
	pwdRebuildLockTTL  = 15 * time.Second
	pwdRebuildCallWait = 10 * time.Second
)

var (
	errPwdChanged   = errors.New("pwd version changed")
	errCannotVerify = errors.New("cannot verify pwd version")
)

// PwdVersionSource returns the current password version from the source of truth (auth-service -> database).
type PwdVersionSource interface {
	GetPwdVersion(ctx context.Context, userID uint64) (int64, error)
}

// PwdChange is one account whose password changed recently, with its current version.
type PwdChange struct {
	UserID     uint64
	PwdVersion int64
}

// PwdRecentSource is optionally implemented by a PwdVersionSource: it lists the accounts whose password changed within
// the last `within`, so a Redis that lost its data can be repopulated in one call.
type PwdRecentSource interface {
	ListRecentPwdChanges(ctx context.Context, within time.Duration) ([]PwdChange, error)
}

// AuthOption configures AuthMiddleware.
type AuthOption func(*pwdChecker)

// WithPwdVersionFallback makes AuthMiddleware ask the database (through src) whenever Redis cannot be trusted.
// window must be >= the TTL of the Redis deny entries (access token lifetime + 1 minute).
func WithPwdVersionFallback(src PwdVersionSource, window time.Duration) AuthOption {
	return func(c *pwdChecker) { c.src, c.window = src, window }
}

type pwdChecker struct {
	rdb    *redis.Client
	src    PwdVersionSource
	window time.Duration
	now    func() time.Time
	logger *zap.Logger
}

func (c *pwdChecker) fromDB(ctx context.Context, userID uint64, tokenVersion int64) error {
	if c.src == nil {
		return errCannotVerify
	}
	v, err := c.src.GetPwdVersion(ctx, userID)
	if err != nil {
		c.logger.Warn("Middleware: cannot read pwd version from database", zap.Error(err))
		return errCannotVerify
	}
	if v != tokenVersion {
		return errPwdChanged
	}
	return nil
}

type rebuildResult int

const (
	rebuildUnsupported rebuildResult = iota // the source cannot list changes: use the sentinel window
	rebuildBusy                             // another request is rebuilding right now: ask the database, create nothing
	rebuildFailed                           // this request tried and failed: use the sentinel window
	rebuildDone                             // deny entries written and sentinel = now: Redis is trusted
)

// rebuild repopulates the deny entries of the last window from the database and trusts Redis again. Only one caller
// rebuilds at a time (lock); the others must NOT create the sentinel, or the window would block the rebuild's own sentinel.
func (c *pwdChecker) rebuild(ctx context.Context) rebuildResult {
	rec, ok := c.src.(PwdRecentSource)
	if !ok || c.window <= 0 {
		return rebuildUnsupported
	}
	if got, err := c.rdb.SetNX(ctx, pwdRebuildLockKey, 1, pwdRebuildLockTTL).Result(); err != nil {
		return rebuildFailed
	} else if !got {
		return rebuildBusy
	}
	defer c.rdb.Del(context.WithoutCancel(ctx), pwdRebuildLockKey)
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pwdRebuildCallWait)
	defer cancel()
	changes, err := rec.ListRecentPwdChanges(callCtx, c.window)
	if err != nil {
		c.logger.Warn("Middleware: cannot list recent password changes, falling back to the database window", zap.Error(err))
		return rebuildFailed
	}
	pipe := c.rdb.Pipeline()
	for _, ch := range changes {
		pipe.SetNX(callCtx, fmt.Sprintf("%d:pwd_version", ch.UserID), ch.PwdVersion, c.window)
	}
	// Sentinel last: only once the deny entries are in Redis is it safe to trust it.
	pipe.SetNX(callCtx, pwdSafeAfterKey, c.now().Unix(), 0)
	if _, err := pipe.Exec(callCtx); err != nil {
		c.logger.Warn("Middleware: cannot write rebuilt pwd version entries", zap.Error(err))
		return rebuildFailed
	}
	c.logger.Info("Middleware: Redis pwd version entries rebuilt from the database", zap.Int("accounts", len(changes)))
	return rebuildDone
}

// verify returns nil when the token's password version is current.
func (c *pwdChecker) verify(ctx context.Context, userID uint64, tokenVersion int64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	key := fmt.Sprintf("%d:pwd_version", userID)
	vals, err := c.rdb.MGet(ctx, pwdSafeAfterKey, key).Result()
	if err != nil {
		c.logger.Warn("Middleware: Redis unavailable for pwd version, asking the database", zap.Error(err))
		return c.fromDB(ctx, userID, tokenVersion)
	}
	if len(vals) != 2 {
		return c.fromDB(ctx, userID, tokenVersion)
	}
	// Without a database source there is nothing to fall back to: trust Redis when it answers (legacy behaviour).
	if c.src != nil {
		safeAfter, ok := int64(0), false
		if s, isStr := vals[0].(string); isStr {
			if n, perr := strconv.ParseInt(s, 10, 64); perr == nil {
				safeAfter, ok = n, true
			}
		}
		if !ok {
			// Redis is fresh or lost its data: deny entries of the last window may be gone. Rebuild them from the database
			// once; if that is not possible, start the window and let the database decide every request until it passes.
			res := c.rebuild(ctx)
			if res == rebuildDone || res == rebuildBusy {
				return c.fromDB(ctx, userID, tokenVersion) // this one request: the key it read may predate the rebuild
			}
			if c.window > 0 {
				_ = c.rdb.SetNX(ctx, pwdSafeAfterKey, c.now().Add(c.window).Unix(), 0).Err()
			}
			return c.fromDB(ctx, userID, tokenVersion)
		}
		if c.now().Unix() < safeAfter {
			return c.fromDB(ctx, userID, tokenVersion)
		}
	}
	// Redis is trustworthy: a missing key means no password change within the last token lifetime.
	s, isStr := vals[1].(string)
	if !isStr {
		return nil
	}
	n, perr := strconv.Atoi(s)
	if perr == nil && int64(n) == tokenVersion {
		return nil
	}
	// The key can be STALE: after a second change within the key's TTL it still holds the previous version until the
	// outbox event arrives, so a token issued right now would be refused. A mismatch is rare, so let the database decide.
	if c.src != nil {
		return c.fromDB(ctx, userID, tokenVersion)
	}
	return errPwdChanged
}

// PwdVerifier checks a token's password version the same way AuthMiddleware does (Redis fast path, database fallback).
type PwdVerifier struct{ c *pwdChecker }

// NewPwdVerifier builds a verifier with the same options as AuthMiddleware, for tokens that are not user JWTs (agent token).
func NewPwdVerifier(logger *zap.Logger, rdb *redis.Client, opts ...AuthOption) *PwdVerifier {
	c := &pwdChecker{rdb: rdb, now: time.Now, logger: logger}
	for _, o := range opts {
		o(c)
	}
	return &PwdVerifier{c: c}
}

// Verify returns nil when version is current; ErrPwdChanged when the password changed; any other error = cannot tell.
func (v *PwdVerifier) Verify(ctx context.Context, userID uint64, version int64) error {
	return v.c.verify(ctx, userID, version)
}

// IsPwdChanged tells "password changed" apart from "could not verify".
func IsPwdChanged(err error) bool { return errors.Is(err, errPwdChanged) }
