package service

import (
	"auth-service/internal/repository"
	"auth-service/pkg/dto"
	"auth-service/pkg/model"
	"auth-service/pkg/outbox"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testSecret = "auth-test-secret-long-enough"

// testService connects to an isolated database on TEST_POSTGRES_DSN
// (e.g. "host=localhost port=5433 user=postgres sslmode=disable"); skipped when unset.
func testService(t *testing.T) *AuthService {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	name := fmt.Sprintf("auth_test_%d", rand.Int63())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create db: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Account{}, &outbox.PwdVersionEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return &AuthService{
		AccountRepo:   repository.NewAccountRepository(db),
		JWTSecret:     testSecret,
		JWTExpireTime: time.Minute,
		ZapLogger:     zap.NewNop(),
	}
}

func register(t *testing.T, s *AuthService, username, password, role string) error {
	t.Helper()
	_, err := s.Register(context.Background(), &dto.RegisterInput{
		Username: username, Password: password, Role: role, RoleNotRegister: "seller_employee",
	})
	return err
}

func TestRegisterValidatesRoleAndPasswordBeforeTouchingTheDatabase(t *testing.T) {
	s := &AuthService{ZapLogger: zap.NewNop()} // no repository: any DB access would panic
	cases := map[string]struct{ password, role, want string }{
		"unknown role":            {"password123", "admin", "invalid role"},
		"empty role":              {"password123", "", "invalid role"},
		"employee not self-serve": {"password123", "seller_employee", "can not register role"},
		"short password":          {"short", "buyer", "at least 8 characters"},
	}
	for name, c := range cases {
		err := register(t, s, "alice", c.password, c.role)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want error containing %q", name, err, c.want)
		}
	}
}

func TestRegisterLoginAndPasswordsAreHashed(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := register(t, s, "alice", "password123", "buyer"); err != nil {
		t.Fatal(err)
	}
	if err := register(t, s, "alice", "password123", "buyer"); err == nil {
		t.Error("duplicate registration must fail")
	}

	acc, _ := s.AccountRepo.GetAccountByUsernameRole(ctx, "alice", "buyer")
	if acc.Password == "password123" || !strings.HasPrefix(acc.Password, "$2") {
		t.Errorf("password must be stored as a bcrypt hash, got %q", acc.Password)
	}

	out, err := s.Login(ctx, &dto.LoginInput{Username: "alice", Password: "password123", Role: "buyer"})
	if err != nil || out.AccessToken == "" || out.RefreshToken == "" {
		t.Fatalf("login failed: %v", err)
	}
	// Wrong password and unknown user are indistinguishable
	_, err1 := s.Login(ctx, &dto.LoginInput{Username: "alice", Password: "wrong-password", Role: "buyer"})
	_, err2 := s.Login(ctx, &dto.LoginInput{Username: "nobody", Password: "password123", Role: "buyer"})
	if err1 == nil || err2 == nil || err1.Error() != err2.Error() {
		t.Errorf("login errors must be identical: %v / %v", err1, err2)
	}
}

func TestTokenTypes(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := register(t, s, "alice", "password123", "buyer"); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Login(ctx, &dto.LoginInput{Username: "alice", Password: "password123", Role: "buyer"})

	claimsOf := func(tok string) *jwt.MapClaims {
		c := jwt.MapClaims{}
		if _, err := jwt.ParseWithClaims(tok, &c, func(*jwt.Token) (any, error) { return []byte(testSecret), nil }); err != nil {
			t.Fatal(err)
		}
		return &c
	}
	if got := (*claimsOf(out.AccessToken))["Type"]; got != "access" {
		t.Errorf("access token type = %v", got)
	}
	if got := (*claimsOf(out.RefreshToken))["Type"]; got != "refresh" {
		t.Errorf("refresh token type = %v", got)
	}

	// An access token can not be used to refresh
	if _, err := s.RefreshToken(ctx, &dto.RefreshTokenInput{RefreshToken: out.AccessToken}); err == nil {
		t.Error("access token accepted as refresh token")
	}
	if _, err := s.RefreshToken(ctx, &dto.RefreshTokenInput{RefreshToken: out.RefreshToken}); err != nil {
		t.Errorf("refresh failed: %v", err)
	}
}

