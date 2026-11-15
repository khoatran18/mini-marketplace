package agent

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	TypeAgent        = "agent"
	TypeProposal     = "proposal"
	AudienceAgent    = "gateway-agent"
	AudienceProposal = "gateway-proposal"
	ActorAgent       = "ai-agent"
	// SystemRole is the only role a system token has: the same READ-ONLY ops tools as an operator (ADR-47).
	SystemRole = "operator"
)

var (
	ErrDisabled = errors.New("agent keys are not configured")
	ErrInvalid  = errors.New("invalid or expired token")
)

// Claims is both the agent token and the signed proposal; Typ tells them apart (and the audience differs, so one can
// never be presented as the other).
type Claims struct {
	Username   string `json:"username,omitempty"`
	Role       string `json:"role,omitempty"`
	PwdVersion int64  `json:"pwd_version,omitempty"`
	// Sys names a system identity (ADR-47): the token belongs to no account, has subject 0, is never checked against a
	// password version, works only on /ops reads and can never ask for a proposal.
	Sys string            `json:"sys,omitempty"`
	Typ string            `json:"typ"`
	Act map[string]string `json:"act,omitempty"`
	// Proposal only: what the user is asked to confirm. The body is bound by hash, so the agent or the UI cannot change
	// an amount or an id after the proposal was signed.
	RunID    string `json:"run,omitempty"`
	Method   string `json:"method,omitempty"`
	Path     string `json:"path,omitempty"`
	BodyHash string `json:"body_sha256,omitempty"`
	Summary  string `json:"summary,omitempty"`
	jwt.RegisteredClaims
}

// IsSystem reports a token issued by IssueSystemToken.
func (c *Claims) IsSystem() bool { return c.Sys != "" }

// UserID returns the numeric subject.
func (c *Claims) UserID() uint64 {
	id, _ := strconv.ParseUint(c.Subject, 10, 64)
	return id
}

// Signer issues and verifies agent tokens and proposals.
type Signer struct {
	Keys *KeyRing
	Now  func() time.Time
}

func (s *Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Signer) sign(c *Claims) (string, error) {
	key, ok := s.Keys.Active()
	if !ok {
		return "", ErrDisabled
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	t.Header["kid"] = key.ID
	return t.SignedString(key.Secret)
}

// IssueAgentToken makes the short-lived token the gateway hands to ai-agent instead of the user's JWT. The token is bound
// to the user and role of the verified JWT; its jti is the run id used everywhere to trace the chat. Returns token and jti.
func (s *Signer) IssueAgentToken(userID uint64, username, role string, pwdVersion int64, ttl time.Duration) (string, string, error) {
	now := s.now()
	jti := uuid.NewString()
	c := &Claims{
		Username: username, Role: role, PwdVersion: pwdVersion, Typ: TypeAgent,
		Act: map[string]string{"sub": ActorAgent},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatUint(userID, 10), Audience: jwt.ClaimStrings{AudienceAgent}, ID: jti,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok, err := s.sign(c)
	return tok, jti, err
}

// IssueSystemToken makes the token of a system identity (ADR-47), used by the RCA watcher to investigate a critical incident when
// no person is logged in. It carries role SystemRole and no account: subject "0", no pwd_version. Returns token and jti.
func (s *Signer) IssueSystemToken(name string, ttl time.Duration) (string, string, error) {
	now := s.now()
	jti := uuid.NewString()
	c := &Claims{
		Role: SystemRole, Sys: name, Typ: TypeAgent, Username: "system:" + name,
		Act: map[string]string{"sub": ActorAgent},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "0", Audience: jwt.ClaimStrings{AudienceAgent}, ID: jti,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok, err := s.sign(c)
	return tok, jti, err
}

// SignProposal signs an action proposal made under the agent run runID for one user.
func (s *Signer) SignProposal(userID uint64, runID, method, path, bodyHash, summary string, ttl time.Duration) (string, *Claims, error) {
	now := s.now()
	c := &Claims{
		Typ: TypeProposal, RunID: runID, Method: method, Path: path, BodyHash: bodyHash, Summary: summary,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatUint(userID, 10), Audience: jwt.ClaimStrings{AudienceProposal}, ID: uuid.NewString(),
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok, err := s.sign(c)
	return tok, c, err
}

// Verify checks signature (key chosen by kid, HS256 only), expiry, audience and type.
func (s *Signer) Verify(token, typ string) (*Claims, error) {
	if !s.Keys.Enabled() {
		return nil, ErrDisabled
	}
	aud := AudienceAgent
	if typ == TypeProposal {
		aud = AudienceProposal
	}
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		k, ok := s.Keys.Lookup(kid)
		if !ok {
			return nil, fmt.Errorf("unknown kid")
		}
		return k.Secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithAudience(aud), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.now))
	if err != nil || c.Typ != typ || c.ID == "" || (c.UserID() == 0 && !(typ == TypeAgent && c.IsSystem())) {
		return nil, ErrInvalid
	}
	return c, nil
}
