package service

import (
	"context"
	"errors"
	"order-service/internal/repository"
	productpb "order-service/pkg/client/productclient"
	"order-service/pkg/dto"
	"order-service/pkg/model"
	"order-service/pkg/outbox"
	"testing"

	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var catalog = []*productpb.Product{
	{Id: 1, Name: "Phone", Price: 100.50, SellerId: 7, Inventory: 10},
	{Id: 2, Name: "Case", Price: 9.99, SellerId: 7, Inventory: 5},
	{Id: 3, Name: "Free", Price: 0, SellerId: 7, Inventory: 5},
}

func codeOf(err error) codes.Code { return status.Code(err) }

func TestToMinorRoundsToCents(t *testing.T) {
	cases := map[float64]int64{0: 0, 9.99: 999, 100.5: 10050, 0.1 + 0.2: 30, 19.999: 2000}
	for in, want := range cases {
		if got := toMinor(in); got != want {
			t.Errorf("toMinor(%v) = %d, want %d", in, got, want)
		}
	}
	if fromMinor(999) != 9.99 {
		t.Errorf("fromMinor(999) = %v", fromMinor(999))
	}
}

func TestPriceItems(t *testing.T) {
	svc, fake := newTestService(nil, catalog...)
	ctx := context.Background()

	t.Run("uses catalog prices and merges duplicate products", func(t *testing.T) {
		items := []*dto.OrderItem{
			{ProductID: 1, Quantity: 1, Price: 0.01}, // client price must be ignored
			{ProductID: 2, Quantity: 2},
			{ProductID: 1, Quantity: 2},
		}
		priced, total, err := svc.priceItems(ctx, items)
		if err != nil {
			t.Fatal(err)
		}
		if len(priced) != 2 {
			t.Fatalf("want 2 merged lines, got %d", len(priced))
		}
		if priced[0].productID != 1 || priced[0].quantity != 3 || priced[0].unitMinor != 10050 {
			t.Errorf("unexpected first line %+v", priced[0])
		}
		if want := int64(3*10050 + 2*999); total != want {
			t.Errorf("total = %d, want %d", total, want)
		}
	})

	invalid := map[string][]*dto.OrderItem{
		"no items":           nil,
		"zero quantity":      {{ProductID: 1, Quantity: 0}},
		"negative quantity":  {{ProductID: 1, Quantity: -1}},
		"huge quantity":      {{ProductID: 1, Quantity: maxItemQuantity + 1}},
		"missing product id": {{Quantity: 1}},
	}
	for name, items := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, _, err := svc.priceItems(ctx, items); codeOf(err) != codes.InvalidArgument {
				t.Errorf("want InvalidArgument, got %v", err)
			}
		})
	}

	t.Run("unknown product", func(t *testing.T) {
		_, _, err := svc.priceItems(ctx, []*dto.OrderItem{{ProductID: 99, Quantity: 1}})
		if codeOf(err) != codes.NotFound {
			t.Errorf("want NotFound, got %v", err)
		}
	})
	t.Run("not enough inventory", func(t *testing.T) {
		_, _, err := svc.priceItems(ctx, []*dto.OrderItem{{ProductID: 2, Quantity: 6}})
		if codeOf(err) != codes.FailedPrecondition {
			t.Errorf("want FailedPrecondition, got %v", err)
		}
	})
	t.Run("product without a price is not sellable", func(t *testing.T) {
		_, _, err := svc.priceItems(ctx, []*dto.OrderItem{{ProductID: 3, Quantity: 1}})
		if codeOf(err) != codes.FailedPrecondition {
			t.Errorf("want FailedPrecondition, got %v", err)
		}
	})
	t.Run("catalog outage", func(t *testing.T) {
		fake.err = errors.New("boom")
		defer func() { fake.err = nil }()
		_, _, err := svc.priceItems(ctx, []*dto.OrderItem{{ProductID: 1, Quantity: 1}})
		if codeOf(err) != codes.Unavailable {
			t.Errorf("want Unavailable, got %v", err)
		}
	})
}

