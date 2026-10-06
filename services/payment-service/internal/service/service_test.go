package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"payment-service/pkg/events"
	"payment-service/pkg/model"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testSecret = "test-webhook-secret-123456"

// clock is a controllable time source shared by the service under test.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// harness bundles the service, its database, a fake clock and an HTTP server running the real webhook handler.
type harness struct {
	*PaymentService
	clock *clock
	srv   *httptest.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	name := fmt.Sprintf("payment_test_%d", rand.Int63())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create db: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := Config{
		WebhookSecret: testSecret, WalletDelay: 3 * time.Second, TransferDelay: 8 * time.Second, TimeoutDelay: 30 * time.Second,
		Dev: true, WebhookMaxSkew: 5 * time.Minute,
	}
	svc := NewPaymentService(db, zap.NewNop(), cfg)
	clk := &clock{t: time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)}
	svc.Now = clk.now
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/payments/webhook", svc.WebhookHandler())
	srv := httptest.NewServer(mux)
	svc.Cfg.WebhookURL = srv.URL + "/internal/payments/webhook"
	t.Cleanup(func() {
		srv.Close()
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return &harness{PaymentService: svc, clock: clk, srv: srv}
}

// newPayment creates a payment for buyer 5 that expires in 15 minutes.
func (h *harness) newPayment(t *testing.T, checkout, method string, amountMinor int64) *model.Payment {
	t.Helper()
	p, created, err := h.CreatePayment(context.Background(), paymentRequested{
		CheckoutID: checkout, BuyerID: 5, Method: method, AmountMinor: amountMinor, ExpiresAt: h.clock.now().Add(15 * time.Minute), OrderIDs: []uint64{11, 12},
	})
	if err != nil || !created {
		t.Fatalf("create payment: %v created=%v", err, created)
	}
	return p
}

func (h *harness) confirm(p *model.Payment, in ConfirmInput) (*View, error) {
	in.PaymentID, in.BuyerID = p.ID, 5
	return h.Confirm(context.Background(), in)
}

func (h *harness) reload(t *testing.T, id uint64) *model.Payment {
	t.Helper()
	var p model.Payment
	if err := h.DB.First(&p, id).Error; err != nil {
		t.Fatal(err)
	}
	return &p
}

func (h *harness) emitted(t *testing.T, topic string) []map[string]any {
	t.Helper()
	var rows []events.Event
	h.DB.Where("topic = ?", topic).Order("id ASC").Find(&rows)
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var m map[string]any
		_ = json.Unmarshal(r.Payload, &m)
		out = append(out, m)
	}
	return out
}

func card(number string) ConfirmInput {
	return ConfirmInput{CardNumber: number, CardExp: "12/30", CardCVC: "123"}
}

// ---- creation ---------------------------------------------------------------------------------

func TestCreatePaymentIsIdempotentAndValidated(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_abc12345", model.MethodTransfer, 1500000)
	if p.Status != model.StatusRequiresAction || p.Currency != "VND" || p.Provider != "mockpay" {
		t.Errorf("new payment: %+v", p)
	}
	if p.ProviderRef != "MMCO_ABC12345" && p.ProviderRef != "MM"+strings.ToUpper("co_abc12345"[len("co_abc12345")-8:]) {
		t.Errorf("a bank transfer needs a reference for the buyer: %q", p.ProviderRef)
	}
	again, created, err := h.CreatePayment(ctx, paymentRequested{CheckoutID: "co_abc12345", BuyerID: 5, Method: model.MethodCard, AmountMinor: 1, ExpiresAt: h.clock.now().Add(time.Hour)})
	if err != nil || created || again.ID != p.ID || again.AmountMinor != 1500000 || again.Method != model.MethodTransfer {
		t.Errorf("a second request for the same checkout must return the first payment: %+v created=%v err=%v", again, created, err)
	}
	for name, bad := range map[string]paymentRequested{
		"no checkout":    {BuyerID: 5, Method: model.MethodCard, AmountMinor: 1},
		"no buyer":       {CheckoutID: "x", Method: model.MethodCard, AmountMinor: 1},
		"zero amount":    {CheckoutID: "x", BuyerID: 5, Method: model.MethodCard},
		"negative":       {CheckoutID: "x", BuyerID: 5, Method: model.MethodCard, AmountMinor: -5},
		"cod is offline": {CheckoutID: "x", BuyerID: 5, Method: "COD", AmountMinor: 5},
	} {
		if _, _, err := h.CreatePayment(ctx, bad); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, err)
		}
	}
	// a deadline in the past falls back to 15 minutes from now
	late, _, _ := h.CreatePayment(ctx, paymentRequested{CheckoutID: "co_late", BuyerID: 5, Method: model.MethodCard, AmountMinor: 5, ExpiresAt: h.clock.now().Add(-time.Hour)})
	if !late.ExpiresAt.After(h.clock.now()) {
		t.Error("a past deadline must be replaced")
	}
}

