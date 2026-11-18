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
	Scope  string    `json:"scope,omitempty"` // set by Active when the scope key (not the global one) is what blocks
	At     time.Time `json:"at,omitempty"`
}

// The switch has a global key and one key per scope, so an operator can stop one persona without stopping the rest (a leaked
// buyer prompt does not have to take the RCA watcher down). Global on = everything off, whatever the scope keys say.
const (
	ScopeAll      = "all" // the original global switch
	ScopeBuyer    = "buyer"
	ScopeSeller   = "seller" // seller_admin and seller_employee
	ScopeAdmin    = "admin"
	ScopeOperator = "operator"
	ScopeRCA      = "rca" // the RCA watcher and its investigation chat (system token and /ops/rca routes)
)

// KillScopes lists the scopes that can be switched on their own, in the order the UI shows them.
var KillScopes = []string{ScopeBuyer, ScopeSeller, ScopeAdmin, ScopeOperator, ScopeRCA}

// ValidKillScope accepts "all" and the five scopes.
func ValidKillScope(s string) bool {
	if s == ScopeAll {
		return true
	}
	for _, k := range KillScopes {
		if s == k {
			return true
		}
	}
	return false
}

// ScopeOfRole maps the role in a token or JWT to its switch scope. A role that is not known maps to "" and is only subject to the
// global switch (it cannot reach an agent route anyway: the scopes table has no entry for it).
func ScopeOfRole(role string) string {
	switch role {
	case "buyer":
		return ScopeBuyer
	case "seller_admin", "seller_employee":
		return ScopeSeller
	case "admin":
		return ScopeAdmin
	case "operator":
		return ScopeOperator
	}
	return ""
}

// ScopeOfClaims is ScopeOfRole for an agent token. A system token (the RCA watcher) has role "operator" (same read tools) but belongs to
// the rca scope, so a stopped operator chat does not stop it and the other way round.
func ScopeOfClaims(c *Claims) string {
	if c.IsSystem() {
		return ScopeRCA
	}
	return ScopeOfRole(c.Role)
}

// Kill is where the switch is kept. State returns an error when it cannot tell (Redis down): callers then treat the agent as OFF,
// exactly like the chat slot and the rate limits, which already refuse when Redis is unreachable.
type Kill interface {
	// State reads exactly one key: ScopeAll (or "") for the global switch, or one scope.
	State(ctx context.Context, scope string) (KillState, error)
	Set(ctx context.Context, scope string, st KillState) error
	Clear(ctx context.Context, scope string) error
	// Active reads the global key and, if scope is not "", that scope's key in ONE round trip; the global one wins.
	Active(ctx context.Context, scope string) (KillState, error)
}

const killKey = "ai_kill"

func keyOf(scope string) string {
	if scope == "" || scope == ScopeAll {
		return killKey
	}
	return killKey + ":" + scope
}

// RedisKill keeps each switch in one Redis key (no TTL: it stays on until somebody turns it off).
type RedisKill struct{ RDB *redis.Client }

func decode(raw string) KillState {
	var st KillState
	if json.Unmarshal([]byte(raw), &st) != nil { // a value we cannot read still means "somebody switched it on"
		return KillState{On: true, Reason: "unreadable switch value"}
	}
	st.On = true
	return st
}

func (r RedisKill) State(ctx context.Context, scope string) (KillState, error) {
	raw, err := r.RDB.Get(ctx, keyOf(scope)).Result()
	if errors.Is(err, redis.Nil) {
		return KillState{}, nil
	}
	if err != nil {
		return KillState{}, err
	}
	return decode(raw), nil
}

func (r RedisKill) Active(ctx context.Context, scope string) (KillState, error) {
	keys := []string{killKey}
	if scope != "" && scope != ScopeAll {
		keys = append(keys, keyOf(scope))
	}
	vals, err := r.RDB.MGet(ctx, keys...).Result()
	if err != nil {
		return KillState{}, err
	}
	for i, v := range vals {
		if s, ok := v.(string); ok {
			st := decode(s)
			if i > 0 {
				st.Scope = scope
			}
			return st, nil
		}
	}
	return KillState{}, nil
}

func (r RedisKill) Set(ctx context.Context, scope string, st KillState) error {
	st.On = true
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return r.RDB.Set(ctx, keyOf(scope), raw, 0).Err()
}

func (r RedisKill) Clear(ctx context.Context, scope string) error {
	return r.RDB.Del(ctx, keyOf(scope)).Err()
}

// Blocked reports whether agent traffic of this scope must be refused right now ("" = only the global switch is looked at). A nil Kill
// (no Redis, tests) never blocks. An error blocks (fail closed): every call the agent makes can cost money or touch data, so "cannot
// tell" is treated as "off".
func Blocked(ctx context.Context, k Kill, scope string) (KillState, bool) {
	if k == nil {
		return KillState{}, false
	}
	st, err := k.Active(ctx, scope)
	if err != nil {
		return KillState{On: true, Reason: "kill switch state unavailable"}, true
	}
	return st, st.On
}
