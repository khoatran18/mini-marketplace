package migrate_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"order-service/internal/migrate"
	"order-service/pkg/events"
	"order-service/pkg/model"
	"order-service/pkg/outbox"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("order_mig_%d", rand.Int63())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			s.Close()
		}
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		if s, err := admin.DB(); err == nil {
			s.Close()
		}
	})
	return db
}

func run(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Run(context.Background(), sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func count(t *testing.T, db *gorm.DB, q string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(q).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunOnEmptyDBAndTwice(t *testing.T) {
	db := newDB(t)
	run(t, db)
	run(t, db) // lần hai: không làm gì
	for _, tb := range []string{"orders", "order_items", "order_status_histories", "checkouts", "cart_items", "create_order_events", "cancel_order_events", "domain_events", migrate.VersionTable} {
		if !db.Migrator().HasTable(tb) {
			t.Errorf("missing table %s", tb)
		}
	}
	// model khớp schema: ghi/đọc được
	o := model.Order{BuyerID: 1, TotalPrice: 10}
	if err := db.Create(&o).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.OrderItem{OrderID: o.ID, ProductID: 1, Quantity: 1, Price: 10}).Error; err != nil {
		t.Fatal(err)
	}
	if err := events.Emit(db, "t", "k", map[string]int{"v": 1}); err != nil {
		t.Fatal(err)
	}
}

func TestRunOnLegacyAutoMigratedDB(t *testing.T) {
	db := newDB(t)
	// DB cũ do GORM AutoMigrate tạo, đã có dữ liệu (kể cả trạng thái legacy SUCCESS)
	if err := db.AutoMigrate(&model.Order{}, &model.OrderItem{}, &model.OrderStatusHistory{}, &model.Checkout{}, &model.CartItem{},
		&outbox.CreateOrderEvent{}, &outbox.CancelOrderEvent{}, &events.Event{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Order{BuyerID: 7, Status: model.LegacyStatusSuccess, TotalPrice: 5}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Order{BuyerID: 7, Status: model.StatusPaid, TotalPrice: 6}).Error; err != nil {
		t.Fatal(err)
	}
	run(t, db)
	run(t, db)
	if n := count(t, db, "SELECT count(*) FROM orders"); n != 2 {
		t.Errorf("orders = %d, want 2 (data kept)", n)
	}
	if n := count(t, db, "SELECT count(*) FROM orders WHERE status='CONFIRMED' AND buyer_id=7"); n != 1 {
		t.Errorf("legacy SUCCESS order not migrated to CONFIRMED")
	}
}