// ---- cards -------------------------------------------------------------------------------------

func TestTestCardOutcomes(t *testing.T) {
	cases := []struct {
		name, number, wantStatus, wantFailure string
	}{
		{"success", CardSuccess, model.StatusSucceeded, ""},
		{"declined", CardDeclined, model.StatusRequiresAction, "card_declined"},
		{"insufficient funds", CardInsufficient, model.StatusRequiresAction, "insufficient_funds"},
		{"provider error", CardProviderError, model.StatusRequiresAction, "provider_error"},
		{"unknown card", "5555555555554444", model.StatusRequiresAction, "do_not_honor"},
		{"spaces and dashes are ignored", "4242 4242-4242 4242", model.StatusSucceeded, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			p := h.newPayment(t, "co_1", model.MethodCard, 100000)
			v, err := h.confirm(p, card(c.number))
			if err != nil {
				t.Fatal(err)
			}
			if v.Payment.Status != c.wantStatus || v.Payment.FailureCode != c.wantFailure {
				t.Fatalf("status=%s failure=%s", v.Payment.Status, v.Payment.FailureCode)
			}
			wantEvents := map[string]int{"payment.succeeded": 0, "payment.failed": 0}
			if c.wantStatus == model.StatusSucceeded {
				wantEvents["payment.succeeded"] = 1
			} else {
				wantEvents["payment.failed"] = 1
			}
			for topic, n := range wantEvents {
				if got := len(h.emitted(t, topic)); got != n {
					t.Errorf("%s events = %d, want %d", topic, got, n)
				}
			}
			if c.wantStatus == model.StatusRequiresAction && v.NextAction != "enter_card" {
				t.Errorf("after a decline the buyer can try again, next_action = %s", v.NextAction)
			}
			if c.wantStatus == model.StatusSucceeded && (v.Payment.PaidAt == nil || v.NextAction != "none") {
				t.Errorf("paid payment: %+v next=%s", v.Payment, v.NextAction)
			}
		})
	}
}

func TestCardDataIsNeverStored(t *testing.T) {
	h := newHarness(t)
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	if _, err := h.confirm(p, ConfirmInput{CardNumber: CardDeclined, CardExp: "12/30", CardCVC: "987"}); err != nil {
		t.Fatal(err)
	}
	var hits int64
	for _, table := range []string{"payments", "attempts", "refunds", "webhook_deliveries", "domain_events"} {
		var n int64
		h.DB.Raw("SELECT count(*) FROM "+table+" t WHERE t::text LIKE ? OR t::text LIKE ?", "%"+CardDeclined+"%", "%987%").Scan(&n)
		hits += n
	}
	if hits != 0 {
		t.Errorf("the card number or cvc was persisted (%d rows)", hits)
	}
	if got := h.reload(t, p.ID); got.CardLast4 != "0002" {
		t.Errorf("only the last four digits are kept: %q", got.CardLast4)
	}
}

