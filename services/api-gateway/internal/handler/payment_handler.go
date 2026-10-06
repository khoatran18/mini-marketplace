package handler

import (
	"api-gateway/internal/client"
	"api-gateway/pkg/dto"
	paymentpb "api-gateway/pkg/pb/paymentservice"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PaymentHandler exposes the SIMULATED payment flow. Buyers only ever act on their own payments; the buyer id comes
// from the token. Card numbers received here only select the simulated outcome and are never logged.
type PaymentHandler struct {
	Clients *client.ClientManager
	Logger  *zap.Logger
}

// NewPaymentHandler creates a PaymentHandler.
func NewPaymentHandler(cm *client.ClientManager, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{Clients: cm, Logger: logger}
}

func (h *PaymentHandler) payment(c *gin.Context) (paymentpb.PaymentServiceClient, context.Context, context.CancelFunc, bool) {
	pc, err := h.Clients.Payment()
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: payment client", err)
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	return pc, ctx, cancel, true
}

// paymentJSON is the public representation: amounts in VND units next to the minor units, the payment page URL, and a
// clear "simulated" marker. Admin details (attempts, refunds) are included only when present in the message.
func paymentJSON(p *paymentpb.Payment) map[string]any {
	m := messageToMap(p.ProtoReflect())
	m["amount"] = float64(p.GetAmountMinor()) / 100
	m["refunded"] = float64(p.GetRefundedMinor()) / 100
	m["pay_url"] = "/pay/" + strconv.FormatUint(p.GetId(), 10)
	m["simulated"] = true
	if len(p.GetAttempts()) == 0 {
		delete(m, "attempts")
	}
	if len(p.GetRefunds()) == 0 {
		delete(m, "refunds")
	}
	return m
}

// ForCheckout implements PaymentComposer: the payment of a checkout, or nil when it does not exist (yet).
func (h *PaymentHandler) ForCheckout(ctx context.Context, buyerID uint64, checkoutID string) map[string]any {
	pc, err := h.Clients.Payment()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := pc.GetPaymentByCheckout(ctx, &paymentpb.GetPaymentByCheckoutRequest{CheckoutId: checkoutID, BuyerId: buyerID})
	if err != nil {
		if status.Code(err) != codes.NotFound {
			h.Logger.Warn("PaymentHandler: payment lookup failed", zap.Error(err))
		}
		return nil
	}
	return paymentJSON(res)
}

// Methods lists the available payment methods. All online methods are simulations; COD is the offline method.
func (h *PaymentHandler) Methods(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"methods": []gin.H{
		{"code": "COD", "label": "Thanh toán khi nhận hàng (COD)", "online": false, "simulated": false},
		{"code": "MOCK_CARD", "label": "Thẻ (MÔ PHỎNG)", "online": true, "simulated": true,
			"test_cards": []gin.H{
				{"number": "4242 4242 4242 4242", "result": "Thành công ngay"},
				{"number": "4000 0000 0000 0002", "result": "Bị từ chối (card_declined)"},
				{"number": "4000 0000 0000 9995", "result": "Không đủ tiền (insufficient_funds)"},
				{"number": "4000 0000 0000 3220", "result": "Cần xác thực 3-D Secure, mã OTP 123456"},
				{"number": "4000 0000 0000 0119", "result": "Lỗi nhà cung cấp tạm thời (thử lại được)"},
				{"number": "4000 0000 0000 0341", "result": "Không phản hồi, kết quả về sau ~30 giây qua webhook"},
				{"number": "4000 0000 0000 0259", "result": "Thành công, webhook gửi 2 lần"},
				{"number": "4000 0000 0000 0067", "result": "Thành công nhưng webhook đến sau hạn thanh toán"},
			}},
		{"code": "MOCK_WALLET", "label": "Ví điện tử (MÔ PHỎNG)", "online": true, "simulated": true},
		{"code": "MOCK_BANK_TRANSFER", "label": "Chuyển khoản (MÔ PHỎNG)", "online": true, "simulated": true},
	}})
}

