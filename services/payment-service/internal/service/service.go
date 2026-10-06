// Package service implements the SIMULATED payment provider ("mockpay") and the payment lifecycle of checkouts.
// Nothing here talks to a real payment network; the behaviours (asynchronous results, declines, 3-D Secure,
// duplicated or late webhooks, refunds, idempotency) mirror what a real integration has to handle.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"payment-service/pkg/events"
	"payment-service/pkg/model"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Topics produced by payment-service (outbox) and consumed from order-service.
const (
	TopicPaymentRequested = "payment.requested"
	TopicRefundRequested  = "order.refund_requested"
	TopicSucceeded        = "payment.succeeded"
	TopicFailed           = "payment.failed"
	TopicRefunded         = "payment.refunded"
)

// Config holds the simulation settings (environment variables, see deploy/.env.example).
type Config struct {
	WebhookURL     string        // PAYMENT_WEBHOOK_URL: where simulated provider notifications are POSTed (our own admin port)
	WebhookSecret  string        // PAYMENT_WEBHOOK_SECRET: HMAC key (>= 16 characters)
	WalletDelay    time.Duration // MOCK_WEBHOOK_DELAY_MS: wallet confirmation latency
	TransferDelay  time.Duration // MOCK_TRANSFER_DELAY_MS: bank transfer confirmation latency
	TimeoutDelay   time.Duration // MOCK_TIMEOUT_DELAY_MS: result of the "timeout" test card
	ChaosRate      float64       // PAYMENT_CHAOS_RATE (dev only): share of card attempts failing with provider_error
	Dev            bool          // ENV=dev enables ForceStatus and chaos
	WebhookMaxSkew time.Duration // accepted clock difference of webhook signatures
}

// ConfigFromEnv reads the configuration; it fails when the webhook secret is missing or weak.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		WebhookURL:     os.Getenv("PAYMENT_WEBHOOK_URL"),
		WebhookSecret:  os.Getenv("PAYMENT_WEBHOOK_SECRET"),
		WalletDelay:    time.Duration(envInt("MOCK_WEBHOOK_DELAY_MS", 3000)) * time.Millisecond,
		TransferDelay:  time.Duration(envInt("MOCK_TRANSFER_DELAY_MS", 8000)) * time.Millisecond,
		TimeoutDelay:   time.Duration(envInt("MOCK_TIMEOUT_DELAY_MS", 30000)) * time.Millisecond,
		Dev:            os.Getenv("ENV") == "dev",
		WebhookMaxSkew: 5 * time.Minute,
	}
	if v, err := strconv.ParseFloat(os.Getenv("PAYMENT_CHAOS_RATE"), 64); err == nil && v > 0 && v <= 1 {
		cfg.ChaosRate = v
	}
	if cfg.WebhookURL == "" {
		port := os.Getenv("ADMIN_PORT")
		if port == "" {
			port = "8081"
		}
		cfg.WebhookURL = "http://127.0.0.1:" + port + "/internal/payments/webhook"
	}
	if len(cfg.WebhookSecret) < 16 {
		return cfg, errors.New("PAYMENT_WEBHOOK_SECRET must be set (at least 16 characters)")
	}
	return cfg, nil
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 {
		return v
	}
	return def
}

// PaymentService holds the business logic.
type PaymentService struct {
	DB     *gorm.DB
	Logger *zap.Logger
	Cfg    Config
	HTTP   *http.Client
	Now    func() time.Time

	rndMu sync.Mutex
	rnd   *rand.Rand
}

// NewPaymentService creates the service.
func NewPaymentService(db *gorm.DB, logger *zap.Logger, cfg Config) *PaymentService {
	return &PaymentService{DB: db, Logger: logger, Cfg: cfg, HTTP: &http.Client{Timeout: 5 * time.Second}, Now: time.Now, rnd: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// Migrate creates the tables.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.Payment{}, &model.Attempt{}, &model.Refund{}, &model.WebhookDelivery{}, &model.ProcessedEvent{}); err != nil {
		return err
	}
	return events.Migrate(db)
}

func (s *PaymentService) chaos() bool {
	if !s.Cfg.Dev || s.Cfg.ChaosRate <= 0 {
		return false
	}
	s.rndMu.Lock()
	defer s.rndMu.Unlock()
	return s.rnd.Float64() < s.Cfg.ChaosRate
}