func TestCardInputValidation(t *testing.T) {
	h := newHarness(t)
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	bad := map[string]ConfirmInput{
		"short number":   {CardNumber: "4242", CardExp: "12/30", CardCVC: "123"},
		"letters":        {CardNumber: "4242abcd42424242", CardExp: "12/30", CardCVC: "123"},
		"expired card":   {CardNumber: CardSuccess, CardExp: "09/26", CardCVC: "123"},
		"bad expiry":     {CardNumber: CardSuccess, CardExp: "13/30", CardCVC: "123"},
		"expiry format":  {CardNumber: CardSuccess, CardExp: "2030-12", CardCVC: "123"},
		"short cvc":      {CardNumber: CardSuccess, CardExp: "12/30", CardCVC: "12"},
		"cvc with text":  {CardNumber: CardSuccess, CardExp: "12/30", CardCVC: "12a"},
		"nothing at all": {},
	}
	for name, in := range bad {
		if _, err := h.confirm(p, in); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, err)
		}
	}
	// the current month is still valid, bad input never counts as a failed attempt
	h.clock.t = time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	if _, err := h.confirm(p, ConfirmInput{CardNumber: CardSuccess, CardExp: "10/26", CardCVC: "123"}); err != nil {
		t.Errorf("a card expiring this month is valid: %v", err)
	}
	if got := h.reload(t, p.ID); got.FailedCount != 0 {
		t.Errorf("validation errors must not count as failed attempts: %d", got.FailedCount)
	}
}

func TestFiveFailedAttemptsEndThePayment(t *testing.T) {
	h := newHarness(t)
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	for i := 1; i <= model.MaxFailedAttempts; i++ {
		v, err := h.confirm(p, card(CardDeclined))
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		wantTerminal := i == model.MaxFailedAttempts
		if (v.Payment.Status == model.StatusFailed) != wantTerminal {
			t.Fatalf("attempt %d: status %s", i, v.Payment.Status)
		}
	}
	failed := h.emitted(t, "payment.failed")
	if len(failed) != 5 || failed[4]["terminal"] != true || failed[0]["terminal"] != false {
		t.Errorf("failure events: %v", failed)
	}
	if _, err := h.confirm(p, card(CardSuccess)); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("a failed payment can not be retried: %v", err)
	}
}

