package repository

import (
	"errors"
	"order-service/pkg/model"
	"testing"
)

func TestStateMachineTable(t *testing.T) {
	type edge struct{ from, to, actor string }
	allowed := map[edge]bool{
		{"PENDING", "AWAITING_PAYMENT", "system"}: true, {"PENDING", "CONFIRMED", "system"}: true, {"PENDING", "FAILED", "system"}: true,
		{"AWAITING_PAYMENT", "PAID", "payment"}: true, {"AWAITING_PAYMENT", "EXPIRED", "system"}: true,
		{"AWAITING_PAYMENT", "CANCELED", "buyer"}: true, {"AWAITING_PAYMENT", "CANCELED", "seller"}: true, {"AWAITING_PAYMENT", "CANCELED", "admin"}: true,
		{"CONFIRMED", "SHIPPED", "seller"}: true,
		{"CONFIRMED", "CANCELED", "buyer"}: true, {"CONFIRMED", "CANCELED", "seller"}: true, {"CONFIRMED", "CANCELED", "admin"}: true,
		{"PAID", "SHIPPED", "seller"}: true,
		{"PAID", "CANCELED", "buyer"}: true, {"PAID", "CANCELED", "seller"}: true, {"PAID", "CANCELED", "admin"}: true,
		{"SHIPPED", "DELIVERED", "seller"}: true, {"SHIPPED", "DELIVERED", "buyer"}: true, {"SHIPPED", "DELIVERED", "admin"}: true, {"SHIPPED", "DELIVERED", "system"}: true,
		{"DELIVERED", "REFUND_REQUESTED", "buyer"}: true,
		{"REFUND_REQUESTED", "REFUNDED", "seller"}: true, {"REFUND_REQUESTED", "REFUNDED", "admin"}: true,
		{"REFUND_REQUESTED", "DELIVERED", "seller"}: true, {"REFUND_REQUESTED", "DELIVERED", "admin"}: true,
	}
	actors := []string{ActorSystem, ActorBuyer, ActorSeller, ActorAdmin, ActorPayment, "hacker", ""}
	for _, from := range append(model.AllStatuses, "SUCCESS", "") {
		for _, to := range model.AllStatuses {
			for _, actor := range actors {
				want := allowed[edge{from, to, actor}]
				if got := CanTransition(from, to, actor); got != want {
					t.Errorf("CanTransition(%s -> %s by %q) = %v, want %v", from, to, actor, got, want)
				}
			}
		}
	}
}

func TestTerminalStatusesHaveNoWayOut(t *testing.T) {
	for _, from := range []string{model.StatusFailed, model.StatusExpired, model.StatusCanceled, model.StatusRefunded} {
		if len(transitions[from]) != 0 {
			t.Errorf("%s must be terminal", from)
		}
	}
}

func TestInvalidTransitionErrorMatchesSentinel(t *testing.T) {
	err := error(&InvalidTransitionError{From: "PAID", To: "PENDING", Actor: "buyer"})
	if !errors.Is(err, ErrNotAllowed) {
		t.Error("errors.Is(err, ErrNotAllowed) must hold")
	}
	if err.Error() == "" {
		t.Error("error text")
	}
}

func TestMinorUnits(t *testing.T) {
	for in, want := range map[float64]int64{0: 0, 19.99: 1999, 380000: 38000000, 0.005: 1, -3.5: -350} {
		if got := minor(in); got != want {
			t.Errorf("minor(%v) = %d, want %d", in, got, want)
		}
	}
}