// ---- events we emit -----------------------------------------------------------------------------

type succeededEvent struct {
	V               int       `json:"v"`
	PaymentID       uint64    `json:"payment_id"`
	CheckoutID      string    `json:"checkout_id"`
	BuyerID         uint64    `json:"buyer_id"`
	Method          string    `json:"method"`
	AmountMinor     int64     `json:"amount_minor"`
	At              time.Time `json:"at"`
	ProviderEventID string    `json:"provider_event_id"`
}

type failedEvent struct {
	V           int       `json:"v"`
	PaymentID   uint64    `json:"payment_id"`
	CheckoutID  string    `json:"checkout_id"`
	BuyerID     uint64    `json:"buyer_id"`
	Method      string    `json:"method"`
	FailureCode string    `json:"failure_code"`
	Terminal    bool      `json:"terminal"`
	At          time.Time `json:"at"`
}

type refundedEvent struct {
	V           int       `json:"v"`
	PaymentID   uint64    `json:"payment_id"`
	RefundID    uint64    `json:"refund_id"`
	CheckoutID  string    `json:"checkout_id"`
	OrderID     uint64    `json:"order_id"`
	AmountMinor int64     `json:"amount_minor"`
	Reason      string    `json:"reason"`
	At          time.Time `json:"at"`
}

func pkey(id uint64) string { return strconv.FormatUint(id, 10) }

// ---- state changes (always inside a transaction that holds the payment row lock) ------------------

func lockPayment(tx *gorm.DB, id uint64) (*model.Payment, error) {
	var p model.Payment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "payment not found")
		}
		return nil, err
	}
	return &p, nil
}

// succeed marks the payment as paid and emits payment.succeeded. Money captured after the buyer canceled or the
// payment expired is still money: the status becomes SUCCEEDED and order-service refunds what no order needs.
// Returns false when the payment was already paid (idempotent).
func (s *PaymentService) succeed(tx *gorm.DB, p *model.Payment, providerEventID string) (bool, error) {
	switch p.Status {
	case model.StatusSucceeded, model.StatusPartialRefunded, model.StatusRefunded:
		return false, nil
	}
	now := s.Now().UTC()
	p.Status, p.PaidAt, p.FailureCode = model.StatusSucceeded, &now, ""
	if err := tx.Model(p).Updates(map[string]any{"status": p.Status, "paid_at": now, "failure_code": ""}).Error; err != nil {
		return false, err
	}
	return true, events.Emit(tx, TopicSucceeded, pkey(p.ID), succeededEvent{
		V: 1, PaymentID: p.ID, CheckoutID: p.CheckoutID, BuyerID: p.BuyerID, Method: p.Method, AmountMinor: p.AmountMinor, At: now, ProviderEventID: providerEventID,
	})
}

// fail records a failed attempt. The payment stays open for another try until MaxFailedAttempts, then FAILED.
func (s *PaymentService) fail(tx *gorm.DB, p *model.Payment, scenario, code string) error {
	if p.Status == model.StatusSucceeded || p.Status == model.StatusPartialRefunded || p.Status == model.StatusRefunded {
		return nil
	}
	if err := tx.Create(&model.Attempt{PaymentID: p.ID, Scenario: scenario, Status: model.AttemptFailed, FailureCode: code}).Error; err != nil {
		return err
	}
	p.FailedCount++
	p.FailureCode = code
	terminal := p.FailedCount >= model.MaxFailedAttempts
	updates := map[string]any{"failed_count": p.FailedCount, "failure_code": code}
	if terminal {
		p.Status = model.StatusFailed
	} else if p.Status == model.StatusProcessing {
		p.Status = model.StatusRequiresAction // a rejected submission re-opens the payment
	}
	updates["status"] = p.Status
	if err := tx.Model(p).Updates(updates).Error; err != nil {
		return err
	}
	return events.Emit(tx, TopicFailed, pkey(p.ID), failedEvent{
		V: 1, PaymentID: p.ID, CheckoutID: p.CheckoutID, BuyerID: p.BuyerID, Method: p.Method, FailureCode: code, Terminal: terminal, At: s.Now().UTC(),
	})
}

