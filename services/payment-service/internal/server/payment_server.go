package server

import (
	"context"
	"payment-service/internal/service"
	paymentpb "payment-service/pkg/pb"
	"time"

	"buf.build/go/protovalidate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PaymentServer adapts the gRPC API to PaymentService.
type PaymentServer struct {
	paymentpb.UnimplementedPaymentServiceServer
	Service *service.PaymentService
}

func ts(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func toProto(v *service.View) *paymentpb.Payment {
	p := v.Payment
	out := &paymentpb.Payment{
		Id: p.ID, CheckoutId: p.CheckoutID, BuyerId: p.BuyerID, Method: p.Method, Status: p.Status, AmountMinor: p.AmountMinor,
		Currency: p.Currency, Provider: p.Provider, ProviderRef: p.ProviderRef, FailureCode: p.FailureCode, CardLast4: p.CardLast4,
		ExpiresAt: p.ExpiresAt.UTC().Format(time.RFC3339), PaidAt: ts(p.PaidAt), CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		RefundedMinor: p.RefundedMinor, NextAction: v.NextAction, OrderIds: v.OrderIDs,
	}
	for _, a := range v.Attempts {
		out.Attempts = append(out.Attempts, &paymentpb.Attempt{Id: a.ID, Scenario: a.Scenario, Status: a.Status, FailureCode: a.FailureCode, At: a.At.UTC().Format(time.RFC3339)})
	}
	for _, r := range v.Refunds {
		out.Refunds = append(out.Refunds, &paymentpb.Refund{
			Id: r.ID, PaymentId: r.PaymentID, OrderId: r.OrderID, AmountMinor: r.AmountMinor, Reason: r.Reason, Status: r.Status, RefundKey: r.RefundKey, At: r.At.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func scope(buyerID uint64, admin bool) service.Scope {
	return service.Scope{BuyerID: buyerID, Admin: admin}
}

// GetPayment returns a payment in the caller's scope.
func (s *PaymentServer) GetPayment(ctx context.Context, req *paymentpb.GetPaymentRequest) (*paymentpb.Payment, error) {
	v, err := s.Service.GetPayment(ctx, req.GetId(), scope(req.GetBuyerId(), req.GetIsAdmin()))
	if err != nil {
		return nil, err
	}
	return toProto(v), nil
}

// GetPaymentByCheckout returns the payment of a checkout in the caller's scope.
func (s *PaymentServer) GetPaymentByCheckout(ctx context.Context, req *paymentpb.GetPaymentByCheckoutRequest) (*paymentpb.Payment, error) {
	v, err := s.Service.GetByCheckout(ctx, req.GetCheckoutId(), scope(req.GetBuyerId(), req.GetIsAdmin()))
	if err != nil {
		return nil, err
	}
	return toProto(v), nil
}

// ConfirmPayment submits the buyer's action on the simulated payment page.
func (s *PaymentServer) ConfirmPayment(ctx context.Context, req *paymentpb.ConfirmPaymentRequest) (*paymentpb.Payment, error) {
	if req.GetBuyerId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "buyer_id is required")
	}
	v, err := s.Service.Confirm(ctx, service.ConfirmInput{
		PaymentID: req.GetPaymentId(), BuyerID: req.GetBuyerId(), CardNumber: req.GetCardNumber(), CardExp: req.GetCardExp(),
		CardCVC: req.GetCardCvc(), OTP: req.GetOtp(), Approve: req.GetApprove(),
	})
	if err != nil {
		return nil, err
	}
	return toProto(v), nil
}

// CancelPayment abandons an unsubmitted payment.
func (s *PaymentServer) CancelPayment(ctx context.Context, req *paymentpb.CancelPaymentRequest) (*paymentpb.Payment, error) {
	v, err := s.Service.Cancel(ctx, req.GetPaymentId(), req.GetBuyerId())
	if err != nil {
		return nil, err
	}
	return toProto(v), nil
}

// ListPayments returns a page of payments (admin).
func (s *PaymentServer) ListPayments(ctx context.Context, req *paymentpb.ListPaymentsRequest) (*paymentpb.ListPaymentsResponse, error) {
	views, total, err := s.Service.List(ctx, req.GetStatus(), req.GetMethod(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	out := &paymentpb.ListPaymentsResponse{Total: uint64(total)}
	for _, v := range views {
		out.Payments = append(out.Payments, toProto(v))
	}
	return out, nil
}

// RefundPayment refunds part or all of a payment (admin).
func (s *PaymentServer) RefundPayment(ctx context.Context, req *paymentpb.RefundPaymentRequest) (*paymentpb.Payment, error) {
	if err := protovalidate.Validate(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	v, err := s.Service.AdminRefund(ctx, req.GetPaymentId(), req.GetAmountMinor(), req.GetReason())
	if err != nil {
		return nil, err
	}
	return toProto(v), nil
}

// ForceStatus applies a provider outcome immediately (development only).
func (s *PaymentServer) ForceStatus(ctx context.Context, req *paymentpb.ForceStatusRequest) (*paymentpb.Payment, error) {
	v, err := s.Service.ForceStatus(ctx, req.GetPaymentId(), req.GetStatus())
	if err != nil {
		return nil, err
	}
	return toProto(v), nil
}
