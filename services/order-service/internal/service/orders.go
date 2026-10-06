package service

import (
	"context"
	"errors"
	"order-service/internal/repository"
	"order-service/pkg/model"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// orderError converts repository errors to gRPC status errors.
func orderError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrOrderNotFound):
		return status.Error(codes.NotFound, "order not found")
	case errors.Is(err, repository.ErrNotAllowed), errors.Is(err, repository.ErrReturnWindow), errors.Is(err, repository.ErrReturnClosed):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return err
}

// OrderView is an order with its status history.
type OrderView struct {
	Order   *model.Order
	History []*model.OrderStatusHistory
}

// GetOrder returns an order inside the caller's scope (NotFound otherwise).
func (s *OrderService) GetOrder(ctx context.Context, id uint64, scope repository.Scope) (*OrderView, error) {
	o, h, err := s.OrderRepo.GetOrder(ctx, id, scope)
	if err != nil {
		return nil, orderError(err)
	}
	return &OrderView{Order: o, History: h}, nil
}

// ListOrders returns a page of orders in scope.
func (s *OrderService) ListOrders(ctx context.Context, scope repository.Scope, statusFilter string, page, pageSize uint64) ([]*model.Order, int64, error) {
	if statusFilter != "" && !validStatus(strings.ToUpper(statusFilter)) {
		return nil, 0, status.Error(codes.InvalidArgument, "unknown status")
	}
	return s.OrderRepo.ListOrders(ctx, scope, statusFilter, page, pageSize)
}

func validStatus(st string) bool {
	for _, v := range model.AllStatuses {
		if v == st {
			return true
		}
	}
	return false
}

// CountOrders returns the number of orders per status in scope.
func (s *OrderService) CountOrders(ctx context.Context, scope repository.Scope) (map[string]int64, error) {
	return s.OrderRepo.CountOrders(ctx, scope)
}

// Action names accepted by ApplyAction.
const (
	ActionCancel          = "cancel"
	ActionShip            = "ship"
	ActionDeliver         = "deliver"
	ActionConfirmReceived = "confirm_received"
	ActionRequestReturn   = "request_return"
	ActionApproveReturn   = "approve_return"
	ActionRejectReturn    = "reject_return"
)

// ActionInput is a user action on an order.
type ActionInput struct {
	OrderID      uint64
	Action       string
	Actor        repository.Actor
	Reason       string
	Carrier      string
	TrackingCode string
}

type actionRule struct {
	to     string
	actors []string
}

var actionRules = map[string]actionRule{
	ActionCancel:          {model.StatusCanceled, []string{repository.ActorBuyer, repository.ActorSeller, repository.ActorAdmin}},
	ActionShip:            {model.StatusShipped, []string{repository.ActorSeller}},
	ActionDeliver:         {model.StatusDelivered, []string{repository.ActorSeller, repository.ActorAdmin}},
	ActionConfirmReceived: {model.StatusDelivered, []string{repository.ActorBuyer}},
	ActionRequestReturn:   {model.StatusRefundRequested, []string{repository.ActorBuyer}},
	ActionApproveReturn:   {model.StatusRefunded, []string{repository.ActorSeller, repository.ActorAdmin}},
	ActionRejectReturn:    {model.StatusDelivered, []string{repository.ActorSeller, repository.ActorAdmin}},
}

// ApplyAction performs a user action (cancel, ship, ...) and returns the updated order.
func (s *OrderService) ApplyAction(ctx context.Context, in *ActionInput) (*OrderView, error) {
	rule, ok := actionRules[in.Action]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown action")
	}
	allowed := false
	for _, a := range rule.actors {
		allowed = allowed || a == in.Actor.Type
	}
	if !allowed {
		return nil, status.Errorf(codes.PermissionDenied, "a %s can not %s an order", in.Actor.Type, in.Action)
	}
	reason := strings.TrimSpace(in.Reason)
	if len([]rune(reason)) > 500 {
		return nil, status.Error(codes.InvalidArgument, "reason must be at most 500 characters")
	}
	p := repository.TransitionParams{To: rule.to, Actor: in.Actor, Reason: reason}
	switch in.Action {
	case ActionShip:
		p.Carrier, p.TrackingCode = strings.TrimSpace(in.Carrier), strings.TrimSpace(in.TrackingCode)
		if p.Carrier == "" || len(p.Carrier) > 50 || p.TrackingCode == "" || len(p.TrackingCode) > 64 {
			return nil, status.Error(codes.InvalidArgument, "carrier (max 50) and tracking_code (max 64) are required to ship")
		}
	case ActionCancel:
		if in.Actor.Type != repository.ActorBuyer && reason == "" {
			return nil, status.Error(codes.InvalidArgument, "a reason is required")
		}
	case ActionRequestReturn, ActionRejectReturn:
		if reason == "" {
			return nil, status.Error(codes.InvalidArgument, "a reason is required")
		}
		p.ReturnWindow = s.Settings.ReturnWindow
	}
	o, err := s.OrderRepo.Transition(ctx, in.OrderID, p)
	if err != nil {
		return nil, orderError(err)
	}
	scope := repository.Scope{Admin: true}
	_, history, err := s.OrderRepo.GetOrder(ctx, o.ID, scope)
	if err != nil {
		return nil, orderError(err)
	}
	return &OrderView{Order: o, History: history}, nil
}
