package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	analyticspb "api-gateway/pkg/pb/analyticsservice"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Limits of POST /events (docs/platform/03-data-model.md §2.4).
const (
	maxEventsPerBatch = 50
	maxEventsBody     = 64 << 10
)

// TrackingHandler receives clickstream events from the browser. It is a thin, public, rate-limited door:
// the gateway attaches the identity (user, role, store, hashed IP) itself — anything the client claims about
// those is ignored — and analytics-service validates and stores the events. Tracking is best effort: a failure
// never breaks the shop.
type TrackingHandler struct {
	Clients *client.ClientManager
	Auth    *authclient.AuthClient
	Logger  *zap.Logger
	secret  []byte
	now     func() time.Time
}

// NewTrackingHandler creates a TrackingHandler. ipSecret keys the IP hash; empty = a random per-process key
// (hashes then differ between replicas/restarts, which only weakens the unique-visitor estimate, not privacy).
func NewTrackingHandler(cm *client.ClientManager, auth *authclient.AuthClient, ipSecret string, logger *zap.Logger) *TrackingHandler {
	key := []byte(ipSecret)
	if len(key) == 0 {
		key = make([]byte, 32)
		_, _ = rand.Read(key)
	}
	return &TrackingHandler{Clients: cm, Auth: auth, Logger: logger, secret: key, now: time.Now}
}

// trackEvent is the browser envelope. There are deliberately NO user_id / role / store_id / ip fields.
type trackEvent struct {
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	TsClient    string `json:"ts_client"`
	AnonymousID string `json:"anonymous_id"`
	SessionID   string `json:"session_id"`
	Surface     string `json:"surface"`
	Page        struct {
		Path     string `json:"path"`
		Referrer string `json:"referrer"`
	} `json:"page"`
	Item struct {
		ProductID uint64 `json:"product_id"`
		Position  uint32 `json:"position"`
	} `json:"item"`
	Props  json.RawMessage `json:"props"`
	Device struct {
		Type string `json:"type"`
		OS   string `json:"os"`
	} `json:"device"`
	Consent struct {
		Analytics bool `json:"analytics"`
	} `json:"consent"`
	AppVersion string `json:"app_version"`
}

func (h *TrackingHandler) analytics() (analyticspb.AnalyticsServiceClient, error) {
	return h.Clients.Analytics()
}