func TestThreeDSecureNeedsTheOTP(t *testing.T) {
	h := newHarness(t)
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	v, err := h.confirm(p, card(CardThreeDSecure))
	if err != nil || v.Payment.Status != model.StatusRequiresAction || v.NextAction != "otp" {
		t.Fatalf("challenge: %v %+v next=%s", err, v.Payment, v.NextAction)
	}
	if len(h.emitted(t, "payment.succeeded")) != 0 {
		t.Fatal("not paid before the OTP")
	}
	v, err = h.confirm(p, ConfirmInput{OTP: "000000"})
	if err != nil || v.Payment.FailureCode != "otp_incorrect" || v.Payment.Status != model.StatusRequiresAction || v.NextAction != "otp" {
		t.Fatalf("wrong otp: %v %+v", err, v.Payment)
	}
	v, err = h.confirm(p, ConfirmInput{OTP: OTPCode})
	if err != nil || v.Payment.Status != model.StatusSucceeded || v.Payment.PaidAt == nil {
		t.Fatalf("right otp: %v %+v", err, v)
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 1 {
		t.Errorf("succeeded events = %d", n)
	}
}

// ---- asynchronous outcomes ----------------------------------------------------------------------

func TestTimeoutCardIsAnsweredByWebhookLater(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	v, err := h.confirm(p, card(CardTimeout))
	if err != nil || v.Payment.Status != model.StatusProcessing || v.NextAction != "wait" {
		t.Fatalf("submit: %v %+v", err, v)
	}
	if _, err := h.confirm(p, card(CardSuccess)); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("a payment being processed can not be paid again: %v", err)
	}
	h.clock.advance(20 * time.Second)
	if n, _ := h.DeliverDueWebhooks(ctx); n != 0 {
		t.Fatalf("the timeout card answers after 30 s, delivered %d at 20 s", n)
	}
	h.clock.advance(11 * time.Second)
	if n, err := h.DeliverDueWebhooks(ctx); err != nil || n != 1 {
		t.Fatalf("delivery at 31 s: %d %v", n, err)
	}
	if got := h.reload(t, p.ID); got.Status != model.StatusSucceeded || got.PaidAt == nil {
		t.Fatalf("after the webhook: %+v", got)
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 1 {
		t.Errorf("succeeded events = %d", n)
	}
	var d model.WebhookDelivery
	h.DB.First(&d)
	if d.Status != model.WebhookDelivered || d.Attempts != 1 || d.LastCode != 200 || d.DeliveredAt == nil {
		t.Errorf("delivery row: %+v", d)
	}
}

func TestDoubleWebhookCardPaysOnce(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	if _, err := h.confirm(p, card(CardDoubleWebhook)); err != nil {
		t.Fatal(err)
	}
	var queued int64
	h.DB.Model(&model.WebhookDelivery{}).Count(&queued)
	if queued != 2 {
		t.Fatalf("the provider notifies twice, queued %d", queued)
	}
	h.clock.advance(5 * time.Second)
	if n, err := h.DeliverDueWebhooks(ctx); err != nil || n != 2 {
		t.Fatalf("delivered %d: %v", n, err)
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 1 {
		t.Errorf("the duplicate notification must not pay twice, got %d events", n)
	}
	var attempts int64
	h.DB.Model(&model.Attempt{}).Where("status = ?", model.AttemptSucceeded).Count(&attempts)
	if attempts != 1 {
		t.Errorf("succeeded attempts = %d", attempts)
	}
}

func TestWebhookAfterExpiryStillPaysSoOrdersCanRefund(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	if v, err := h.confirm(p, card(CardWebhookAfterExpiry)); err != nil || v.Payment.Status != model.StatusProcessing {
		t.Fatalf("submit: %v", err)
	}
	h.clock.advance(14 * time.Minute)
	if n, _ := h.DeliverDueWebhooks(ctx); n != 0 {
		t.Fatal("the notification is held back until after the deadline")
	}
	if n, _ := h.ExpirePayments(ctx); n != 0 {
		t.Error("a payment being processed must not be expired")
	}
	h.clock.advance(2*time.Minute + 5*time.Second)
	if n, err := h.DeliverDueWebhooks(ctx); err != nil || n != 1 {
		t.Fatalf("late delivery: %d %v", n, err)
	}
	got := h.reload(t, p.ID)
	if got.Status != model.StatusSucceeded || !got.PaidAt.After(got.ExpiresAt) {
		t.Fatalf("the money arrived after the deadline: %+v", got)
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 1 {
		t.Errorf("succeeded events = %d (order-service refunds if the order expired)", n)
	}
}

func TestWalletAndBankTransfer(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	wallet := h.newPayment(t, "co_w", model.MethodWallet, 100000)
	if v, _ := h.confirm(wallet, ConfirmInput{Approve: false}); v.Payment.FailureCode != "user_cancelled" || v.Payment.Status != model.StatusRequiresAction {
		t.Errorf("declined in the wallet: %+v", v.Payment)
	}
	v, err := h.confirm(wallet, ConfirmInput{Approve: true})
	if err != nil || v.Payment.Status != model.StatusProcessing {
		t.Fatalf("approve: %v", err)
	}
	h.clock.advance(2 * time.Second)
	if n, _ := h.DeliverDueWebhooks(ctx); n != 0 {
		t.Error("the wallet answers after 3 s")
	}
	h.clock.advance(2 * time.Second)
	if n, _ := h.DeliverDueWebhooks(ctx); n != 1 || h.reload(t, wallet.ID).Status != model.StatusSucceeded {
		t.Errorf("wallet paid after 4 s: delivered %d", n)
	}

	transfer := h.newPayment(t, "co_t", model.MethodTransfer, 100000)
	if _, err := h.confirm(transfer, ConfirmInput{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("transfer without confirming: %v", err)
	}
	v, _ = h.confirm(transfer, ConfirmInput{Approve: true})
	if v.Payment.Status != model.StatusProcessing {
		t.Fatalf("transfer submitted: %s", v.Payment.Status)
	}
	h.clock.advance(5 * time.Second)
	if n, _ := h.DeliverDueWebhooks(ctx); n != 0 {
		t.Error("transfers take 8 s")
	}
	h.clock.advance(4 * time.Second)
	if n, _ := h.DeliverDueWebhooks(ctx); n != 1 || h.reload(t, transfer.ID).Status != model.StatusSucceeded {
		t.Error("transfer should be paid after 9 s")
	}
}

// ---- webhook endpoint -----------------------------------------------------------------------------

func postWebhook(url string, body []byte, header string) int {
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	if header != "" {
		req.Header.Set("X-Signature", header)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestWebhookSignatureAndIdempotency(t *testing.T) {
	h := newHarness(t)
	p := h.newPayment(t, "co_1", model.MethodWallet, 100000)
	body := []byte(fmt.Sprintf(`{"provider_event_id":"evt_manual_1","type":"payment.succeeded","payment_id":%d}`, p.ID))
	good := Sign(testSecret, h.clock.now(), body)

	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", body, ""); c != 401 {
		t.Errorf("no signature: %d", c)
	}
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", body, Sign("another-secret-1234567", h.clock.now(), body)); c != 401 {
		t.Errorf("wrong secret: %d", c)
	}
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", append(body, ' '), good); c != 401 {
		t.Errorf("tampered body: %d", c)
	}
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", body, Sign(testSecret, h.clock.now().Add(-10*time.Minute), body)); c != 401 {
		t.Errorf("replayed old signature: %d", c)
	}
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", body, "garbage"); c != 401 {
		t.Errorf("garbage header: %d", c)
	}
	if got := h.reload(t, p.ID); got.Status != model.StatusRequiresAction {
		t.Fatal("rejected webhooks must not change anything")
	}
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", body, good); c != 200 {
		t.Fatalf("valid webhook: %d", c)
	}
	if got := h.reload(t, p.ID); got.Status != model.StatusSucceeded {
		t.Fatalf("status %s", got.Status)
	}
	// the same event again is acknowledged but changes nothing
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", body, good); c != 200 {
		t.Errorf("duplicate: %d", c)
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 1 {
		t.Errorf("events = %d", n)
	}
	// malformed / unknown
	bad := []byte(`{"type":"payment.succeeded"}`)
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", bad, Sign(testSecret, h.clock.now(), bad)); c != 400 {
		t.Errorf("missing fields: %d", c)
	}
	unknown := []byte(`{"provider_event_id":"e2","type":"payment.succeeded","payment_id":424242}`)
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", unknown, Sign(testSecret, h.clock.now(), unknown)); c != 404 {
		t.Errorf("unknown payment: %d", c)
	}
	weird := []byte(fmt.Sprintf(`{"provider_event_id":"e3","type":"payment.exploded","payment_id":%d}`, p.ID))
	if c := postWebhook(h.srv.URL+"/internal/payments/webhook", weird, Sign(testSecret, h.clock.now(), weird)); c != 400 {
		t.Errorf("unknown type: %d", c)
	}
	// a provider reported failure is recorded as a failed attempt
	p2 := h.newPayment(t, "co_2", model.MethodWallet, 5000)
	f := []byte(fmt.Sprintf(`{"provider_event_id":"e4","type":"payment.failed","payment_id":%d,"failure_code":"wallet_limit"}`, p2.ID))
	postWebhook(h.srv.URL+"/internal/payments/webhook", f, Sign(testSecret, h.clock.now(), f))
	if got := h.reload(t, p2.ID); got.FailureCode != "wallet_limit" || got.FailedCount != 1 {
		t.Errorf("provider failure: %+v", got)
	}
}

