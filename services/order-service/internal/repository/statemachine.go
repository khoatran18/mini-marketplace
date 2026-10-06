package repository

import (
	"errors"
	"fmt"
	"order-service/pkg/model"
	"slices"
)

// Actor types.
const (
	ActorSystem  = "system"
	ActorBuyer   = "buyer"
	ActorSeller  = "seller"
	ActorAdmin   = "admin"
	ActorPayment = "payment"
)

// Actor is who causes a status change. Buyers are identified by ID (user id), sellers by StoreID.
type Actor struct {
	Type    string
	ID      uint64
	StoreID uint64
}

// transitions lists, for every status, the statuses it can move to and which actors may do it.
//
//	PENDING ─▶ AWAITING_PAYMENT (online) | CONFIRMED (COD) | FAILED          system
//	AWAITING_PAYMENT ─▶ PAID (payment) | EXPIRED (system) | CANCELED (buyer, seller, admin)
//	CONFIRMED/PAID ─▶ SHIPPED (seller) | CANCELED (buyer, seller, admin)
//	SHIPPED ─▶ DELIVERED (seller, buyer, admin, system after AUTO_DELIVER_DAYS)
//	DELIVERED ─▶ REFUND_REQUESTED (buyer)
//	REFUND_REQUESTED ─▶ REFUNDED | DELIVERED (return rejected)          seller, admin
//
// FAILED, EXPIRED, CANCELED and REFUNDED are terminal.
var transitions = map[string]map[string][]string{
	model.StatusPending: {
		model.StatusAwaitingPayment: {ActorSystem},
		model.StatusConfirmed:       {ActorSystem},
		model.StatusFailed:          {ActorSystem},
	},
	model.StatusAwaitingPayment: {
		model.StatusPaid:     {ActorPayment},
		model.StatusExpired:  {ActorSystem},
		model.StatusCanceled: {ActorBuyer, ActorSeller, ActorAdmin},
	},
	model.StatusConfirmed: {
		model.StatusShipped:  {ActorSeller},
		model.StatusCanceled: {ActorBuyer, ActorSeller, ActorAdmin},
	},
	model.StatusPaid: {
		model.StatusShipped:  {ActorSeller},
		model.StatusCanceled: {ActorBuyer, ActorSeller, ActorAdmin},
	},
	model.StatusShipped: {
		model.StatusDelivered: {ActorSeller, ActorBuyer, ActorAdmin, ActorSystem},
	},
	model.StatusDelivered: {
		model.StatusRefundRequested: {ActorBuyer},
	},
	model.StatusRefundRequested: {
		model.StatusRefunded:  {ActorSeller, ActorAdmin},
		model.StatusDelivered: {ActorSeller, ActorAdmin},
	},
}

// CanTransition reports whether actorType may move an order from one status to another.
func CanTransition(from, to, actorType string) bool {
	return slices.Contains(transitions[from][to], actorType)
}

// Errors returned by Transition.
var (
	ErrOrderNotFound = errors.New("order not found")
	ErrNotAllowed    = errors.New("this action is not allowed for the order in its current status")
	ErrReturnWindow  = errors.New("the return window has closed")
	ErrReturnClosed  = errors.New("a return was already rejected for this order")
)

// InvalidTransitionError carries the statuses involved (use errors.Is(err, ErrNotAllowed)).
type InvalidTransitionError struct{ From, To, Actor string }

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("%s can not move an order from %s to %s", e.Actor, e.From, e.To)
}

// Is makes errors.Is(err, ErrNotAllowed) true.
func (e *InvalidTransitionError) Is(target error) bool { return target == ErrNotAllowed }
