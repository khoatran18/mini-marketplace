package service

import (
	"context"
	"order-service/internal/repository"
	"order-service/pkg/model"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCartLifecycle(t *testing.T) {
	db := testDB(t)
	inactive := product(2, 10, 1000, 5)
	inactive.Status = "hidden"
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 5), inactive, product(3, 20, 50000, 100))
	ctx := context.Background()

	cart, err := s.SetCartItem(ctx, 5, 1, 2)
	if err != nil || len(cart.Lines) != 1 || cart.ItemCount != 2 || cart.Subtotal != 200000 || cart.Lines[0].Name != "product-1" || !cart.Lines[0].Available {
		t.Fatalf("add: %v %+v", err, cart)
	}
	// replacing sets the quantity (it does not add)
	if cart, _ = s.SetCartItem(ctx, 5, 1, 4); cart.ItemCount != 4 {
		t.Errorf("replace quantity: %d", cart.ItemCount)
	}
	// stock and visibility are checked when adding
	if _, err := s.SetCartItem(ctx, 5, 1, 6); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("more than the stock: %v", err)
	}
	if _, err := s.SetCartItem(ctx, 5, 2, 1); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("hidden product: %v", err)
	}
	if _, err := s.SetCartItem(ctx, 5, 99, 1); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("unknown product: %v", err)
	}
	if _, err := s.SetCartItem(ctx, 5, 3, 100); status.Code(err) != codes.InvalidArgument {
		t.Errorf("quantity above 99: %v", err)
	}
	if _, err := s.SetCartItem(ctx, 0, 1, 1); status.Code(err) != codes.InvalidArgument {
		t.Errorf("no user: %v", err)
	}
	// carts are private
	if other, _ := s.GetCart(ctx, 6); len(other.Lines) != 0 {
		t.Error("buyer 6 sees buyer 5's cart")
	}
	// quantity 0 removes
	s.SetCartItem(ctx, 5, 3, 1)
	if cart, _ = s.SetCartItem(ctx, 5, 1, 0); len(cart.Lines) != 1 || cart.Lines[0].ProductID != 3 {
		t.Errorf("remove: %+v", cart.Lines)
	}
	if cart, _ = s.ClearCart(ctx, 5); len(cart.Lines) != 0 || cart.Subtotal != 0 {
		t.Errorf("clear: %+v", cart)
	}
}

func TestCartShowsProblemsThatAppearAfterAdding(t *testing.T) {
	db := testDB(t)
	s, fake := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 5))
	ctx := context.Background()
	s.SetCartItem(ctx, 5, 1, 3)
	fake.products[1].Inventory = 1 // stock sold in the meantime
	cart, _ := s.GetCart(ctx, 5)
	if cart.Lines[0].Available || cart.Lines[0].Issue != "only 1 left" || cart.Subtotal != 0 {
		t.Errorf("stale line must be flagged and not priced: %+v subtotal=%v", cart.Lines[0], cart.Subtotal)
	}
	fake.products[1].Status = "banned"
	if cart, _ = s.GetCart(ctx, 5); cart.Lines[0].Issue != "product is not available" {
		t.Errorf("banned product: %q", cart.Lines[0].Issue)
	}
	delete(fake.products, 1)
	if cart, _ = s.GetCart(ctx, 5); cart.Lines[0].Issue != "product not found" || cart.Lines[0].Name != "" {
		t.Errorf("deleted product: %+v", cart.Lines[0])
	}
}