func TestSignatureHelpers(t *testing.T) {
	now := time.Now()
	body := []byte("hello")
	if err := VerifySignature("s3cret-s3cret-s3cret", Sign("s3cret-s3cret-s3cret", now, body), body, now, time.Minute); err != nil {
		t.Errorf("valid signature: %v", err)
	}
	for name, header := range map[string]string{
		"empty": "", "no v1": "t=123", "no t": "v1=abc", "non numeric t": "t=abc,v1=abc",
		"future": Sign("s3cret-s3cret-s3cret", now.Add(time.Hour), body),
	} {
		if VerifySignature("s3cret-s3cret-s3cret", header, body, now, time.Minute) == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestWebhookDeliveryRetriesWithBackoffThenGivesUp(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.srv.Close() // our own endpoint is down
	p := h.newPayment(t, "co_1", model.MethodWallet, 100000)
	if _, err := h.confirm(p, ConfirmInput{Approve: true}); err != nil {
		t.Fatal(err)
	}
	var due []time.Duration
	for round := 1; round <= 6; round++ {
		h.clock.advance(11 * time.Minute) // later than the longest backoff
		if n, err := h.DeliverDueWebhooks(ctx); err != nil || n != 0 {
			t.Fatalf("round %d: delivered=%d err=%v", round, n, err)
		}
		var d model.WebhookDelivery
		h.DB.First(&d)
		if d.Attempts != round {
			t.Fatalf("round %d: attempts=%d", round, d.Attempts)
		}
		due = append(due, d.DeliverAt.Sub(h.clock.now()))
	}
	var d model.WebhookDelivery
	h.DB.First(&d)
	if d.Status != model.WebhookGaveUp {
		t.Fatalf("after 6 attempts the delivery is abandoned: %+v", d)
	}
	if due[0] != 2*time.Second || due[1] != 10*time.Second || due[2] != 30*time.Second || due[3] != 2*time.Minute || due[4] != 10*time.Minute {
		t.Errorf("backoff schedule: %v", due)
	}
	if h.reload(t, p.ID).Status != model.StatusProcessing {
		t.Error("an undelivered notification must not change the payment")
	}
}

// ---- scope, cancel, expiry ------------------------------------------------------------------------

func TestPaymentsBelongToTheirBuyer(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	other := ConfirmInput{PaymentID: p.ID, BuyerID: 6, CardNumber: CardSuccess, CardExp: "12/30", CardCVC: "123"}
	if _, err := h.Confirm(ctx, other); status.Code(err) != codes.NotFound {
		t.Errorf("another buyer paying: %v", err)
	}
	if _, err := h.Cancel(ctx, p.ID, 6); status.Code(err) != codes.NotFound {
		t.Errorf("another buyer canceling: %v", err)
	}
	if _, err := h.GetPayment(ctx, p.ID, Scope{BuyerID: 6}); status.Code(err) != codes.NotFound {
		t.Errorf("another buyer reading: %v", err)
	}
	if _, err := h.GetByCheckout(ctx, "co_1", Scope{BuyerID: 6}); status.Code(err) != codes.NotFound {
		t.Errorf("another buyer reading by checkout: %v", err)
	}
	if _, err := h.GetPayment(ctx, p.ID, Scope{}); status.Code(err) != codes.NotFound {
		t.Errorf("an empty scope sees nothing: %v", err)
	}
	if v, err := h.GetByCheckout(ctx, "co_1", Scope{BuyerID: 5}); err != nil || v.Payment.ID != p.ID || len(v.Attempts) != 0 {
		t.Errorf("the owner reads it without admin details: %v", err)
	}
	h.confirm(p, card(CardDeclined))
	if v, _ := h.GetPayment(ctx, p.ID, Scope{Admin: true}); len(v.Attempts) != 1 || v.Attempts[0].FailureCode != "card_declined" {
		t.Errorf("admin sees attempts: %+v", v.Attempts)
	}
	if views, total, _ := h.List(ctx, "requires_action", "mock_card", 1, 10); total != 1 || len(views) != 1 {
		t.Errorf("list filters are case-insensitive: %d", total)
	}
}

func TestCancelAndExpiry(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	a := h.newPayment(t, "co_a", model.MethodCard, 100000)
	if v, err := h.Cancel(ctx, a.ID, 5); err != nil || v.Payment.Status != model.StatusCanceled || v.NextAction != "none" {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := h.Cancel(ctx, a.ID, 5); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("cancel twice: %v", err)
	}
	if _, err := h.confirm(a, card(CardSuccess)); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("paying a canceled payment: %v", err)
	}
	b := h.newPayment(t, "co_b", model.MethodWallet, 100000)
	h.confirm(b, ConfirmInput{Approve: true})
	if _, err := h.Cancel(ctx, b.ID, 5); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("canceling a payment being processed: %v", err)
	}

	c := h.newPayment(t, "co_c", model.MethodCard, 100000)
	h.clock.advance(14 * time.Minute)
	if n, _ := h.ExpirePayments(ctx); n != 0 {
		t.Error("not expired yet")
	}
	h.clock.advance(2 * time.Minute)
	if n, err := h.ExpirePayments(ctx); err != nil || n != 1 { // only the untouched card payment; b is processing, a is canceled
		t.Fatalf("expire: %d %v", n, err)
	}
	if h.reload(t, c.ID).Status != model.StatusExpired || h.reload(t, b.ID).Status != model.StatusProcessing {
		t.Error("expiry must only close payments waiting for the buyer")
	}
	// confirming after the deadline marks it expired and refuses
	d := h.newPayment(t, "co_d", model.MethodCard, 100000)
	h.clock.advance(20 * time.Minute)
	if _, err := h.confirm(d, card(CardSuccess)); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("paying after the deadline: %v", err)
	}
	if h.reload(t, d.ID).Status != model.StatusExpired {
		t.Error("the late attempt must leave the payment expired")
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 0 {
		t.Errorf("nothing was paid, got %d events", n)
	}
}

