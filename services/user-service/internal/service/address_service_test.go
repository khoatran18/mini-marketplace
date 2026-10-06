package service

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"user-service/internal/repository"
	"user-service/pkg/model"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testService connects to an isolated database on TEST_POSTGRES_DSN; skipped when unset.
func testService(t *testing.T) *UserService {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	name := fmt.Sprintf("user_test_%d", rand.Int63())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create db: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Address{}); err != nil {
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
	return &UserService{UserRepo: repository.NewUserRepository(db), ZapLogger: zap.NewNop()}
}

func addr(user uint64, label string) *model.Address {
	return &model.Address{UserID: user, Label: label, ReceiverName: "Nguyễn Văn A", Phone: "0901 234 567", Line1: "12 Lê Lợi", District: "Q1", City: "TP.HCM"}
}

func defaults(t *testing.T, s *UserService, user uint64) []string {
	t.Helper()
	list, err := s.ListAddresses(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range list {
		if a.IsDefault {
			out = append(out, a.Label)
		}
	}
	return out
}

func TestFirstAddressIsDefaultAndOnlyOneDefaultExists(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	home, err := s.UpsertAddress(ctx, addr(1, "home"))
	if err != nil || !home.IsDefault {
		t.Fatalf("first address must become the default: %v %+v", err, home)
	}
	office, _ := s.UpsertAddress(ctx, addr(1, "office"))
	if office.IsDefault {
		t.Error("a later address must not steal the default")
	}
	// choosing a new default clears the old one
	office.IsDefault = true
	if _, err := s.UpsertAddress(ctx, office); err != nil {
		t.Fatal(err)
	}
	if got := defaults(t, s, 1); len(got) != 1 || got[0] != "office" {
		t.Errorf("defaults = %v", got)
	}
	// un-defaulting the default is ignored: there is always one
	office.IsDefault = false
	if _, err := s.UpsertAddress(ctx, office); err != nil {
		t.Fatal(err)
	}
	if got := defaults(t, s, 1); len(got) != 1 || got[0] != "office" {
		t.Errorf("defaults after unset = %v", got)
	}
	// creating a new address as default moves the default too
	third := addr(1, "parents")
	third.IsDefault = true
	if _, err := s.UpsertAddress(ctx, third); err != nil {
		t.Fatal(err)
	}
	if got := defaults(t, s, 1); len(got) != 1 || got[0] != "parents" {
		t.Errorf("defaults after create = %v", got)
	}
	// deleting the default promotes the oldest remaining one
	if err := s.DeleteAddress(ctx, 1, third.ID); err != nil {
		t.Fatal(err)
	}
	if got := defaults(t, s, 1); len(got) != 1 || got[0] != "home" {
		t.Errorf("defaults after delete = %v", got)
	}
	// id 0 means the default address
	if a, err := s.GetAddress(ctx, 1, 0); err != nil || a.Label != "home" {
		t.Errorf("default lookup: %v %+v", err, a)
	}
}

func TestAddressesAreIsolatedPerUser(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	mine, _ := s.UpsertAddress(ctx, addr(1, "mine"))
	if _, err := s.GetAddress(ctx, 2, mine.ID); status.Code(err) != codes.NotFound {
		t.Errorf("reading another user's address: %v", err)
	}
	if err := s.DeleteAddress(ctx, 2, mine.ID); status.Code(err) != codes.NotFound {
		t.Errorf("deleting another user's address: %v", err)
	}
	hijack := addr(2, "hijack")
	hijack.ID = mine.ID
	if _, err := s.UpsertAddress(ctx, hijack); status.Code(err) != codes.NotFound {
		t.Errorf("updating another user's address: %v", err)
	}
	if got, _ := s.GetAddress(ctx, 1, mine.ID); got.Label != "mine" {
		t.Errorf("address was modified: %+v", got)
	}
	if list, _ := s.ListAddresses(ctx, 2); len(list) != 0 {
		t.Errorf("user 2 sees %d addresses", len(list))
	}
}

func TestAddressLimitAndValidation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	for i := 0; i < model.MaxAddressesPerUser; i++ {
		if _, err := s.UpsertAddress(ctx, addr(1, fmt.Sprint("a", i))); err != nil {
			t.Fatalf("address %d: %v", i, err)
		}
	}
	if _, err := s.UpsertAddress(ctx, addr(1, "one too many")); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("limit: %v", err)
	}
	if _, err := s.UpsertAddress(ctx, addr(2, "other user is unaffected")); err != nil {
		t.Errorf("limit is per user: %v", err)
	}
	bad := map[string]func(a *model.Address){
		"no receiver": func(a *model.Address) { a.ReceiverName = " " },
		"bad phone":   func(a *model.Address) { a.Phone = "abc" },
		"short phone": func(a *model.Address) { a.Phone = "123" },
		"no line":     func(a *model.Address) { a.Line1 = "" },
		"no city":     func(a *model.Address) { a.City = "" },
		"long label":  func(a *model.Address) { a.Label = string(make([]rune, 51)) + "x" },
		"no user":     func(a *model.Address) { a.UserID = 0 },
	}
	for name, mutate := range bad {
		a := addr(3, "x")
		mutate(a)
		if _, err := s.UpsertAddress(ctx, a); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, err)
		}
	}
	for _, phone := range []string{"0901234567", "+84 901 234 567", "090-123-4567"} {
		a := addr(4, "ok")
		a.Phone = phone
		if _, err := s.UpsertAddress(ctx, a); err != nil {
			t.Errorf("phone %q should be accepted: %v", phone, err)
		}
	}
}
