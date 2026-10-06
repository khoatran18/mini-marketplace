package service

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache caches report JSON in Redis.
type RedisCache struct{ C *redis.Client }

// Get returns the cached value.
func (r RedisCache) Get(ctx context.Context, key string) (string, bool) {
	v, err := r.C.Get(ctx, key).Result()
	return v, err == nil
}

// Set stores a value; failures are ignored (cache only).
func (r RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) {
	_ = r.C.Set(ctx, key, value, ttl).Err()
}
