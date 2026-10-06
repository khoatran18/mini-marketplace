package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"order-service/internal/client/productclient"
	"order-service/internal/client/serviceclientmanager"
	"order-service/internal/repository"
	productpb "order-service/pkg/client/productclient"
	"order-service/pkg/events"
	"order-service/pkg/model"
	"os"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeProductClient serves a fixed catalog; the embedded interface panics on unused methods.
type fakeProductClient struct {
	productpb.ProductServiceClient
	products map[uint64]*productpb.Product
	err      error
}

func (f *fakeProductClient) GetProductsByID(_ context.Context, in *productpb.GetProductsByIDRequest, _ ...grpc.CallOption) (*productpb.GetProductsByIDResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	res := &productpb.GetProductsByIDResponse{}
	for _, id := range in.GetId() {
		if p, ok := f.products[id]; ok {
			res.Product = append(res.Product, p)
		}
	}
	return res, nil
}

// product builds an active catalog entry: id, store, price, available stock.
func product(id, store uint64, price float64, stock int64) *productpb.Product {
	return &productpb.Product{
		Id: id, Name: fmt.Sprintf("product-%d", id), Price: price, SellerId: store, Inventory: stock, Status: "active",
		Sku: fmt.Sprintf("SKU-%d", id), CategoryId: 3, ImageUrls: []string{"https://img.example/" + fmt.Sprint(id) + ".jpg"}, StockLevel: "ok",
	}
}

func testSettings() Settings {
	return Settings{PaymentTTL: 15 * time.Minute, ReturnWindow: 7 * 24 * time.Hour, AutoDeliverAfter: 7 * 24 * time.Hour, ShippingFlatMinor: 30000 * 100, FreeShipOverMinor: 500000 * 100}
}

func newTestService(repo *repository.OrderRepository, catalog ...*productpb.Product) (*OrderService, *fakeProductClient) {
	fake := &fakeProductClient{products: map[uint64]*productpb.Product{}}
	for _, p := range catalog {
		fake.products[p.Id] = p
	}
	scm := &serviceclientmanager.ServiceClientManager{
		ProductServiceClient: productclient.NewProductClient(fake, nil, zap.NewNop()),
	}
	return &OrderService{OrderRepo: repo, ZapLogger: zap.NewNop(), SCM: scm, Settings: testSettings()}, fake
}

// testDB creates an isolated database on the server named by TEST_POSTGRES_DSN; skipped when unset.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	name := fmt.Sprintf("order_test_%d", rand.Int63())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create db: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := repository.Migrate(db); err != nil {
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
	return db
}

// ---- helpers shared by the tests ------------------------------------------------------------

var testAddress = map[string]any{"receiver_name": "Nguyễn Văn A", "phone": "0901234567", "line1": "12 Lê Lợi", "district": "Q1", "city": "TP.HCM"}

func checkout(t *testing.T, s *OrderService, buyer uint64, method, key string, items ...Line) *CheckoutResult {
	t.Helper()
	res, err := s.Checkout(context.Background(), &CheckoutInput{BuyerID: buyer, Items: items, PaymentMethod: method, Address: testAddress, IdempotencyKey: key})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	return res
}

func validated(orderID uint64, success bool) *kafka.Message {
	b, _ := json.Marshal(map[string]any{"order_id": orderID, "success": success})
	return &kafka.Message{Value: b}
}

func paymentSucceeded(checkoutID string, amountMinor int64) *kafka.Message {
	b, _ := json.Marshal(map[string]any{"checkout_id": checkoutID, "payment_id": 1, "amount_minor": amountMinor})
	return &kafka.Message{Value: b}
}

func reload(t *testing.T, s *OrderService, id uint64) *model.Order {
	t.Helper()
	o, _, err := s.OrderRepo.GetOrder(context.Background(), id, repository.Scope{Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// emitted returns the payloads of the outbox rows of a topic, oldest first.
func emitted(t *testing.T, db *gorm.DB, topic string) []map[string]any {
	t.Helper()
	var rows []events.Event
	if err := db.Where("topic = ?", topic).Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var m map[string]any
		_ = json.Unmarshal(r.Payload, &m)
		out = append(out, m)
	}
	return out
}

func statusOf(t *testing.T, s *OrderService, id uint64) string { return reload(t, s, id).Status }

func msg(s string) *kafka.Message { return &kafka.Message{Value: []byte(s)} }
