package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// KillState is the emergency switch for every AI feature (chat, tool calls, proposals, the RCA watcher). It lives in Redis so
// every gateway replica sees a change at once, without a deploy. Off (no key) is the normal state.
type KillState struct {
	On     bool      `json:"on"`
	By     string    `json:"by,omitempty"` // "<role>:<userID>" of whoever flipped it
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at,omitempty"`
}

// Kill is where the switch is kept. State returns an error when it cannot tell (Redis down): callers then treat the agent as OFF,
// exactly like the chat slot and the rate limits, which already refuse when Redis is unreachable.
type Kill interface {
	State(ctx context.Context) (KillState, error)
	Set(ctx context.Context, st KillState) error
	Clear(ctx context.Context) error
}

const killKey = "ai_kill"

// RedisKill keeps the switch in one Redis key (no TTL: it stays on until somebody turns it off).
type RedisKill struct{ RDB *redis.Client }

func (r RedisKill) State(ctx context.Context) (KillState, error) {
	raw, err := r.RDB.Get(ctx, killKey).Result()
	if errors.Is(err, redis.Nil) {
		return KillState{}, nil
	}
	if err != nil {
		return KillState{}, err
	}
	var st KillState
	if json.Unmarshal([]byte(raw), &st) != nil { // a value we cannot read still means "somebody switched it on"
		return KillState{On: true, Reason: "unreadable switch value"}, nil
	}
	st.On = true
	return st, nil
}

func (r RedisKill) Set(ctx context.Context, st KillState) error {
	st.On = true
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return r.RDB.Set(ctx, killKey, raw, 0).Err()
}

func (r RedisKill) Clear(ctx context.Context) error { return r.RDB.Del(ctx, killKey).Err() }

// Blocked reports whether agent traffic must be refused right now. A nil Kill (no Redis, tests) never blocks. An error blocks
// (fail closed): every call the agent makes can cost money or touch data, so "cannot tell" is treated as "off".
func Blocked(ctx context.Context, k Kill) (KillState, bool) {
	if k == nil {
		return KillState{}, false
	}
	st, err := k.State(ctx)
	if err != nil {
		return KillState{On: true, Reason: "kill switch state unavailable"}, true
	}
	return st, st.On
}
