package service

import (
	"auth-service/pkg/dto"
	"context"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

func TestEnsureAdminRejectsCredentialsThatCouldNeverLogIn(t *testing.T) {
	s := &AuthService{ZapLogger: zap.NewNop()} // validation happens before any DB access
	for name, c := range map[string]struct{ user, pass string }{
		"short password":     {"root_admin", "short"},
		"password too long":  {"root_admin", "this_password_is_way_too_long"},
		"symbols in pass":    {"root_admin", "pass-word-123"},
		"username too short": {"ab", "password123"},
		"empty":              {"", ""},
	} {
		if _, err := s.EnsureAdmin(context.Background(), c.user, c.pass); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestEnsureAdminCreatesOnceAndAdminCanLogIn(t *testing.T) {
	s := testService(t)
	ctx := context.Background()

	created, err := s.EnsureAdmin(ctx, "root_admin", "password123")
	if err != nil || !created {
		t.Fatalf("first call: created=%v err=%v", created, err)
	}
	// Idempotent, and a different password for an existing admin is ignored (no silent takeover)
	created, err = s.EnsureAdmin(ctx, "root_admin", "another_pass1")
	if err != nil || created {
		t.Fatalf("second call: created=%v err=%v", created, err)
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "root_admin", Password: "another_pass1", Role: "admin"}); err == nil {
		t.Error("existing admin's password must not be replaced by bootstrap")
	}

	out, err := s.Login(ctx, &dto.LoginInput{Username: "root_admin", Password: "password123", Role: "admin"})
	if err != nil {
		t.Fatalf("admin login: %v", err)
	}
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(out.AccessToken, &claims, func(*jwt.Token) (any, error) { return []byte(testSecret), nil }); err != nil {
		t.Fatal(err)
	}
	if claims["Role"] != "admin" || claims["Type"] != "access" {
		t.Errorf("unexpected claims: %v", claims)
	}
	acc, _ := s.AccountRepo.GetAccountByUsernameRole(ctx, "root_admin", "admin")
	if acc.StoreID != 0 || !strings.HasPrefix(acc.Password, "$2") {
		t.Errorf("admin must have no store and a bcrypt password: %+v", acc)
	}
}

func TestAdminCanNeverBeRegisteredThroughTheAPI(t *testing.T) {
	s := testService(t)
	if err := register(t, s, "sneaky", "password123", "admin"); err == nil || !strings.Contains(err.Error(), "invalid role") {
		t.Fatalf("registering admin must be rejected, got %v", err)
	}
	// ... and an admin password does not log in under another role
	if _, err := s.EnsureAdmin(context.Background(), "root_admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(context.Background(), &dto.LoginInput{Username: "root_admin", Password: "password123", Role: "buyer"}); err == nil {
		t.Error("admin credentials must not work for the buyer role")
	}
}
