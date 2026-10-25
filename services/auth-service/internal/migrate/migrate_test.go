package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"auth-service/pkg/model"
	"auth-service/pkg/outbox"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newDB tạo DB tạm trên TEST_POSTGRES_DSN (bỏ qua nếu chưa đặt).
func newDB(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("auth_mig_%d", rand.Int63())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		sqlDB.Close()
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		if a, err := admin.DB(); err == nil {
			a.Close()
		}
	})
	return db, sqlDB
}

func tableExists(t *testing.T, db *gorm.DB, name string) bool {
	var ok bool
	if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", name).Scan(&ok).Error; err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestRunEmptyDBAndIdempotent(t *testing.T) {
	db, sqlDB := newDB(t)
	for i := 0; i < 2; i++ {
		if err := Run(context.Background(), sqlDB); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	for _, tb := range []string{"accounts", "pwd_version_events", TableName} {
		if !tableExists(t, db, tb) {
			t.Fatalf("missing table %s", tb)
		}
	}
	if err := db.Create(&model.Account{Username: "u", Password: "p", Role: "buyer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Account{Username: "u", Password: "p", Role: "buyer"}).Error; err == nil {
		t.Fatal("expected unique violation on (username, role)")
	}
	// R5: same username under another role is allowed.
	if err := db.Create(&model.Account{Username: "u", Password: "p", Role: "seller"}).Error; err != nil {
		t.Fatalf("same username other role should be allowed: %v", err)
	}
}

func TestRunLegacyDBKeepsData(t *testing.T) {
	db, sqlDB := newDB(t)
	// DB cũ do GORM AutoMigrate tạo, đã có dữ liệu.
	if err := db.AutoMigrate(&model.Account{}, &outbox.PwdVersionEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Account{Username: "old", Password: "p", Role: "buyer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), sqlDB); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.Account{}).Where("username = ?", "old").Count(&n)
	if n != 1 {
		t.Fatalf("legacy row lost, n=%d", n)
	}
}
