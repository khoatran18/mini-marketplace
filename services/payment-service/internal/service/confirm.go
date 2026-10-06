package service

import (
	"context"
	"fmt"
	"payment-service/pkg/model"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// The simulated cards. Their outcome is fixed so tests and demos are repeatable (documented in
// .claude/docs/platform/04-orders-payments.md). Card numbers are never stored: only the last four digits.
const (
	CardSuccess            = "4242424242424242" // paid immediately
	CardDeclined           = "4000000000000002" // card_declined
	CardInsufficient       = "4000000000009995" // insufficient_funds
	CardThreeDSecure       = "4000000000003220" // asks for an OTP (123456), then succeeds
	CardProviderError      = "4000000000000119" // provider_error (retry allowed)
	CardTimeout            = "4000000000000341" // no answer; the result arrives by webhook after MOCK_TIMEOUT_DELAY_MS
	CardDoubleWebhook      = "4000000000000259" // succeeds, the provider notifies twice (same event id)
	CardWebhookAfterExpiry = "4000000000000067" // succeeds, but the notification arrives after the payment deadline
	OTPCode                = "123456"
)

type outcome int

const (
	outSuccess outcome = iota
	outDecline
	out3DS
	outTimeout
	outDouble
	outLate
)

type cardResult struct {
	name    string
	outcome outcome
	code    string // failure code for declines
}

func cardScenario(number string) cardResult {
	switch number {
	case CardSuccess:
		return cardResult{name: "success", outcome: outSuccess}
	case CardDeclined:
		return cardResult{name: "card_declined", outcome: outDecline, code: "card_declined"}
	case CardInsufficient:
		return cardResult{name: "insufficient_funds", outcome: outDecline, code: "insufficient_funds"}
	case CardThreeDSecure:
		return cardResult{name: "requires_3ds", outcome: out3DS}
	case CardProviderError:
		return cardResult{name: "provider_error", outcome: outDecline, code: "provider_error"}
	case CardTimeout:
		return cardResult{name: "timeout", outcome: outTimeout}
	case CardDoubleWebhook:
		return cardResult{name: "double_webhook", outcome: outDouble}
	case CardWebhookAfterExpiry:
		return cardResult{name: "late_webhook", outcome: outLate}
	}
	return cardResult{name: "do_not_honor", outcome: outDecline, code: "do_not_honor"}
}

var (
	digitsOnly = regexp.MustCompile(`^[0-9]{12,19}$`)
	expPattern = regexp.MustCompile(`^(0[1-9]|1[0-2])/([0-9]{2})$`)
	cvcPattern = regexp.MustCompile(`^[0-9]{3,4}$`)
)

func cleanNumber(s string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(s))
}

func validateCard(number, exp, cvc string, now time.Time) error {
	if !digitsOnly.MatchString(number) {
		return status.Error(codes.InvalidArgument, "card number must have 12-19 digits")
	}
	m := expPattern.FindStringSubmatch(exp)
	if m == nil {
		return status.Error(codes.InvalidArgument, "expiry must look like MM/YY")
	}
	var month, year int
	fmt.Sscanf(m[1], "%d", &month)
	fmt.Sscanf(m[2], "%d", &year)
	expires := time.Date(2000+year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC) // first instant after the expiry month
	if !now.Before(expires) {
		return status.Error(codes.InvalidArgument, "the card has expired")
	}
	if !cvcPattern.MatchString(cvc) {
		return status.Error(codes.InvalidArgument, "cvc must have 3 or 4 digits")
	}
	return nil
}

// ConfirmInput is what the buyer submitted on the simulated payment page.
type ConfirmInput struct {
	PaymentID  uint64
	BuyerID    uint64
	CardNumber string
	CardExp    string
	CardCVC    string
	OTP        string
	Approve    bool
}

// Confirm applies the buyer's action to a payment and returns it. The outcome of cards is chosen by the card number
// (see the constants above); wallets and bank transfers answer asynchronously through a signed webhook.
func (s *PaymentService) Confirm(ctx context.Context, in ConfirmInput) (*View, error) {
	var expired bool
	var out model.Payment
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := lockPayment(tx, in.PaymentID)
		if err != nil {
			return err
		}
		if p.BuyerID != in.BuyerID {
			return status.Error(codes.NotFound, "payment not found")
		}
		switch p.Status {
		case model.StatusRequiresAction:
		case model.StatusProcessing:
			return status.Error(codes.FailedPrecondition, "the payment is being processed")
		case model.StatusSucceeded, model.StatusPartialRefunded, model.StatusRefunded:
			return status.Error(codes.FailedPrecondition, "the payment is already paid")
		default:
			return status.Errorf(codes.FailedPrecondition, "the payment is %s", strings.ToLower(p.Status))
		}
		if s.Now().After(p.ExpiresAt) {
			expired = true
			p.Status = model.StatusExpired
			return tx.Model(p).Update("status", model.StatusExpired).Error
		}
		switch p.Method {
		case model.MethodCard:
			err = s.confirmCard(tx, p, in)
		case model.MethodWallet:
			if in.Approve {
				err = s.submit(tx, p, "wallet", s.Cfg.WalletDelay, 1)
			} else {
				err = s.fail(tx, p, "wallet", "user_cancelled")
			}
		case model.MethodTransfer:
			if !in.Approve {
				return status.Error(codes.InvalidArgument, "confirm that you have made the transfer")
			}
			err = s.submit(tx, p, "bank_transfer", s.Cfg.TransferDelay, 1)
		default:
			return status.Error(codes.FailedPrecondition, "unsupported payment method")
		}
		if err != nil {
			return err
		}
		out = *p
		return nil
	})
	if err != nil {
		return nil, err
	}
	if expired {
		return nil, status.Error(codes.FailedPrecondition, "the payment has expired")
	}
	return s.viewOf(&out, false), nil
}