// Get returns one of the caller's payments.
func (h *PaymentHandler) Get(c *gin.Context) {
	v, err := resolveViewer(c, nil)
	if err != nil || !v.Authenticated() {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.GetPayment(ctx, &paymentpb.GetPaymentRequest{Id: id, BuyerId: v.UserID})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: GetPayment", err)
		return
	}
	c.JSON(http.StatusOK, paymentJSON(res))
}

type confirmBody struct {
	CardNumber string `json:"card_number"`
	CardExp    string `json:"card_exp"`
	CardCVC    string `json:"card_cvc"`
	OTP        string `json:"otp"`
	Approve    bool   `json:"approve"`
}

// Confirm submits the buyer's action on the simulated payment page.
func (h *PaymentHandler) Confirm(c *gin.Context) {
	v, err := resolveViewer(c, nil)
	if err != nil || !v.Authenticated() {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body confirmBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid request body") // never echo the body: it may contain card data
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.ConfirmPayment(ctx, &paymentpb.ConfirmPaymentRequest{
		PaymentId: id, BuyerId: v.UserID, CardNumber: body.CardNumber, CardExp: body.CardExp, CardCvc: body.CardCVC, Otp: body.OTP, Approve: body.Approve,
	})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: ConfirmPayment", err)
		return
	}
	c.JSON(http.StatusOK, paymentJSON(res))
}

// Cancel abandons an unsubmitted payment of the caller.
func (h *PaymentHandler) Cancel(c *gin.Context) {
	v, err := resolveViewer(c, nil)
	if err != nil || !v.Authenticated() {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.CancelPayment(ctx, &paymentpb.CancelPaymentRequest{PaymentId: id, BuyerId: v.UserID})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: CancelPayment", err)
		return
	}
	c.JSON(http.StatusOK, paymentJSON(res))
}

// AdminList lists payments (admin).
func (h *PaymentHandler) AdminList(c *gin.Context) {
	page, size, ok := pageParams(c)
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.ListPayments(ctx, &paymentpb.ListPaymentsRequest{Status: c.Query("status"), Method: c.Query("method"), Page: page, PageSize: size})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: ListPayments", err)
		return
	}
	items := make([]map[string]any, 0, len(res.GetPayments()))
	for _, p := range res.GetPayments() {
		items = append(items, paymentJSON(p))
	}
	c.JSON(http.StatusOK, gin.H{"payments": items, "total": res.GetTotal()})
}

// AdminGet returns any payment with attempts and refunds (admin).
func (h *PaymentHandler) AdminGet(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.GetPayment(ctx, &paymentpb.GetPaymentRequest{Id: id, IsAdmin: true})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: GetPayment (admin)", err)
		return
	}
	c.JSON(http.StatusOK, paymentJSON(res))
}

type refundBody struct {
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
}

// AdminRefund refunds part or all of a payment (admin).
func (h *PaymentHandler) AdminRefund(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body refundBody
	if err := c.ShouldBindJSON(&body); err != nil || body.AmountMinor <= 0 {
		badRequest(c, "amount_minor must be a positive number")
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.RefundPayment(ctx, &paymentpb.RefundPaymentRequest{PaymentId: id, AmountMinor: body.AmountMinor, Reason: body.Reason})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: RefundPayment", err)
		return
	}
	c.JSON(http.StatusOK, paymentJSON(res))
}

type forceBody struct {
	Status string `json:"status" binding:"required"`
}

// AdminForce applies a provider outcome immediately; payment-service only allows it when ENV=dev.
func (h *PaymentHandler) AdminForce(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body forceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	pc, ctx, cancel, ok := h.payment(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.ForceStatus(ctx, &paymentpb.ForceStatusRequest{PaymentId: id, Status: body.Status})
	if err != nil {
		respondError(c, h.Logger, "PaymentHandler: ForceStatus", err)
		return
	}
	c.JSON(http.StatusOK, paymentJSON(res))
}