// ipHash is HMAC(secret, UTC date | ip): stable within a day (unique visitors), unlinkable across days.
func (h *TrackingHandler) ipHash(ip string) string {
	m := hmac.New(sha256.New, h.secret)
	m.Write([]byte(h.now().UTC().Format("2006-01-02") + "|" + ip))
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// uaFamily is a coarse browser family (no version, no full user agent is stored).
func uaFamily(ua string) string {
	l := strings.ToLower(ua)
	switch {
	case l == "":
		return ""
	case strings.Contains(l, "bot") || strings.Contains(l, "spider") || strings.Contains(l, "crawl"):
		return "bot"
	case strings.Contains(l, "edg/"):
		return "edge"
	case strings.Contains(l, "firefox"):
		return "firefox"
	case strings.Contains(l, "chrome") || strings.Contains(l, "crios"):
		return "chrome"
	case strings.Contains(l, "safari"):
		return "safari"
	}
	return "other"
}

func country(c *gin.Context) string {
	// set by a CDN / edge proxy when there is one; never taken from the body
	if v := strings.ToUpper(strings.TrimSpace(c.GetHeader("CF-IPCountry"))); len(v) == 2 {
		return v
	}
	return ""
}

// serverFields attaches the identity the gateway knows. Errors resolving the store are not fatal for tracking.
func (h *TrackingHandler) identity(c *gin.Context) (userID uint64, role string, storeID uint64) {
	if id, ok := currentUserID(c); ok {
		userID = id
		_, role, _ = currentUser(c)
		if (role == "seller_admin" || role == "seller_employee") && h.Auth != nil {
			if v, err := resolveViewer(c, h.Auth); err == nil {
				storeID = v.StoreID
			}
		}
	}
	return
}

// Ingest godoc
// @Summary Send clickstream events
// @Description Public (a token is optional and only used to attach the user). At most 50 events and 64 KB per call. Always answers 202 for well-formed bodies: events that fail validation are counted in `rejected`.
// @Tags tracking
// @Accept json
// @Produce json
// @Router /events [post]
func (h *TrackingHandler) Ingest(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxEventsBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "body too large (max 64 KB)"})
			return
		}
		badRequest(c, "cannot read body")
		return
	}
	var in struct {
		Events []trackEvent `json:"events"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		badRequest(c, "invalid JSON")
		return
	}
	if len(in.Events) == 0 {
		badRequest(c, "events must not be empty")
		return
	}
	if len(in.Events) > maxEventsPerBatch {
		badRequest(c, "at most 50 events per request")
		return
	}
	userID, role, storeID := h.identity(c)
	ipHash, ua, cc := h.ipHash(c.ClientIP()), uaFamily(c.GetHeader("User-Agent")), country(c)

	events := make([]*analyticspb.TrackEvent, 0, len(in.Events))
	for _, e := range in.Events {
		props := ""
		if len(e.Props) > 0 && string(e.Props) != "null" {
			props = string(e.Props)
		}
		events = append(events, &analyticspb.TrackEvent{
			EventId: e.EventID, EventType: e.EventType, TsClient: e.TsClient, AnonymousId: e.AnonymousID, SessionId: e.SessionID,
			Surface: e.Surface, Path: e.Page.Path, Referrer: e.Page.Referrer, ProductId: e.Item.ProductID, Position: e.Item.Position,
			PropsJson: props, DeviceType: e.Device.Type, Os: e.Device.OS, AppVersion: e.AppVersion, AnalyticsConsent: e.Consent.Analytics,
			// server-attached, never from the body
			UserId: userID, Role: role, StoreId: storeID, IpHash: ipHash, UaFamily: ua, Country: cc,
		})
	}
	if res, ok := h.send(c.Request.Context(), events); ok {
		c.JSON(http.StatusAccepted, gin.H{"accepted": res.Accepted, "rejected": res.Rejected, "reasons": res.Reasons})
		return
	}
	// analytics-service is down: tracking is best effort, the page must not notice
	c.JSON(http.StatusAccepted, gin.H{"accepted": 0, "rejected": 0, "dropped": len(events)})
}

func (h *TrackingHandler) send(ctx context.Context, events []*analyticspb.TrackEvent) (*analyticspb.IngestEventsResponse, bool) {
	ac, err := h.analytics()
	if err != nil {
		h.Logger.Warn("TrackingHandler: analytics client", zap.Error(err))
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	res, err := ac.IngestEvents(ctx, &analyticspb.IngestEventsRequest{Events: events})
	if err != nil {
		h.Logger.Warn("TrackingHandler: ingest failed", zap.Error(err))
		return nil, false
	}
	return res, true
}

// Identify godoc
// @Summary Link the anonymous visitor to the signed-in user
// @Description Requires a token. Stored as an `identify` event (user from the token, anonymous_id from the body); used to join pre-login behaviour with the account.
// @Tags tracking
// @Security BearerAuth
// @Router /events/identify [post]
func (h *TrackingHandler) Identify(c *gin.Context) {
	var in struct {
		AnonymousID string `json:"anonymous_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)).Decode(&in); err != nil || in.AnonymousID == "" {
		badRequest(c, "anonymous_id is required")
		return
	}
	userID, role, storeID := h.identity(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "sign in first"})
		return
	}
	ev := &analyticspb.TrackEvent{
		EventType: "identify", AnonymousId: in.AnonymousID, AnalyticsConsent: true,
		UserId: userID, Role: role, StoreId: storeID, IpHash: h.ipHash(c.ClientIP()), UaFamily: uaFamily(c.GetHeader("User-Agent")), Country: country(c),
	}
	if res, ok := h.send(c.Request.Context(), []*analyticspb.TrackEvent{ev}); ok {
		c.JSON(http.StatusAccepted, gin.H{"accepted": res.Accepted, "rejected": res.Rejected})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": 0, "dropped": 1})
}