// submit moves the payment to PROCESSING and schedules the provider's success notification.
func (s *PaymentService) submit(tx *gorm.DB, p *model.Payment, scenario string, delay time.Duration, copies int) error {
	attempt := &model.Attempt{PaymentID: p.ID, Scenario: scenario, Status: model.AttemptPending}
	if err := tx.Create(attempt).Error; err != nil {
		return err
	}
	p.Status = model.StatusProcessing
	if err := tx.Model(p).Update("status", p.Status).Error; err != nil {
		return err
	}
	return s.scheduleWebhook(tx, p, fmt.Sprintf("evt_%d_%d", p.ID, attempt.ID), model.EventSucceeded, "", delay, copies)
}

func (s *PaymentService) confirmCard(tx *gorm.DB, p *model.Payment, in ConfirmInput) error {
	now := s.Now()
	// second step of 3-D Secure: only the OTP is checked
	if p.ThreeDSPassed && in.OTP != "" {
		if in.OTP != OTPCode {
			return s.fail(tx, p, "requires_3ds", "otp_incorrect")
		}
		if err := tx.Create(&model.Attempt{PaymentID: p.ID, Scenario: "requires_3ds", Status: model.AttemptSucceeded}).Error; err != nil {
			return err
		}
		_, err := s.succeed(tx, p, fmt.Sprintf("evt_%d_3ds_%d", p.ID, now.UnixNano()))
		return err
	}
	number := cleanNumber(in.CardNumber)
	if err := validateCard(number, strings.TrimSpace(in.CardExp), strings.TrimSpace(in.CardCVC), now); err != nil {
		return err
	}
	last4 := number[len(number)-4:]
	if p.CardLast4 != last4 {
		p.CardLast4 = last4
		if err := tx.Model(p).Update("card_last4", last4).Error; err != nil {
			return err
		}
	}
	res := cardScenario(number)
	if s.chaos() {
		res = cardResult{name: "chaos", outcome: outDecline, code: "provider_error"}
	}
	switch res.outcome {
	case outSuccess:
		if err := tx.Create(&model.Attempt{PaymentID: p.ID, Scenario: res.name, Status: model.AttemptSucceeded}).Error; err != nil {
			return err
		}
		_, err := s.succeed(tx, p, fmt.Sprintf("evt_%d_card_%d", p.ID, now.UnixNano()))
		return err
	case outDecline:
		return s.fail(tx, p, res.name, res.code)
	case out3DS:
		p.ThreeDSPassed = true
		if err := tx.Create(&model.Attempt{PaymentID: p.ID, Scenario: res.name, Status: model.AttemptPending}).Error; err != nil {
			return err
		}
		return tx.Model(p).Update("three_ds_passed", true).Error
	case outTimeout:
		return s.submit(tx, p, res.name, s.Cfg.TimeoutDelay, 1)
	case outDouble:
		return s.submit(tx, p, res.name, s.Cfg.WalletDelay, 2)
	case outLate:
		delay := p.ExpiresAt.Add(2 * time.Second).Sub(now)
		if delay < s.Cfg.WalletDelay {
			delay = s.Cfg.WalletDelay
		}
		return s.submit(tx, p, res.name, delay, 1)
	}
	return nil
}

// Cancel lets the buyer abandon a payment that has not been submitted.
func (s *PaymentService) Cancel(ctx context.Context, paymentID, buyerID uint64) (*View, error) {
	var out model.Payment
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := lockPayment(tx, paymentID)
		if err != nil {
			return err
		}
		if p.BuyerID != buyerID {
			return status.Error(codes.NotFound, "payment not found")
		}
		if p.Status != model.StatusRequiresAction {
			return status.Errorf(codes.FailedPrecondition, "a %s payment can not be canceled", strings.ToLower(p.Status))
		}
		p.Status = model.StatusCanceled
		if err := tx.Model(p).Update("status", p.Status).Error; err != nil {
			return err
		}
		out = *p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.viewOf(&out, false), nil
}