func TestChangePasswordInvalidatesRefreshTokens(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := register(t, s, "alice", "password123", "buyer"); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Login(ctx, &dto.LoginInput{Username: "alice", Password: "password123", Role: "buyer"})

	// Wrong old password: generic message, nothing leaked from bcrypt
	_, err := s.ChangePassword(ctx, &dto.ChangePasswordInput{Username: "alice", Role: "buyer", OldPassword: "nope-nope", NewPassword: "newpassword1"})
	if err == nil || strings.Contains(err.Error(), "crypto/bcrypt") {
		t.Errorf("want generic error, got %v", err)
	}
	// Weak new password
	if _, err := s.ChangePassword(ctx, &dto.ChangePasswordInput{Username: "alice", Role: "buyer", OldPassword: "password123", NewPassword: "short"}); err == nil {
		t.Error("short new password accepted")
	}

	if _, err := s.ChangePassword(ctx, &dto.ChangePasswordInput{Username: "alice", Role: "buyer", OldPassword: "password123", NewPassword: "newpassword1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RefreshToken(ctx, &dto.RefreshTokenInput{RefreshToken: out.RefreshToken}); err == nil {
		t.Error("refresh token issued before the password change must be rejected")
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "alice", Password: "password123", Role: "buyer"}); err == nil {
		t.Error("old password still works")
	}
	if _, err := s.Login(ctx, &dto.LoginInput{Username: "alice", Password: "newpassword1", Role: "buyer"}); err != nil {
		t.Errorf("new password rejected: %v", err)
	}

	var events []outbox.PwdVersionEvent
	s.AccountRepo.DB.Find(&events)
	if len(events) != 1 || events[0].PwdVersion != 1 {
		t.Errorf("want one outbox event with version 1, got %+v", events)
	}
}

func TestRegisterSellerRolesOnlyCreatesEmployeesForTheAdminsStore(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := register(t, s, "boss", "password123", "seller_admin"); err != nil {
		t.Fatal(err)
	}
	boss, _ := s.AccountRepo.GetAccountByUsernameRole(ctx, "boss", "seller_admin")
	s.AccountRepo.DB.Model(boss).Update("store_id", 9)

	in := func(role string) *dto.RegisterSellerRolesInput {
		return &dto.RegisterSellerRolesInput{SellerAdminID: boss.ID, Username: "emp-" + role, Password: "password123", Role: role}
	}
	for _, role := range []string{"seller_admin", "buyer", "root"} {
		if _, err := s.RegisterSellerRoles(ctx, in(role)); err == nil {
			t.Errorf("role %q must not be creatable through register-seller-roles", role)
		}
	}
	if _, err := s.RegisterSellerRoles(ctx, in("seller_employee")); err != nil {
		t.Fatal(err)
	}
	emp, _ := s.AccountRepo.GetAccountByUsernameRole(ctx, "emp-seller_employee", "seller_employee")
	if emp.StoreID != 9 {
		t.Errorf("employee store = %d, want the admin's store 9", emp.StoreID)
	}

	// Employees and buyers can not create accounts
	if err := register(t, s, "carol", "password123", "buyer"); err != nil {
		t.Fatal(err)
	}
	carol, _ := s.AccountRepo.GetAccountByUsernameRole(ctx, "carol", "buyer")
	if _, err := s.RegisterSellerRoles(ctx, &dto.RegisterSellerRolesInput{SellerAdminID: carol.ID, Username: "x", Password: "password123", Role: "seller_employee"}); err == nil {
		t.Error("buyer created an employee account")
	}
}
