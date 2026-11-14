// Package agent implements ADR-42: short-lived tokens for the AI agent and signed action proposals the user confirms.
// Everything is signed with keys only the gateway holds (never JWT_SECRET), so a user token can never pass as an agent
// token or a proposal and the agent service cannot forge either.
package agent

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var kidPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,16}$`)

const minSecretLen = 32

// Key is one signing secret with its id (the JWT header `kid`).
type Key struct {
	ID     string
	Secret []byte
}

// KeyRing holds the keys for rotation: the first one signs, all of them verify. Rotation = put a new key first, keep the
// old one for longer than the longest token/proposal lifetime, then drop it (docs/instruction/03_RUNBOOK.md §18).
type KeyRing struct{ keys []Key }

// ParseKeyRing reads "kid:secret,kid:secret" (the first entry is the active signing key). An empty spec gives an empty
// ring, which means the agent features are off.
func ParseKeyRing(spec string) (*KeyRing, error) {
	ring := &KeyRing{}
	seen := map[string]bool{}
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		kid, secret, ok := strings.Cut(item, ":")
		if !ok || !kidPattern.MatchString(kid) {
			return nil, errors.New("agent keys: each entry must look like kid:secret (kid = 1-16 letters, digits, _ or -)")
		}
		if len(secret) < minSecretLen {
			return nil, fmt.Errorf("agent keys: secret of %q is too short (min %d characters)", kid, minSecretLen)
		}
		if seen[kid] {
			return nil, fmt.Errorf("agent keys: duplicate kid %q", kid)
		}
		seen[kid] = true
		ring.keys = append(ring.keys, Key{ID: kid, Secret: []byte(secret)})
	}
	return ring, nil
}

// Enabled reports whether there is at least one key.
func (r *KeyRing) Enabled() bool { return r != nil && len(r.keys) > 0 }

// Active returns the signing key.
func (r *KeyRing) Active() (Key, bool) {
	if !r.Enabled() {
		return Key{}, false
	}
	return r.keys[0], true
}

// Lookup finds a verification key by id.
func (r *KeyRing) Lookup(kid string) (Key, bool) {
	if r == nil {
		return Key{}, false
	}
	for _, k := range r.keys {
		if k.ID == kid {
			return k, true
		}
	}
	return Key{}, false
}

// IDs lists the key ids (active first) for the start-up log; never the secrets.
func (r *KeyRing) IDs() []string {
	var out []string
	if r != nil {
		for _, k := range r.keys {
			out = append(out, k.ID)
		}
	}
	return out
}
