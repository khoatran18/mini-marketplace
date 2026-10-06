package middleware

import (
	"context"
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

		if statusCode >= 500 {
			logger.Error("Error request failed with sever error", logFields...)
		} else if statusCode >= 400 {
			logger.Warn("Error request failed with client error", logFields...)
		} else {
			logger.Info("Request success", logFields...)
		}
	}
}

// RateLimitingMiddleware solve problem about CORS
func RateLimitingMiddleware(limit int, period time.Duration, logger *zap.Logger, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := fmt.Sprintf("rate_limit_ip:%s", c.ClientIP())
		count, err := rdb.Incr(context.Background(), key).Result()
		if err != nil {
			logger.Error("Middleware: error incrementing rate limit",
				zap.Error(err),
			)
			c.Next()
			return
		}

		// New IP or expired time
		if count == 1 {
			_, err := rdb.Expire(context.Background(), key, period).Result()
			if err != nil {
				logger.Error("Middleware: warn error setting expired key",
					zap.Error(err),
					zap.String("key", key),
				)
				rdb.Del(context.Background(), key)
			}
		}

		// Limit
		if count > int64(limit) {
			logger.Warn("Middleware: warn too many requests",
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

// AuthMiddleware solve problem about jwt
func AuthMiddleware(logger *zap.Logger, redisClient *redis.Client, jwtSecret string) gin.HandlerFunc {
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
			c.Set("userID", claims.UserID)
			c.Set("userRole", claims.Role)

			// Check logic for change password
			keyPwdVersion := fmt.Sprintf("%d:pwd_version", claims.UserID)
			exist, err := redisClient.Exists(context.Background(), keyPwdVersion).Result()
			if err != nil {
				logger.Warn("Middleware: error checking existence of pwdVersion")
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Error checking existence of pwdVersion"})
				c.Abort()
				return
			}
			if exist > 0 {
				valueStr := redisClient.Get(context.Background(), keyPwdVersion).Val()
				value, err := strconv.Atoi(valueStr)
				if err != nil {
					logger.Warn("Middleware: warn invalid or expired pwd version", zap.Error(err))
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired pwd version"})
					c.Abort()
					return
				}
				if value != int(claims.PwdVersion) {
					fmt.Printf("%d : %d \n", value, claims.PwdVersion)
					logger.Warn("Middleware: warn invalid or expired pwd version")
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired pwd version"})
					c.Abort()
					return
				}

				logger.Info(fmt.Sprintf("Middleware: auth success with userID %v, pwdVersion %d", claims.UserID, claims.PwdVersion))
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