// ---- refunds ---------------------------------------------------------------------------------------

func TestRefunds(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)

	// a refund before payment is ignored
	if r, err := h.Refund(ctx, RefundInput{CheckoutID: "co_1", AmountMinor: 100, Key: "k0"}); err != nil || r != nil {
		t.Fatalf("refund of an unpaid payment: %v %v", r, err)
	}
	h.confirm(p, card(CardSuccess))

	r, err := h.Refund(ctx, RefundInput{CheckoutID: "co_1", OrderID: 11, AmountMinor: 30000, Reason: "order_canceled", Key: "order:11:cancel"})
	if err != nil || r == nil || r.Status != model.RefundSucceeded {
		t.Fatalf("partial refund: %v %+v", err, r)
	}
	if got := h.reload(t, p.ID); got.Status != model.StatusPartialRefunded || got.RefundedMinor != 30000 {
		t.Fatalf("after partial refund: %+v", got)
	}
	// the same key again is a no-op
	if r, _ := h.Refund(ctx, RefundInput{CheckoutID: "co_1", OrderID: 11, AmountMinor: 30000, Key: "order:11:cancel"}); r != nil {
		t.Error("a repeated refund key must not refund twice")
	}
	// more than what is left is recorded as failed and moves no money
	r, _ = h.Refund(ctx, RefundInput{CheckoutID: "co_1", AmountMinor: 80000, Key: "too-much"})
	if r == nil || r.Status != model.RefundFailed || r.Reason != "exceeds_paid_amount" || h.reload(t, p.ID).RefundedMinor != 30000 {
		t.Fatalf("over-refund: %+v", r)
	}
	if n := len(h.emitted(t, "payment.refunded")); n != 1 {
		t.Errorf("refunded events = %d, want 1 (failed refunds emit nothing)", n)
	}
	// refund the rest -> REFUNDED
	if r, _ = h.Refund(ctx, RefundInput{PaymentID: p.ID, AmountMinor: 70000, Key: "rest"}); r == nil || r.Status != model.RefundSucceeded {
		t.Fatalf("remaining refund: %+v", r)
	}
	if got := h.reload(t, p.ID); got.Status != model.StatusRefunded || got.RefundedMinor != 100000 {
		t.Errorf("fully refunded: %+v", got)
	}
	ev := h.emitted(t, "payment.refunded")
	if len(ev) != 2 || ev[0]["order_id"] != float64(11) || ev[0]["amount_minor"] != float64(30000) || ev[0]["checkout_id"] != "co_1" || ev[1]["order_id"] != float64(0) {
		t.Errorf("events: %v", ev)
	}
	// nothing left to refund
	if r, _ := h.Refund(ctx, RefundInput{PaymentID: p.ID, AmountMinor: 1, Key: "again"}); r != nil && r.Status == model.RefundSucceeded {
		t.Error("a fully refunded payment can not be refunded again")
	}
	if _, err := h.Refund(ctx, RefundInput{CheckoutID: "co_1", AmountMinor: 0, Key: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("zero amount: %v", err)
	}
}

