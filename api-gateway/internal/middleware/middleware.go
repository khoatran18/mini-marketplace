package middleware

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type UserClaims struct {
	UserId string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

var (
	rdb    *redis.Client
	logger *zap.Logger
)

// InitMiddleware setup zap logger and redis
func InitMiddleware() {

	// Zap logger setup
	cfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       true,
		DisableCaller:     false,
		DisableStacktrace: true,
		Encoding:          "json",
		EncoderConfig:     zap.NewProductionEncoderConfig(),
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.TimeKey = "timestamp"

	var err error
	logger, err = cfg.Build()
	if err != nil {
		panic(fmt.Sprintf("cannot initialize zap logger: %v", err))
	}

	logger.Info("Initialize zap successfully!")

	// Redis setup
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
		logger.Info("Redis address not set, using defaul localhost:6379")
	}
	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err = rdb.Ping(context.Background()).Result()
	if err != nil {
		logger.Fatal("Failed to connect to redis", zap.Error(err))
	}
	logger.Info("Connect successfully to Redis")
}

func RequestLoggingMiddleware() gin.HandlerFunc {
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

		if statusCode >= 500 {
			logger.Error("Error request failed with sever error", logFields...)
		} else if statusCode >= 400 {
			logger.Warn("Error request failed with client error", logFields...)
		} else {
			logger.Info("Request success", logFields...)
		}
	}
}

func RateLimitingMiddleware(limit int, period time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := fmt.Sprintf("rate_limit_ip:%s", c.ClientIP())
		count, err := rdb.Incr(context.Background(), key).Result()
		if err != nil {
			logger.Error("Error incrementing rate limit",
				zap.Error(err),
				zap.String("key", key),
			)
			c.Next()
			return
		}

		// New IP or expired time
		if count == 1 {
			_, err := rdb.Expire(context.Background(), key, period).Result()
			if err != nil {
				logger.Error("Error setting expired key",
					zap.Error(err),
					zap.String("key", key),
				)
				rdb.Del(context.Background(), key)
			}
		}

		// Limit
		if count > int64(limit) {
			logger.Warn("Warn too many requests",
				zap.String("key", key),
				zap.Int("limit", limit),
				zap.Int64("count", count),
			)
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			c.Abort()
			return
		}

		c.Writer.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Writer.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limit-int(count)))
		c.Next()
	}
}

func AuthMiddleware() gin.HandlerFunc {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return func(c *gin.Context) {
			logger.Fatal("Error cannot load JWT_SECRET in .env")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "JWT_SECRET is not configured"})
			c.Abort()
		}
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("Warm Authorization header is required")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("Warn wrong Authorization header format")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Wrong authorization header"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				logger.Error("Error unexpected signing method",
					zap.String("alg", token.Header["alg"].(string)),
				)
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil {
			logger.Warn("Warn invalid or expired token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
			c.Set("userID", claims.UserId)
			c.Set("userRole", claims.Role)
			c.Next()
		} else {
			logger.Warn("Warn invalid token claims")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

	}
}

func AuthorizationMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exist := c.Get("userRole")
		if !exist {
			logger.Warn("Warn role is required")
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
			logger.Warn("Warn role does not have permission")
			c.JSON(http.StatusForbidden, gin.H{"error": "Do not have permission to access"})
			c.Abort()
			return
		}

		c.Next()
	}
}
