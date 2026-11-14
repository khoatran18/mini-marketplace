package middleware

import (
	"api-gateway/pkg/ops"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type UserClaims struct {
	UserID     uint64
	Username   string
	Role       string
	PwdVersion int64
	Type       string
	jwt.RegisteredClaims
}

// RequestLoggingMiddleware write logs for middleware
func RequestLoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		c.Next()

		latency := time.Since(startTime)
		statusCode := c.Writer.Status()
		userID, exist := c.Get("userID")
		if !exist {
			userID = ""
		}

		logFields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Duration("latency", latency),
			zap.Int("status", statusCode),
			zap.Any("userID", userID),
		}
		if rid, ok := c.Get("requestID"); ok {
			logFields = append(logFields, zap.Any("request_id", rid))
		}

		if statusCode >= 500 {
			logger.Error("Error request failed with sever error", logFields...)
		} else if statusCode >= 400 {
			logger.Warn("Error request failed with client error", logFields...)
		} else {
			logger.Info("Request success", logFields...)
		}
	}
}

// rateLimitScript increments the counter and sets the TTL atomically on first hit,
// so a crash between INCR and EXPIRE can never leave an immortal key.
var rateLimitScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return count
`)

// RateLimitingMiddleware limits requests per client IP within a period.
// scope separates counters (e.g. "global", "auth") so different limits do not share a key.
func RateLimitingMiddleware(scope string, limit int, period time.Duration, logger *zap.Logger, rdb *redis.Client) gin.HandlerFunc {
	return RateLimitByKey(scope, limit, period, logger, rdb, func(c *gin.Context) string { return c.ClientIP() }, false)
}

// RateLimitByKey is the counter behind every limiter. keyFn picks who is counted (IP, user id...); an empty key falls back to
// the client IP. failClosed=false lets requests through when Redis is down (a cache outage must not take the API down);
// failClosed=true answers 503 instead, for routes where a request costs real money (AI).
func RateLimitByKey(scope string, limit int, period time.Duration, logger *zap.Logger, rdb *redis.Client, keyFn func(*gin.Context) string, failClosed bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		who := keyFn(c)
		if who == "" {
			who = c.ClientIP()
		}
		key := fmt.Sprintf("rate_limit:%s:%s", scope, who)
		count, err := rateLimitScript.Run(c.Request.Context(), rdb, []string{key}, period.Milliseconds()).Int64()
		if err != nil {
			logger.Error("Middleware: error incrementing rate limit", zap.Error(err))
			if failClosed {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily unavailable, retry"})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Limit
		if count > int64(limit) {
			logger.Warn("Middleware: warn too many requests",
				zap.String("key", key),
				zap.Int("limit", limit),
				zap.Int64("count", count),
			)
			group := scope
			if group == "global" {
				group = "default"
			}
			ops.IncRateLimited(group)
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			c.Abort()
			return
		}

		c.Writer.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Writer.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limit-int(count)))
		c.Next()
	}
}

// UserKey counts per authenticated user: the JWT user (userID) or, on agent routes, the subject of the agent token.
func UserKey(c *gin.Context) string {
	if id, ok := c.Get("userID"); ok {
		if n, ok := id.(uint64); ok && n != 0 {
			return fmt.Sprintf("u%d", n)
		}
	}
	if v, ok := c.Get("agentClaims"); ok {
		if cl, ok := v.(interface{ UserID() uint64 }); ok && cl.UserID() != 0 {
			return fmt.Sprintf("u%d", cl.UserID())
		}
	}
	return ""
}

// AuthMiddleware solve problem about jwt
func AuthMiddleware(logger *zap.Logger, redisClient *redis.Client, jwtSecret string, opts ...AuthOption) gin.HandlerFunc {
	checker := &pwdChecker{rdb: redisClient, now: time.Now, logger: logger}
	for _, o := range opts {
		o(checker)
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("Middleware: warn authorization header is required")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("Middleware: warn wrong Authorization header format")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Wrong authorization header"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				logger.Error("Middleware: error unexpected signing method",
					zap.String("alg", token.Header["alg"].(string)),
				)
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil {
			logger.Warn("Middleware: warn invalid or expired token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
			if claims.Type != "access" {
				logger.Warn("Middleware: warn token is not an access token")
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token type"})
				c.Abort()
				return
			}
			c.Set("userID", claims.UserID)
			c.Set("userRole", claims.Role)
			c.Set("username", claims.Username)
			c.Set("pwdVersion", claims.PwdVersion)

			// Revocation after a password change (Redis fast path, database as source of truth; see pwdversion.go)
			if err := checker.verify(c.Request.Context(), claims.UserID, int64(claims.PwdVersion)); err != nil {
				if errors.Is(err, errPwdChanged) {
					logger.Warn("Middleware: warn invalid or expired pwd version")
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired pwd version"})
				} else {
					logger.Error("Middleware: cannot verify pwd version (Redis and database both unavailable)")
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot verify credentials right now, retry"})
				}
				c.Abort()
				return
			}

			c.Next()
		} else {
			logger.Warn("Middleware: warn invalid token claims")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

	}
}

// OptionalAuth lets anonymous requests through (no identity is set) but validates a token when one is sent:
// a bad or expired token is still a 401, never silently treated as anonymous.
func OptionalAuth(strict gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.Next()
			return
		}
		strict(c)
	}
}

// AuthorizationMiddleware solve problem about role user
func AuthorizationMiddleware(requiredRoles []string, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exist := c.Get("userRole")
		if !exist {
			logger.Warn("Middleware: warn role is required")
			c.JSON(http.StatusForbidden, gin.H{"error": "Role is required"})
			c.Abort()
			return
		}

		hasPermission := false
		for _, role := range requiredRoles {
			if role == userRole {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			logger.Warn("Middleware: warn role does not have permission")
			c.JSON(http.StatusForbidden, gin.H{"error": "Do not have permission to access"})
			c.Abort()
			return
		}

		c.Next()
	}
}