func TestAdminRefundAndForceStatus(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	if _, err := h.AdminRefund(ctx, p.ID, 100, "goodwill"); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("admin refund of an unpaid payment: %v", err)
	}
	if v, err := h.ForceStatus(ctx, p.ID, "succeeded"); err != nil || v.Payment.Status != model.StatusSucceeded {
		t.Fatalf("force success: %v", err)
	}
	if v, err := h.AdminRefund(ctx, p.ID, 40000, "goodwill"); err != nil || v.Payment.RefundedMinor != 40000 || len(v.Refunds) != 1 {
		t.Fatalf("admin refund: %v %+v", err, v)
	}
	if _, err := h.AdminRefund(ctx, p.ID, 90000, "too much"); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("admin over-refund: %v", err)
	}
	q := h.newPayment(t, "co_2", model.MethodCard, 100000)
	if _, err := h.ForceStatus(ctx, q.ID, "bogus"); status.Code(err) != codes.InvalidArgument {
		t.Errorf("unknown forced status: %v", err)
	}
	h.ForceStatus(ctx, q.ID, "EXPIRED")
	if h.reload(t, q.ID).Status != model.StatusExpired {
		t.Error("forced expiry")
	}
	h.Cfg.Dev = false
	if _, err := h.ForceStatus(ctx, q.ID, "SUCCEEDED"); status.Code(err) != codes.PermissionDenied {
		t.Errorf("force outside dev: %v", err)
	}
}

