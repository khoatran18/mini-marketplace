package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"product-service/internal/repository"
	"product-service/pkg/dto"
	"product-service/pkg/model"
	"product-service/pkg/outbox"
	"sync"
	"testing"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testService connects to an isolated database on TEST_POSTGRES_DSN
// (e.g. "host=localhost port=5433 user=postgres sslmode=disable"); skipped when unset.
func testService(t *testing.T) *ProductService {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	name := fmt.Sprintf("product_test_%d", rand.Int63())
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
	return &ProductService{ProductRepo: repository.NewProductRepository(db), ZapLogger: zap.NewNop()}
}

func seed(t *testing.T, s *ProductService, sellerID uint64, price float64, inventory int64) uint64 {
	t.Helper()
	p := &model.Product{Name: "p", Price: price, SellerID: sellerID, Inventory: inventory, Attributes: []byte(`{}`)}
	if err := s.ProductRepo.CreateProduct(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func inventoryOf(t *testing.T, s *ProductService, id uint64) int64 {
	t.Helper()
	inv, err := s.ProductRepo.GetInventoryByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func createOrderMsg(orderID uint64, items ...dto.ItemEvent) *kafka.Message {
	b, _ := jsonMarshal(dto.CreateOrderKafkaEvent{OrderID: orderID, Items: toPtrs(items)})
	return &kafka.Message{Value: b}
}

func cancelOrderMsg(orderID uint64, items ...dto.ItemEvent) *kafka.Message {
	b, _ := jsonMarshal(dto.CancelOrderKafkaEvent{OrderID: orderID, Items: toPtrs(items)})
	return &kafka.Message{Value: b}
}

func toPtrs(items []dto.ItemEvent) []*dto.ItemEvent {
	out := make([]*dto.ItemEvent, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}

func validationOf(t *testing.T, s *ProductService, orderID uint64) *outbox.ValidateOrderEvent {
	t.Helper()
	var e outbox.ValidateOrderEvent
	if err := s.ProductRepo.DB.Where("order_id = ?", orderID).First(&e).Error; err != nil {
		t.Fatalf("no validation result for order %d: %v", orderID, err)
	}
	return &e
}

func TestReserveInventorySuccess(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a, b := seed(t, s, 1, 10, 5), seed(t, s, 1, 10, 3)

	if err := s.ValidateProductInventory(ctx, createOrderMsg(1, dto.ItemEvent{ProductID: a, Quantity: 2}, dto.ItemEvent{ProductID: b, Quantity: 3})); err != nil {
		t.Fatal(err)
	}
	if inventoryOf(t, s, a) != 3 || inventoryOf(t, s, b) != 0 {
		t.Errorf("inventory not reserved: a=%d b=%d", inventoryOf(t, s, a), inventoryOf(t, s, b))
	}
	if e := validationOf(t, s, 1); !e.Success || !e.Processed || e.Status != "PENDING" {
		t.Errorf("unexpected result %+v", e)
	}
}

func TestReserveInventoryIsAllOrNothing(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a, b := seed(t, s, 1, 10, 5), seed(t, s, 1, 10, 1)

	// Second line lacks stock: the first line must not stay decremented
	if err := s.ValidateProductInventory(ctx, createOrderMsg(2, dto.ItemEvent{ProductID: a, Quantity: 2}, dto.ItemEvent{ProductID: b, Quantity: 2})); err != nil {
		t.Fatal(err)
	}
	if inventoryOf(t, s, a) != 5 || inventoryOf(t, s, b) != 1 {
		t.Errorf("inventory changed by a failed reservation: a=%d b=%d", inventoryOf(t, s, a), inventoryOf(t, s, b))
	}
	if e := validationOf(t, s, 2); e.Success {
		t.Errorf("want failed validation, got %+v", e)
	}
}

func TestValidateIsIdempotentPerOrder(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a := seed(t, s, 1, 10, 10)
	msg := createOrderMsg(3, dto.ItemEvent{ProductID: a, Quantity: 4})

	for i := 0; i < 3; i++ {
		if err := s.ValidateProductInventory(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	if got := inventoryOf(t, s, a); got != 6 {
		t.Errorf("redelivery decremented stock again: %d", got)
	}
}

func TestConcurrentDuplicateDeliveryReservesOnce(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a := seed(t, s, 1, 10, 100)
	msg := createOrderMsg(4, dto.ItemEvent{ProductID: a, Quantity: 7})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.ValidateProductInventory(ctx, msg) // losers fail on the result row's primary key
		}()
	}
	wg.Wait()
	if got := inventoryOf(t, s, a); got != 93 {
		t.Errorf("inventory = %d, want 93 (reserved exactly once)", got)
	}
}

func TestNoOverselling(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a := seed(t, s, 1, 10, 10)

	var wg sync.WaitGroup
	for order := uint64(100); order < 120; order++ {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			_ = s.ValidateProductInventory(ctx, createOrderMsg(id, dto.ItemEvent{ProductID: a, Quantity: 1}))
		}(order)
	}
	wg.Wait()

	if got := inventoryOf(t, s, a); got != 0 {
		t.Errorf("inventory = %d, want 0", got)
	}
	var ok, failed int64
	s.ProductRepo.DB.Model(&outbox.ValidateOrderEvent{}).Where("success = ?", true).Count(&ok)
	s.ProductRepo.DB.Model(&outbox.ValidateOrderEvent{}).Where("success = ?", false).Count(&failed)
	if ok != 10 || failed != 10 {
		t.Errorf("want 10 successes and 10 failures, got %d/%d", ok, failed)
	}
}

func TestMalformedMessagesAreDropped(t *testing.T) {
	s := testService(t)
	if err := s.ValidateProductInventory(context.Background(), &kafka.Message{Value: []byte("nope")}); err != nil {
		t.Errorf("malformed create event must not be retried: %v", err)
	}
	if err := s.ReleaseProductInventory(context.Background(), &kafka.Message{Value: []byte("nope")}); err != nil {
		t.Errorf("malformed cancel event must not be retried: %v", err)
	}
}

func TestReleaseInventoryExactlyOnce(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a := seed(t, s, 1, 10, 10)
	item := dto.ItemEvent{ProductID: a, Quantity: 4}

	if err := s.ValidateProductInventory(ctx, createOrderMsg(10, item)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ { // duplicate cancel events
		if err := s.ReleaseProductInventory(ctx, cancelOrderMsg(10, item)); err != nil {
			t.Fatal(err)
		}
	}
	if got := inventoryOf(t, s, a); got != 10 {
		t.Errorf("inventory = %d, want 10 (released once)", got)
	}

	// Orders that failed validation or never reserved anything release nothing
	_ = s.ValidateProductInventory(ctx, createOrderMsg(11, dto.ItemEvent{ProductID: a, Quantity: 999}))
	if err := s.ReleaseProductInventory(ctx, cancelOrderMsg(11, dto.ItemEvent{ProductID: a, Quantity: 999})); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseProductInventory(ctx, cancelOrderMsg(12345, item)); err != nil {
		t.Fatal(err)
	}
	if got := inventoryOf(t, s, a); got != 10 {
		t.Errorf("inventory = %d, want 10", got)
	}
}

func TestUpdateProductAuthorizationAndValidation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	id := seed(t, s, 7, 50, 5)

	update := func(callerStore uint64, p dto.Product) error {
		p.ID = id
		_, err := s.UpdateProduct(ctx, &dto.UpdateProductInput{Product: &p, UserID: callerStore})
		return err
	}

	// Another store (or no store) can not edit the product
	if code := status.Code(update(8, dto.Product{Name: "x", Price: 1})); code != codes.PermissionDenied {
		t.Errorf("foreign store: want PermissionDenied, got %v", code)
	}
	if code := status.Code(update(0, dto.Product{Name: "x", Price: 1})); code != codes.PermissionDenied {
		t.Errorf("no store: want PermissionDenied, got %v", code)
	}
	// Invalid fields
	for name, p := range map[string]dto.Product{
		"empty name":         {Name: " ", Price: 1},
		"zero price":         {Name: "x", Price: 0},
		"negative inventory": {Name: "x", Price: 1, Inventory: -1},
	} {
		if code := status.Code(update(7, p)); code != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, code)
		}
	}
	// Unknown product
	_, err := s.UpdateProduct(ctx, &dto.UpdateProductInput{Product: &dto.Product{ID: 9999, Name: "x", Price: 1}, UserID: 7})
	if status.Code(err) != codes.NotFound {
		t.Errorf("unknown product: want NotFound, got %v", err)
	}

	// Owner can update, set inventory to zero, and can not change the owner
	if err := update(7, dto.Product{Name: "renamed", Price: 12.5, Inventory: 0, SellerID: 999, Attributes: []byte(`{"a":1}`)}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ProductRepo.GetProductByID(ctx, id)
	if got.Name != "renamed" || got.Price != 12.5 || got.Inventory != 0 || got.SellerID != 7 {
		t.Errorf("unexpected product after update: %+v", got)
	}
}

func TestCreateProductValidation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	for name, in := range map[string]dto.CreateProductInput{
		"no seller": {Name: "x", Price: 1, Inventory: 1},
		"no name":   {SellerID: 1, Price: 1},
		"bad price": {SellerID: 1, Name: "x", Price: -1},
		"bad stock": {SellerID: 1, Name: "x", Price: 1, Inventory: -5},
	} {
		in := in
		if _, err := s.CreateProduct(ctx, &in); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, err)
		}
	}
}

func TestGetProductsPaginationIsBounded(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		seed(t, s, 1, 1, 1)
	}
	// page 0 used to underflow into a huge offset
	got, err := s.ProductRepo.GetProducts(ctx, 0, 2, false)
	if err != nil || len(got) != 2 {
		t.Fatalf("page 0: %v len=%d", err, len(got))
	}
	got, err = s.ProductRepo.GetProducts(ctx, 2, 2, false)
	if err != nil || len(got) != 1 {
		t.Fatalf("page 2: %v len=%d", err, len(got))
	}
	got, err = s.ProductRepo.GetProducts(ctx, 1, 1_000_000, false)
	if err != nil || len(got) != 3 {
		t.Fatalf("huge page size: %v len=%d", err, len(got))
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