func TestMergeGuestCart(t *testing.T) {
	db := testDB(t)
	inactive := product(2, 10, 1000, 5)
	inactive.Status = "draft"
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 500), inactive, product(3, 20, 50000, 500))
	ctx := context.Background()
	s.SetCartItem(ctx, 5, 1, 90)

	cart, err := s.MergeCart(ctx, 5, []Line{{1, 20}, {2, 1}, {3, 2}, {99, 1}, {3, 1}, {1, -4}})
	if err != nil {
		t.Fatal(err)
	}
	qty := map[uint64]int64{}
	for _, l := range cart.Lines {
		qty[l.ProductID] = l.Quantity
	}
	if len(qty) != 2 || qty[1] != repository.MaxCartQuantity || qty[3] != 3 {
		t.Errorf("merge result: %v (quantities add up, cap at 99, unavailable/unknown/negative lines are skipped)", qty)
	}
	// merging again keeps the cap
	if cart, _ = s.MergeCart(ctx, 5, []Line{{3, 500}}); cart.ItemCount != int64(repository.MaxCartQuantity*2) {
		t.Errorf("cap: %d", cart.ItemCount)
	}
}

func TestCartIsLimitedToFiftyProducts(t *testing.T) {
	db := testDB(t)
	var catalog = make([]*struct{}, 0)
	_ = catalog
	s, fake := newTestService(repository.NewOrderRepository(db))
	for i := uint64(1); i <= 52; i++ {
		fake.products[i] = product(i, 10, 1000, 10)
	}
	ctx := context.Background()
	for i := uint64(1); i <= model.MaxCartLines; i++ {
		if _, err := s.SetCartItem(ctx, 5, i, 1); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
	}
	if _, err := s.SetCartItem(ctx, 5, 51, 1); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("51st product: %v", err)
	}
	// changing an existing line is still fine
	if _, err := s.SetCartItem(ctx, 5, 1, 2); err != nil {
		t.Errorf("update existing line in a full cart: %v", err)
	}
	// merging into a full cart drops the extra lines instead of failing
	if cart, err := s.MergeCart(ctx, 5, []Line{{52, 1}}); err != nil || len(cart.Lines) != model.MaxCartLines {
		t.Errorf("merge into a full cart: %v", err)
	}
}

func TestCheckoutFromCartConsumesOnlyOrderedLines(t *testing.T) {
	db := testDB(t)
	s, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50), product(3, 20, 50000, 50))
	ctx := context.Background()
	s.SetCartItem(ctx, 5, 1, 2)
	s.SetCartItem(ctx, 5, 3, 1)

	res, err := s.Checkout(ctx, &CheckoutInput{BuyerID: 5, FromCart: true, PaymentMethod: model.MethodCOD, Address: testAddress, IdempotencyKey: "cart-1"})
	if err != nil || len(res.Orders) != 2 {
		t.Fatalf("checkout from cart: %v", err)
	}
	if cart, _ := s.GetCart(ctx, 5); len(cart.Lines) != 0 {
		t.Errorf("ordered lines must leave the cart: %+v", cart.Lines)
	}
	// a replay of the same key must not touch the (new) cart contents
	s.SetCartItem(ctx, 5, 1, 1)
	again, _ := s.Checkout(ctx, &CheckoutInput{BuyerID: 5, FromCart: true, PaymentMethod: model.MethodCOD, Address: testAddress, IdempotencyKey: "cart-1"})
	if !again.Replayed {
		t.Fatal("expected a replay")
	}
	if cart, _ := s.GetCart(ctx, 5); len(cart.Lines) != 1 {
		t.Errorf("a replayed checkout must not clear the cart: %+v", cart.Lines)
	}
	// a failing checkout keeps the cart
	s.SetCartItem(ctx, 5, 3, 1)
	failing, _ := newTestService(repository.NewOrderRepository(db), product(1, 10, 100000, 50))
	if _, err := failing.Checkout(ctx, &CheckoutInput{BuyerID: 5, FromCart: true, PaymentMethod: model.MethodCOD, Address: testAddress, IdempotencyKey: "cart-2"}); err == nil {
		t.Fatal("product 3 is unknown to this catalog")
	}
	if cart, _ := s.GetCart(ctx, 5); len(cart.Lines) != 2 {
		t.Errorf("a failed checkout must keep the cart: %+v", cart.Lines)
	}
}
