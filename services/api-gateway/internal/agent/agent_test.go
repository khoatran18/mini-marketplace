package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	secA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-A"
	secB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-B"
)

func ring(t *testing.T, spec string) *KeyRing {
	t.Helper()
	r, err := ParseKeyRing(spec)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParseKeyRing(t *testing.T) {
	if r := ring(t, ""); r.Enabled() {
		t.Error("empty spec must disable the feature")
	}
	for _, bad := range []string{"nosecret", "k1:short", "bad kid:" + secA, "k1:" + secA + ",k1:" + secB} {
		if _, err := ParseKeyRing(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	r := ring(t, "k2:"+secB+", k1:"+secA)
	if k, _ := r.Active(); k.ID != "k2" {
		t.Errorf("active = %s, want the first entry k2", k.ID)
	}
	if ids := r.IDs(); len(ids) != 2 {
		t.Errorf("ids %v", ids)
	}
}

func TestAgentTokenRoundTripAndClaims(t *testing.T) {
	s := &Signer{Keys: ring(t, "k1:"+secA)}
	tok, jti, err := s.IssueAgentToken(7, "bob", "buyer", 3, 150*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Verify(tok, TypeAgent)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID() != 7 || c.Role != "buyer" || c.PwdVersion != 3 || c.ID != jti || c.Act["sub"] != ActorAgent {
		t.Errorf("unexpected claims %+v", c)
	}
	if _, err := s.Verify(tok, TypeProposal); err == nil {
		t.Error("an agent token must not pass as a proposal")
	}
}

func TestRotationKeepsOldTokensUntilTheKeyIsDropped(t *testing.T) {
	old := &Signer{Keys: ring(t, "k1:"+secA)}
	tok, _, _ := old.IssueAgentToken(1, "u", "buyer", 1, time.Minute)

	during := &Signer{Keys: ring(t, "k2:"+secB+",k1:"+secA)} // overlap: k2 signs, k1 still verifies
	if _, err := during.Verify(tok, TypeAgent); err != nil {
		t.Errorf("token signed by the previous key must verify during the overlap: %v", err)
	}
	fresh, _, _ := during.IssueAgentToken(1, "u", "buyer", 1, time.Minute)
	if _, err := (&Signer{Keys: ring(t, "k2:"+secB)}).Verify(fresh, TypeAgent); err != nil {
		t.Errorf("new tokens verify with k2 alone: %v", err)
	}
	after := &Signer{Keys: ring(t, "k2:"+secB)} // old key dropped
	if _, err := after.Verify(tok, TypeAgent); err == nil {
		t.Error("a token of a dropped key must be rejected")
	}
}

func TestRejectedTokens(t *testing.T) {
	s := &Signer{Keys: ring(t, "k1:"+secA)}
	now := time.Now()
	good := func() jwt.RegisteredClaims {
		return jwt.RegisteredClaims{Subject: "1", ID: "x", Audience: jwt.ClaimStrings{AudienceAgent}, ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}
	}
	sign := func(c Claims, kid string, secret string) string {
		tk := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
		if kid != "" {
			tk.Header["kid"] = kid
		}
		out, _ := tk.SignedString([]byte(secret))
		return out
	}
	exp := good()
	exp.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second))
	wrongAud := good()
	wrongAud.Audience = jwt.ClaimStrings{"someone-else"}
	noExp := good()
	noExp.ExpiresAt = nil
	cases := map[string]string{
		"expired":        sign(Claims{Typ: TypeAgent, RegisteredClaims: exp}, "k1", secA),
		"wrong audience": sign(Claims{Typ: TypeAgent, RegisteredClaims: wrongAud}, "k1", secA),
		"wrong typ":      sign(Claims{Typ: "access", RegisteredClaims: good()}, "k1", secA),
		"no expiry":      sign(Claims{Typ: TypeAgent, RegisteredClaims: noExp}, "k1", secA),
		"unknown kid":    sign(Claims{Typ: TypeAgent, RegisteredClaims: good()}, "k9", secA),
		"no kid":         sign(Claims{Typ: TypeAgent, RegisteredClaims: good()}, "", secA),
		"wrong secret":   sign(Claims{Typ: TypeAgent, RegisteredClaims: good()}, "k1", secB),
		"garbage":        "not-a-jwt",
	}
	for name, tok := range cases {
		if _, err := s.Verify(tok, TypeAgent); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestBodyHashIsFormatIndependent(t *testing.T) {
	a := BodyHash([]byte(`{"amount": 100, "reason":"x"}`))
	b := BodyHash([]byte("{\n \"reason\":\"x\",\"amount\":100 }"))
	if a != b {
		t.Error("same JSON in another layout/key order must hash the same")
	}
	if a == BodyHash([]byte(`{"amount":101,"reason":"x"}`)) {
		t.Error("a changed amount must change the hash")
	}
	if BodyHash(nil) != BodyHash([]byte("  ")) {
		t.Error("empty bodies hash alike")
	}
}

func TestProposableList(t *testing.T) {
	yes := [][2]string{{"POST", "/orders/12/cancel"}, {"POST", "/admin/payments/3/refund"}, {"POST", "/orders/12/return"},
		{"POST", "/seller/orders/4/cancel"}, {"POST", "/seller/orders/4/return/approve"}, {"POST", "/seller/orders/4/return/reject"},
		{"POST", "/seller/products/9/inventory/adjust"}}
	no := [][2]string{{"POST", "/orders"}, {"GET", "/orders/12/cancel"}, {"POST", "/auth/change-password"}, {"DELETE", "/admin/accounts/3"}, {"POST", "/orders/1/cancel/../../x"}, {"POST", "/orders/abc/cancel"},
		{"POST", "/seller/orders/4/ship"}, {"POST", "/seller/orders/4/deliver"}, {"POST", "/products"}, {"PATCH", "/products/9/status"}, {"POST", "/seller/products/x/inventory/adjust"}, {"GET", "/seller/products/9/inventory/adjust"}}
	for _, r := range yes {
		if !Proposable(r[0], r[1]) {
			t.Errorf("%v should be proposable", r)
		}
	}
	for _, r := range no {
		if Proposable(r[0], r[1]) {
			t.Errorf("%v must not be proposable", r)
		}
	}
}

func TestEveryActionIsProposableAndMatchesItsRole(t *testing.T) {
	for role, actions := range DefaultActions {
		for _, a := range actions {
			concrete := strings.ReplaceAll(a.Path, ":id", "12")
			if !Proposable(a.Method, concrete) {
				t.Errorf("%s/%s: %s %s is not on the proposable list", role, a.Name, a.Method, concrete)
			}
			if !AllowsAction(role, a.Method, concrete) {
				t.Errorf("%s/%s: AllowsAction should accept %s", role, a.Name, concrete)
			}
			if a.Summary == "" || len(a.Params) == 0 {
				t.Errorf("%s/%s: summary and params are required", role, a.Name)
			}
			for _, p := range a.Params {
				if p.In != "path" && p.In != "body" {
					t.Errorf("%s/%s: param %s has In=%q, want path or body", role, a.Name, p.Name, p.In)
				}
			}
		}
	}
}

func TestAllowsActionIsPerRole(t *testing.T) {
	cases := []struct {
		role, method, path string
		want               bool
	}{
		{"buyer", "POST", "/orders/12/cancel", true},
		{"buyer", "POST", "/seller/orders/12/cancel", false},
		{"buyer", "POST", "/admin/orders/12/cancel", false},
		{"seller_admin", "POST", "/seller/products/3/inventory/adjust", true},
		{"seller_employee", "POST", "/seller/orders/12/return/approve", true},
		{"seller_admin", "POST", "/orders/12/cancel", false},
		{"buyer", "POST", "/orders/12x/cancel", false},
		{"buyer", "GET", "/orders/12/cancel", false},
		{"admin", "POST", "/admin/orders/12/cancel", true},
		{"buyer", "POST", "/admin/payments/12/refund", false},
	}
	for _, c := range cases {
		if got := AllowsAction(c.role, c.method, c.path); got != c.want {
			t.Errorf("AllowsAction(%s,%s,%s) = %v, want %v", c.role, c.method, c.path, got, c.want)
		}
	}
}
