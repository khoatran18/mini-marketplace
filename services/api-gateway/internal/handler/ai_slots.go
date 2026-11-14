package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// SlotStore gives each user at most one running chat stream at a time (every stream can cost model money, and a second tab or a
// script must not multiply it). Acquire returns ok=false when the user already has one; an error means "cannot tell" and the caller
// must refuse rather than allow.
type SlotStore interface {
	Acquire(ctx context.Context, userID string, ttl time.Duration) (token string, ok bool, err error)
	Release(ctx context.Context, userID, token string)
}

// RedisSlots keeps the slot in Redis (SET NX with a TTL, so a crashed gateway never blocks a user for longer than ttl), shared by
// all gateway replicas. Release only deletes the slot if it still holds our token.
type RedisSlots struct{ RDB *redis.Client }

var releaseSlot = redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) end return 0`)

func slotKey(userID string) string { return "ai_slot:" + userID }

func (r RedisSlots) Acquire(ctx context.Context, userID string, ttl time.Duration) (string, bool, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", false, fmt.Errorf("slot token: %w", err)
	}
	token := hex.EncodeToString(b)
	ok, err := r.RDB.SetNX(ctx, slotKey(userID), token, ttl).Result()
	if err != nil {
		return "", false, err
	}
	return token, ok, nil
}

func (r RedisSlots) Release(ctx context.Context, userID, token string) {
	_ = releaseSlot.Run(ctx, r.RDB, []string{slotKey(userID)}, token).Err()
}
