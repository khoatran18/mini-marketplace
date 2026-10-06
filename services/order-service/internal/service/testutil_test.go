package service

import (
	"context"
	"fmt"
	"math/rand"
	"order-service/internal/client/productclient"
	"order-service/internal/client/serviceclientmanager"
	"order-service/internal/repository"
	productpb "order-service/pkg/client/productclient"
	"order-service/pkg/model"
	"order-service/pkg/outbox"
	"os"
	"testing"

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

func newTestService(repo *repository.OrderRepository, catalog ...*productpb.Product) (*OrderService, *fakeProductClient) {
	fake := &fakeProductClient{products: map[uint64]*productpb.Product{}}
	for _, p := range catalog {
		fake.products[p.Id] = p
	}
	scm := &serviceclientmanager.ServiceClientManager{
		ProductServiceClient: productclient.NewProductClient(fake, nil, zap.NewNop()),
	}
	return &OrderService{OrderRepo: repo, ZapLogger: zap.NewNop(), SCM: scm}, fake
}

// testDB creates an isolated database on the server named by TEST_POSTGRES_DSN
// (e.g. "host=localhost port=5433 user=postgres dbname=postgres sslmode=disable").
// The test is skipped when the variable is not set.
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
	if err := db.AutoMigrate(&model.Order{}, &model.OrderItem{}, &outbox.CreateOrderEvent{}, &outbox.CancelOrderEvent{}); err != nil {
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
