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

func TestDevRoleIsBootstrapOnlyAndLogsInWithRoleDev(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := register(t, s, "sneaky", "password123", "dev"); err == nil || !strings.Contains(err.Error(), "invalid role") {
		t.Fatalf("registering dev must be rejected, got %v", err)
	}
	if created, err := s.EnsureDev(ctx, "ops_dev", "password123"); err != nil || !created {
		t.Fatalf("EnsureDev: created=%v err=%v", created, err)
	}
	if created, err := s.EnsureDev(ctx, "ops_dev", "another_pass1"); err != nil || created {
		t.Fatalf("EnsureDev must be idempotent: created=%v err=%v", created, err)
	}
	out, err := s.Login(ctx, &dto.LoginInput{Username: "ops_dev", Password: "password123", Role: "dev"})
	if err != nil {
		t.Fatalf("dev login: %v", err)
	}
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(out.AccessToken, &claims, func(*jwt.Token) (any, error) { return []byte(testSecret), nil }); err != nil {
		t.Fatal(err)
	}
	if claims["Role"] != "dev" {
		t.Errorf("unexpected claims: %v", claims)
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "ops_dev", Password: "password123", Role: "admin"}); err == nil {
		t.Error("dev credentials must not work for the admin role")
	}
}

func TestStaffAccountManagement(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.EnsureAdmin(ctx, "root_admin", "password123"); err != nil {
		t.Fatal(err)
	}
	root, _ := s.AccountRepo.GetAccountByUsername(ctx, "root_admin")

	if err := s.CreateStaffAccount(ctx, "ops_dev", "password123", "dev"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateStaffAccount(ctx, "ops_dev", "password123", "admin"); err == nil {
		t.Error("duplicate username must be refused")
	}
	if err := s.CreateStaffAccount(ctx, "shopper", "password123", "buyer"); err == nil {
		t.Error("buyer is not a staff role")
	}
	if err := s.CreateStaffAccount(ctx, "short_pw", "short", "dev"); err == nil {
		t.Error("short password accepted")
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "ops_dev", Password: "password123"}); err != nil {
		t.Fatalf("new dev can not log in: %v", err)
	}

	dev, _ := s.AccountRepo.GetAccountByUsername(ctx, "ops_dev")
	// A buyer can never be removed through this path
	_ = register(t, s, "alice", "password123", "buyer")
	alice, _ := s.AccountRepo.GetAccountByUsername(ctx, "alice")
	if err := s.DeleteStaffAccount(ctx, alice.ID, root.ID); err == nil {
		t.Error("buyer account deleted through the staff path")
	}
	// Not yourself, not the last admin
	if err := s.DeleteStaffAccount(ctx, root.ID, root.ID); err == nil {
		t.Error("admin deleted themselves")
	}
	if err := s.DeleteStaffAccount(ctx, root.ID, dev.ID); err == nil {
		t.Error("last admin deleted")
	}

	if err := s.ResetStaffPassword(ctx, dev.ID, root.ID, "newpassword1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "ops_dev", Password: "password123"}); err == nil {
		t.Error("old password still works after reset")
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "ops_dev", Password: "newpassword1"}); err != nil {
		t.Errorf("new password rejected: %v", err)
	}

	if err := s.DeleteStaffAccount(ctx, dev.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "ops_dev", Password: "newpassword1"}); err == nil {
		t.Error("deleted account can still log in")
	}
	// The username is free again
	if err := s.CreateStaffAccount(ctx, "ops_dev", "password123", "dev"); err != nil {
		t.Errorf("username not reusable after delete: %v", err)
	}
	accs, err := s.ListAccounts(ctx, "", "ops", 10)
	if err != nil || len(accs) != 1 {
		t.Errorf("list: %v %v", accs, err)
	}
}