// scheduleWebhook queues a simulated provider notification for a payment.
func (s *PaymentService) scheduleWebhook(tx *gorm.DB, p *model.Payment, eventID, eventType, failureCode string, after time.Duration, copies int) error {
	body, _ := json.Marshal(map[string]any{"provider_event_id": eventID, "type": eventType, "payment_id": p.ID, "failure_code": failureCode})
	for i := 0; i < copies; i++ { // copies > 1 simulates a provider that delivers the same event twice
		if err := tx.Create(&model.WebhookDelivery{
			PaymentID: p.ID, ProviderEventID: eventID, Type: eventType, Payload: body, DeliverAt: s.Now().Add(after), Status: model.WebhookPending,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ---- creating payments (from order-service) -------------------------------------------------------

// paymentRequested is the payload of payment.requested.
type paymentRequested struct {
	CheckoutID  string    `json:"checkout_id"`
	BuyerID     uint64    `json:"buyer_id"`
	Method      string    `json:"method"`
	AmountMinor int64     `json:"amount_minor"`
	ExpiresAt   time.Time `json:"expires_at"`
	OrderIDs    []uint64  `json:"order_ids"`
}

// CreatePayment creates the payment of a checkout (idempotent per checkout id) and returns whether it is new.
func (s *PaymentService) CreatePayment(ctx context.Context, req paymentRequested) (*model.Payment, bool, error) {
	if req.CheckoutID == "" || req.BuyerID == 0 || req.AmountMinor <= 0 || !model.ValidMethod(req.Method) {
		return nil, false, status.Error(codes.InvalidArgument, "invalid payment request")
	}
	expires := req.ExpiresAt
	if expires.IsZero() || expires.Before(s.Now()) {
		expires = s.Now().Add(15 * time.Minute)
	}
	ids, _ := json.Marshal(req.OrderIDs)
	p := &model.Payment{
		CheckoutID: req.CheckoutID, BuyerID: req.BuyerID, Method: req.Method, Status: model.StatusRequiresAction, AmountMinor: req.AmountMinor,
		Currency: model.CurrencyVND, Provider: model.ProviderMockPay, ExpiresAt: expires.UTC(), OrderIDs: ids,
	}
	if req.Method == model.MethodTransfer {
		suffix := req.CheckoutID
		if len(suffix) > 8 {
			suffix = suffix[len(suffix)-8:]
		}
		p.ProviderRef = "MM" + strings.ToUpper(suffix) // the "transfer content" the buyer is told to use
	}
	res := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(p)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 0 {
		var existing model.Payment
		if err := s.DB.WithContext(ctx).Where("checkout_id = ?", req.CheckoutID).First(&existing).Error; err != nil {
			return nil, false, err
		}
		return &existing, false, nil
	}
	return p, true, nil
}

// ---- webhook (simulated provider -> us) -------------------------------------------------------------

// Sign returns the X-Signature header value for a body: t=<unix>,v1=HMAC_SHA256(secret, t + "." + body).
func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks the header against the body and rejects signatures older/newer than maxSkew.
func VerifySignature(secret, header string, body []byte, now time.Time, maxSkew time.Duration) error {
	var ts, sig string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			sig = v
		}
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sig == "" {
		return errors.New("malformed signature")
	}
	if d := now.Sub(time.Unix(unix, 0)); d > maxSkew || d < -maxSkew {
		return errors.New("signature timestamp outside the tolerated window")
	}
	want := Sign(secret, time.Unix(unix, 0), body)
	if !hmac.Equal([]byte(want), []byte("t="+ts+",v1="+sig)) {
		return errors.New("signature mismatch")
	}
	return nil
}

type webhookBody struct {
	ProviderEventID string `json:"provider_event_id"`
	Type            string `json:"type"`
	PaymentID       uint64 `json:"payment_id"`
	FailureCode     string `json:"failure_code"`
}

// WebhookHandler is the endpoint the simulated provider calls (mounted on the internal admin port).
func (s *PaymentService) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := VerifySignature(s.Cfg.WebhookSecret, r.Header.Get("X-Signature"), body, s.Now(), s.Cfg.WebhookMaxSkew); err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		var ev webhookBody
		if err := json.Unmarshal(body, &ev); err != nil || ev.ProviderEventID == "" || ev.PaymentID == 0 {
			http.Error(w, "malformed event", http.StatusBadRequest)
			return
		}
		result, err := s.ApplyProviderEvent(r.Context(), ev)
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"` + result + `"}`))
		case status.Code(err) == codes.NotFound:
			http.Error(w, "unknown payment", http.StatusNotFound)
		case status.Code(err) == codes.InvalidArgument:
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			s.Logger.Error("webhook processing failed", zap.Error(err))
			http.Error(w, "processing failed", http.StatusInternalServerError)
		}
	}
}

// ApplyProviderEvent applies a (verified) provider notification exactly once per provider_event_id.
// Returns "applied" or "duplicate".
func (s *PaymentService) ApplyProviderEvent(ctx context.Context, ev webhookBody) (string, error) {
	result := "applied"
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ProcessedEvent{ProviderEventID: ev.ProviderEventID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			result = "duplicate"
			return nil
		}
		p, err := lockPayment(tx, ev.PaymentID)
		if err != nil {
			return err
		}
		switch ev.Type {
		case model.EventSucceeded:
			if err := tx.Create(&model.Attempt{PaymentID: p.ID, Scenario: "webhook", Status: model.AttemptSucceeded}).Error; err != nil {
				return err
			}
			_, err = s.succeed(tx, p, ev.ProviderEventID)
			return err
		case model.EventFailed:
			code := ev.FailureCode
			if code == "" {
				code = "provider_declined"
			}
			return s.fail(tx, p, "webhook", code)
		}
		return status.Errorf(codes.InvalidArgument, "unknown event type %q", ev.Type)
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

// DeliverDueWebhooks sends the notifications that are due to our own webhook endpoint (signed, retried with
// backoff, given up after 6 attempts). Returns how many were delivered successfully.
func (s *PaymentService) DeliverDueWebhooks(ctx context.Context) (int, error) {
	delivered := 0
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var due []model.WebhookDelivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND deliver_at <= ?", model.WebhookPending, s.Now()).Order("deliver_at ASC, id ASC").Limit(20).Find(&due).Error; err != nil {
			return err
		}
		backoff := []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
		for i := range due {
			d := &due[i]
			code := s.postWebhook(ctx, d.Payload)
			d.Attempts++
			updates := map[string]any{"attempts": d.Attempts, "last_code": code}
			switch {
			case code >= 200 && code < 300:
				updates["status"], updates["delivered_at"] = model.WebhookDelivered, s.Now()
				delivered++
			case d.Attempts >= 6:
				updates["status"] = model.WebhookGaveUp
			default:
				updates["deliver_at"] = s.Now().Add(backoff[min(d.Attempts-1, len(backoff)-1)])
			}
			if err := tx.Model(d).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return delivered, err
}

func (s *PaymentService) postWebhook(ctx context.Context, body []byte) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Cfg.WebhookURL, strings.NewReader(string(body)))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", Sign(s.Cfg.WebhookSecret, s.Now(), body))
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// ExpirePayments closes payments whose deadline passed without any buyer action.
func (s *PaymentService) ExpirePayments(ctx context.Context) (int64, error) {
	res := s.DB.WithContext(ctx).Model(&model.Payment{}).
		Where("status = ? AND expires_at < ?", model.StatusRequiresAction, s.Now()).Update("status", model.StatusExpired)
	return res.RowsAffected, res.Error
}

// RunWorkers delivers webhooks and expires payments every interval until ctx is done.
func (s *PaymentService) RunWorkers(ctx context.Context, interval time.Duration, heartbeat func()) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := s.DeliverDueWebhooks(ctx); err != nil {
					s.Logger.Warn("webhook round failed", zap.Error(err))
				}
				if _, err := s.ExpirePayments(ctx); err != nil {
					s.Logger.Warn("expiry round failed", zap.Error(err))
				}
				if heartbeat != nil {
					heartbeat()
				}
			}
		}
	}()
}

func (s *PaymentService) String() string { return fmt.Sprintf("PaymentService(%s)", s.Cfg.WebhookURL) }