// ---- Kafka consumers ---------------------------------------------------------------------------------

func TestConsumers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	req := func(v any) *kafka.Message { b, _ := json.Marshal(v); return &kafka.Message{Value: b} }

	ev := map[string]any{"checkout_id": "co_9", "buyer_id": 5, "method": "MOCK_WALLET", "amount_minor": 250000, "expires_at": h.clock.now().Add(15 * time.Minute), "order_ids": []int{1, 2}}
	for i := 0; i < 3; i++ { // at-least-once delivery
		if err := h.HandlePaymentRequested(ctx, req(ev)); err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	h.DB.Model(&model.Payment{}).Where("checkout_id = ?", "co_9").Count(&n)
	if n != 1 {
		t.Fatalf("duplicate payment.requested created %d payments", n)
	}
	for name, msg := range map[string]*kafka.Message{
		"garbage":          {Value: []byte("not json")},
		"invalid amount":   req(map[string]any{"checkout_id": "co_x", "buyer_id": 5, "method": "MOCK_CARD", "amount_minor": 0}),
		"unsupported kind": req(map[string]any{"checkout_id": "co_y", "buyer_id": 5, "method": "COD", "amount_minor": 5}),
	} {
		if err := h.HandlePaymentRequested(ctx, msg); err != nil {
			t.Errorf("%s must be dropped, not retried: %v", name, err)
		}
	}
	h.DB.Model(&model.Payment{}).Count(&n)
	if n != 1 {
		t.Errorf("invalid requests created payments: %d", n)
	}

	p := h.reload(t, 1)
	h.ForceStatus(ctx, p.ID, "SUCCEEDED")
	refund := map[string]any{"checkout_id": "co_9", "order_id": 0, "amount_minor": 50000, "reason": "order_not_payable", "refund_key": "checkout:co_9:overpay"}
	for i := 0; i < 3; i++ {
		if err := h.HandleRefundRequested(ctx, req(refund)); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.reload(t, p.ID); got.RefundedMinor != 50000 {
		t.Errorf("a refund request delivered 3 times refunded %d", got.RefundedMinor)
	}
	if err := h.HandleRefundRequested(ctx, &kafka.Message{Value: []byte("nope")}); err != nil {
		t.Errorf("malformed refund request: %v", err)
	}
	if err := h.HandleRefundRequested(ctx, req(map[string]any{"checkout_id": "co_unknown", "amount_minor": 5, "refund_key": "k"})); err != nil {
		t.Errorf("a refund for an unknown checkout is ignored: %v", err)
	}
}

func TestConcurrentConfirmationsPayOnce(t *testing.T) {
	h := newHarness(t)
	p := h.newPayment(t, "co_1", model.MethodCard, 100000)
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.confirm(p, card(CardSuccess))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else if status.Code(err) != codes.FailedPrecondition {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d confirmations succeeded, want exactly 1", ok)
	}
	if n := len(h.emitted(t, "payment.succeeded")); n != 1 {
		t.Errorf("succeeded events = %d", n)
	}
}

func TestConfigRequiresAWebhookSecret(t *testing.T) {
	t.Setenv("PAYMENT_WEBHOOK_SECRET", "short")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("a short webhook secret must be rejected")
	}
	t.Setenv("PAYMENT_WEBHOOK_SECRET", "a-long-enough-secret-value")
	t.Setenv("ADMIN_PORT", "9999")
	t.Setenv("ENV", "dev")
	t.Setenv("PAYMENT_CHAOS_RATE", "0.25")
	cfg, err := ConfigFromEnv()
	if err != nil || cfg.WebhookURL != "http://127.0.0.1:9999/internal/payments/webhook" || !cfg.Dev || cfg.ChaosRate != 0.25 {
		t.Errorf("config: %+v %v", cfg, err)
	}
	t.Setenv("ENV", "prod")
	if cfg, _ := ConfigFromEnv(); cfg.Dev {
		t.Error("only ENV=dev enables development tools")
	}
}