func TestCreateOrderRequiresBuyer(t *testing.T) {
	svc, _ := newTestService(nil, catalog...)
	_, err := svc.CreateOrder(context.Background(), &dto.CreateOrderInput{Order: &dto.Order{
		OrderItems: []*dto.OrderItem{{ProductID: 1, Quantity: 1}},
	}})
	if codeOf(err) != codes.InvalidArgument {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func newPGService(t *testing.T) (*OrderService, *repository.OrderRepository) {
	t.Helper()
	repo := repository.NewOrderRepository(testDB(t))
	svc, _ := newTestService(repo, catalog...)
	return svc, repo
}

func createOrder(t *testing.T, svc *OrderService, buyer uint64, items ...*dto.OrderItem) uint64 {
	t.Helper()
	_, err := svc.CreateOrder(context.Background(), &dto.CreateOrderInput{Order: &dto.Order{
		BuyerID: buyer, TotalPrice: 0.01, OrderItems: items,
	}})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	var order model.Order
	if err := svc.OrderRepo.DB.Order("id DESC").First(&order).Error; err != nil {
		t.Fatal(err)
	}
	return order.ID
}

func TestCreateOrderPersistsServerSidePricesAndOutboxOrderID(t *testing.T) {
	svc, repo := newPGService(t)
	// Create a few orders so the order ID differs from the outbox row count
	createOrder(t, svc, 1, &dto.OrderItem{ProductID: 2, Quantity: 1})
	id := createOrder(t, svc, 5, &dto.OrderItem{ProductID: 1, Quantity: 2, Price: 0.01}, &dto.OrderItem{ProductID: 2, Quantity: 1})

	order, err := repo.GetOrderByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if order.BuyerID != 5 || order.Status != model.StatusPending {
		t.Errorf("unexpected order %+v", order)
	}
	if want := 2*100.50 + 9.99; order.TotalPrice != want {
		t.Errorf("total = %v, want %v", order.TotalPrice, want)
	}
	for _, item := range order.OrderItems {
		if item.ProductID == 1 && item.Price != 100.50 {
			t.Errorf("item price = %v, client price must be ignored", item.Price)
		}
	}

	events, err := repo.GetCreateOrderEventNotPublish(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.OrderID == id {
			found = true
		}
	}
	if !found || len(events) != 2 {
		t.Errorf("outbox events must reference real order IDs, got %+v (order %d)", events, id)
	}
}

func TestStatusEventIsIdempotentAndOnlyAppliesToPending(t *testing.T) {
	svc, repo := newPGService(t)
	ctx := context.Background()
	id := createOrder(t, svc, 1, &dto.OrderItem{ProductID: 1, Quantity: 1})

	send := func(success bool) {
		t.Helper()
		msg := &kafka.Message{Value: []byte(`{"order_id":` + itoa(id) + `,"success":` + boolStr(success) + `}`)}
		if err := svc.UpdateOrderStatusByKafka(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	status := func() string {
		o, err := repo.GetOrderByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return o.Status
	}

	send(true)
	if status() != model.StatusSuccess {
		t.Fatalf("want SUCCESS, got %s", status())
	}
	send(false) // late/duplicate contradicting event must not overwrite
	if status() != model.StatusSuccess {
		t.Fatalf("status was overwritten: %s", status())
	}
	send(true) // duplicate delivery is harmless
	if status() != model.StatusSuccess {
		t.Fatalf("want SUCCESS, got %s", status())
	}

	// Unknown order and malformed payloads are dropped without error
	if err := svc.UpdateOrderStatusByKafka(ctx, &kafka.Message{Value: []byte(`{"order_id":9999,"success":true}`)}); err != nil {
		t.Errorf("unknown order: %v", err)
	}
	if err := svc.UpdateOrderStatusByKafka(ctx, &kafka.Message{Value: []byte(`not json`)}); err != nil {
		t.Errorf("malformed payload: %v", err)
	}
}

func TestCancelOrderStateMachine(t *testing.T) {
	svc, repo := newPGService(t)
	ctx := context.Background()
	id := createOrder(t, svc, 1, &dto.OrderItem{ProductID: 1, Quantity: 2})

	// PENDING orders are still being processed and cannot be canceled
	if _, err := svc.CancelOrderByID(ctx, &dto.CancelOrderByIDInput{ID: id}); codeOf(err) != codes.FailedPrecondition {
		t.Fatalf("cancel pending: want FailedPrecondition, got %v", err)
	}
	if _, err := svc.CancelOrderByID(ctx, &dto.CancelOrderByIDInput{ID: 4242}); codeOf(err) != codes.NotFound {
		t.Fatalf("cancel unknown: want NotFound, got %v", err)
	}

	if _, err := repo.TransitionStatus(ctx, id, []string{model.StatusPending}, model.StatusSuccess); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelOrderByID(ctx, &dto.CancelOrderByIDInput{ID: id}); err != nil {
		t.Fatalf("cancel success order: %v", err)
	}
	order, _ := repo.GetOrderByID(ctx, id)
	if order.Status != model.StatusCanceled {
		t.Errorf("want CANCELED, got %s", order.Status)
	}

	var cancelEvents []outbox.CancelOrderEvent
	repo.DB.Find(&cancelEvents)
	if len(cancelEvents) != 1 || cancelEvents[0].OrderID != id || cancelEvents[0].Status != "PENDING" {
		t.Errorf("want one pending cancel event for order %d, got %+v", id, cancelEvents)
	}

	// Canceling twice must not emit a second inventory release
	if _, err := svc.CancelOrderByID(ctx, &dto.CancelOrderByIDInput{ID: id}); codeOf(err) != codes.FailedPrecondition {
		t.Errorf("second cancel: want FailedPrecondition, got %v", err)
	}
	repo.DB.Find(&cancelEvents)
	if len(cancelEvents) != 1 {
		t.Errorf("duplicate cancel event: %+v", cancelEvents)
	}
}

func TestUpdateOrderByIDOnlyChangesStatusAlongAllowedTransitions(t *testing.T) {
	svc, repo := newPGService(t)
	ctx := context.Background()
	id := createOrder(t, svc, 1, &dto.OrderItem{ProductID: 1, Quantity: 1})

	// Other fields in the payload are ignored
	_, err := svc.UpdateOrderByID(ctx, &dto.UpdateOrderByIDInput{Order: &dto.Order{ID: id, BuyerID: 99, TotalPrice: 1, Status: model.StatusSuccess}})
	if err != nil {
		t.Fatal(err)
	}
	order, _ := repo.GetOrderByID(ctx, id)
	if order.BuyerID != 1 || order.TotalPrice != 100.50 || order.Status != model.StatusSuccess {
		t.Errorf("unexpected order after update: %+v", order)
	}

	// SUCCESS -> FAILED is not allowed; cancel must use CancelOrderByID
	if _, err := svc.UpdateOrderByID(ctx, &dto.UpdateOrderByIDInput{Order: &dto.Order{ID: id, Status: model.StatusFailed}}); codeOf(err) != codes.FailedPrecondition {
		t.Errorf("want FailedPrecondition, got %v", err)
	}
	if _, err := svc.UpdateOrderByID(ctx, &dto.UpdateOrderByIDInput{Order: &dto.Order{ID: id, Status: model.StatusCanceled}}); codeOf(err) != codes.InvalidArgument {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestGetOrdersByBuyerIDStatusValidatesStatus(t *testing.T) {
	svc, _ := newPGService(t)
	_, err := svc.GetOrdersByBuyerIDStatus(context.Background(), &dto.GetOrdersByBuyerIDStatusInput{BuyerID: 1, Status: "BOGUS"})
	if codeOf(err) != codes.InvalidArgument {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestOrderItemsAreLabelledWithProductNames(t *testing.T) {
	svc, _ := newPGService(t)
	id := createOrder(t, svc, 1, &dto.OrderItem{ProductID: 2, Quantity: 1})
	out, err := svc.GetOrderByID(context.Background(), &dto.GetOrderByIDInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Order.OrderItems) != 1 || out.Order.OrderItems[0].Name != "Case" {
		t.Errorf("item name must come from the product catalog by product ID: %+v", out.Order.OrderItems)
	}
}

func itoa(n uint64) string { return string(appendUint(nil, n)) }

func appendUint(b []byte, n uint64) []byte {
	if n >= 10 {
		b = appendUint(b, n/10)
	}
	return append(b, byte('0'+n%10))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
