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
// If neither Redis nor the database can answer, the request is refused (fail closed) with 503, not 401, so clients retry
// instead of treating it as "logged out".

const pwdSafeAfterKey = "pwd_version:safe_after"

var (
	errPwdChanged   = errors.New("pwd version changed")
	errCannotVerify = errors.New("cannot verify pwd version")
)

// PwdVersionSource returns the current password version from the source of truth (auth-service -> database).
type PwdVersionSource interface {
	GetPwdVersion(ctx context.Context, userID uint64) (int64, error)
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
			// Redis is fresh or lost its data: deny entries of the last window may be gone. Start the window, ask the database.
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
	if perr != nil || int64(n) != tokenVersion {
		return errPwdChanged
	}
	return nil
}
