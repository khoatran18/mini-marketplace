package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	analyticspb "api-gateway/pkg/pb/analyticsservice"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AnalyticsHandler serves the dashboards. The scope (whole platform vs one store) is derived from the token here —
// query parameters can never widen it — and analytics-service enforces it again.
type AnalyticsHandler struct {
	Clients *client.ClientManager
	Auth    *authclient.AuthClient
	Logger  *zap.Logger
}

// NewAnalyticsHandler creates an AnalyticsHandler.
func NewAnalyticsHandler(cm *client.ClientManager, auth *authclient.AuthClient, logger *zap.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{Clients: cm, Auth: auth, Logger: logger}
}

// parseWhen accepts RFC 3339 or a plain date (YYYY-MM-DD) read in the platform timezone Asia/Ho_Chi_Minh.
// A plain `to` date is inclusive: it means the end of that day.
func parseWhen(s string, isTo bool) (string, bool) {
	if s == "" {
		return "", true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), true
	}
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return "", false
	}
	if isTo {
		t = t.AddDate(0, 0, 1)
	}
	return t.UTC().Format(time.RFC3339), true
}

// report builds the handler of one report. admin selects the platform scope (admin routes) or the caller's store.
func (h *AnalyticsHandler) report(name string, admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := &analyticspb.Scope{Role: "admin"}
		if !admin {
			v, ok := requireSellerStore(c, h.Auth)
			if !ok {
				return
			}
			scope = &analyticspb.Scope{Role: "seller", StoreId: v.StoreID}
		}
		from, ok1 := parseWhen(c.Query("from"), false)
		to, ok2 := parseWhen(c.Query("to"), true)
		if !ok1 || !ok2 {
			badRequest(c, "from/to must be YYYY-MM-DD or RFC 3339")
			return
		}
		limit, _ := strconv.Atoi(c.Query("limit"))
		ac, err := h.Clients.Analytics()
		if err != nil {
			respondError(c, h.Logger, "AnalyticsHandler: client", err)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		res, err := ac.Query(ctx, &analyticspb.QueryRequest{
			Report: name, Scope: scope, From: from, To: to, Granularity: c.Query("granularity"),
			Sort: c.Query("sort"), Limit: int32(limit),
		})
		if err != nil {
			respondError(c, h.Logger, "AnalyticsHandler: "+name, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"as_of": res.AsOf, "source": res.Source, "cached": res.Cached,
			"timezone": "Asia/Ho_Chi_Minh", "data": json.RawMessage(res.DataJson),
		})
	}
}

// Seller returns the handler of a store-scoped report (path segment with dashes, e.g. "top-products").
func (h *AnalyticsHandler) Seller(report string) gin.HandlerFunc {
	return h.report(strings.ReplaceAll(report, "-", "_"), false)
}

// Admin returns the handler of a platform-wide report.
func (h *AnalyticsHandler) Admin(report string) gin.HandlerFunc {
	return h.report(strings.ReplaceAll(report, "-", "_"), true)
}
