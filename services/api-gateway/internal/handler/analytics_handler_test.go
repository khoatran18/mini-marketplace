package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/clientname"
	analyticspb "api-gateway/pkg/pb/analyticsservice"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAnalytics struct {
	analyticspb.AnalyticsServiceClient
	ingest *analyticspb.IngestEventsRequest
	query  *analyticspb.QueryRequest
	err    error
}

func (f *fakeAnalytics) IngestEvents(_ context.Context, in *analyticspb.IngestEventsRequest, _ ...grpc.CallOption) (*analyticspb.IngestEventsResponse, error) {
	f.ingest = in
	if f.err != nil {
		return nil, f.err
	}
	return &analyticspb.IngestEventsResponse{Accepted: int32(len(in.Events))}, nil
}
func (f *fakeAnalytics) Query(_ context.Context, in *analyticspb.QueryRequest, _ ...grpc.CallOption) (*analyticspb.QueryResponse, error) {
	f.query = in
	if f.err != nil {
		return nil, f.err
	}
	return &analyticspb.QueryResponse{AsOf: "2026-10-06T00:00:00Z", Source: "clickhouse", DataJson: `{"hello":"world"}`}, nil
}

func analyticsRouter(role string, storeID uint64) (*gin.Engine, *fakeAnalytics) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	fake := &fakeAnalytics{}
	cm := client.NewClientManager()
	cm.Clients[clientname.AnalyticsClientName] = &client.ServiceClient{Client: fake}
	auth := authclient.NewAuthClient(&fakeAuthClient{storeID: storeID}, nil, logger)
	ah := NewAnalyticsHandler(cm, auth, logger)
	th := NewTrackingHandler(cm, auth, "secret-1", logger)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if role != "" {
			c.Set("userID", uint64(callerID))
			c.Set("userRole", role)
			c.Set("username", "u")
		}
	})
	r.POST("/events", th.Ingest)
	r.POST("/events/identify", th.Identify)
	r.GET("/seller/analytics/summary", ah.Seller("summary"))
	r.GET("/seller/analytics/top-products", ah.Seller("top-products"))
	r.GET("/admin/analytics/traffic", ah.Admin("traffic"))
	r.GET("/admin/analytics/data-health", ah.Admin("data-health"))
	return r, fake
}

func TestSellerReportScopeComesFromTheToken(t *testing.T) {
	r, fake := analyticsRouter("seller_admin", 77)
	// the client tries to read another store and to widen the scope: both parameters are ignored
	w := do(r, "GET", "/seller/analytics/top-products?store_id=999&scope=platform&role=admin&sort=units&limit=5&from=2026-10-01&to=2026-10-31&granularity=week", "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	q := fake.query
	if q.Report != "top_products" || q.Scope.Role != "seller" || q.Scope.StoreId != 77 || q.Sort != "units" || q.Limit != 5 || q.Granularity != "week" {
		t.Fatalf("%+v", q)
	}
	// plain dates are read in Asia/Ho_Chi_Minh, and `to` is inclusive
	if q.From != "2026-09-30T17:00:00Z" || q.To != "2026-10-31T17:00:00Z" {
		t.Fatalf("from/to: %s %s", q.From, q.To)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["source"] != "clickhouse" || out["timezone"] != "Asia/Ho_Chi_Minh" || out["data"].(map[string]any)["hello"] != "world" {
		t.Fatalf("%v", out)
	}
}

func TestSellerWithoutStoreIsRefused(t *testing.T) {
	r, fake := analyticsRouter("seller_admin", 0)
	if w := do(r, "GET", "/seller/analytics/summary", ""); w.Code == http.StatusOK || fake.query != nil {
		t.Fatalf("a seller without a store must not reach analytics: %d", w.Code)
	}
	r, fake = analyticsRouter("buyer", 0)
	if w := do(r, "GET", "/seller/analytics/summary", ""); w.Code != http.StatusForbidden || fake.query != nil {
		t.Fatalf("buyers: %d", w.Code)
	}
}

func TestAdminReportsUsePlatformScope(t *testing.T) {
	r, fake := analyticsRouter("admin", 0)
	if w := do(r, "GET", "/admin/analytics/data-health", ""); w.Code != 200 || fake.query.Report != "data_health" || fake.query.Scope.Role != "admin" || fake.query.Scope.StoreId != 0 {
		t.Fatalf("%d %+v", w.Code, fake.query)
	}
	if w := do(r, "GET", "/admin/analytics/traffic?from=bad", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad date: %d", w.Code)
	}
}

func TestAnalyticsErrorsMapToHTTP(t *testing.T) {
	r, fake := analyticsRouter("admin", 0)
	fake.err = status.Error(codes.InvalidArgument, "range is longer than 400 days")
	if w := do(r, "GET", "/admin/analytics/traffic", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("%d", w.Code)
	}
	fake.err = status.Error(codes.Unavailable, "analytics store error")
	if w := do(r, "GET", "/admin/analytics/traffic", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d", w.Code)
	}
}

func TestTrackingAttachesIdentityOnTheServer(t *testing.T) {
	r, fake := analyticsRouter("seller_admin", 55)
	body := `{"events":[{"event_id":"e1","event_type":"product_view","anonymous_id":"a_1","session_id":"s_1","surface":"pdp",
	  "page":{"path":"/products/3","referrer":"https://x.com/"},"item":{"product_id":3,"position":2},"props":{"ms":100},
	  "device":{"type":"mobile","os":"iOS"},"consent":{"analytics":true},"app_version":"web-1",
	  "user_id":1,"role":"admin","store_id":999,"ip_hash":"forged"}]}`
	w := do(r, "POST", "/events", body, "User-Agent", "Mozilla/5.0 Chrome/120", "CF-IPCountry", "vn")
	if w.Code != http.StatusAccepted {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	e := fake.ingest.Events[0]
	if e.UserId != callerID || e.Role != "seller_admin" || e.StoreId != 55 {
		t.Fatalf("identity must come from the token, not the body: %+v", e)
	}
	if e.IpHash == "" || e.IpHash == "forged" || e.UaFamily != "chrome" || e.Country != "VN" {
		t.Fatalf("%+v", e)
	}
	if e.ProductId != 3 || e.Position != 2 || e.Path != "/products/3" || e.PropsJson != `{"ms":100}` || !e.AnalyticsConsent || e.DeviceType != "mobile" {
		t.Fatalf("%+v", e)
	}
}

func TestTrackingAnonymousAndLimits(t *testing.T) {
	r, fake := analyticsRouter("", 0)
	if w := do(r, "POST", "/events", `{"events":[{"event_type":"page_view","consent":{"analytics":true}}]}`); w.Code != http.StatusAccepted {
		t.Fatal(w.Code)
	}
	if e := fake.ingest.Events[0]; e.UserId != 0 || e.Role != "" || e.StoreId != 0 {
		t.Fatalf("anonymous: %+v", e)
	}
	events := strings.Repeat(`{"event_type":"page_view"},`, 51)
	if w := do(r, "POST", "/events", `{"events":[`+strings.TrimSuffix(events, ",")+`]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("51 events: %d", w.Code)
	}
	for _, b := range []string{`{}`, `{"events":[]}`, `nope`} {
		if w := do(r, "POST", "/events", b); w.Code != http.StatusBadRequest {
			t.Fatalf("%q: %d", b, w.Code)
		}
	}
	big := `{"events":[{"event_type":"page_view","props":{"x":"` + strings.Repeat("a", 70000) + `"}}]}`
	if w := do(r, "POST", "/events", big); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("64KB limit: %d", w.Code)
	}
}

func TestTrackingIsBestEffort(t *testing.T) {
	r, fake := analyticsRouter("", 0)
	fake.err = status.Error(codes.Unavailable, "down")
	w := do(r, "POST", "/events", `{"events":[{"event_type":"page_view"},{"event_type":"page_view"}]}`)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusAccepted || out["dropped"] != float64(2) {
		t.Fatalf("an analytics outage must not break the page: %d %v", w.Code, out)
	}
}

func TestIdentify(t *testing.T) {
	r, fake := analyticsRouter("buyer", 0)
	if w := do(r, "POST", "/events/identify", `{"anonymous_id":"a_9"}`); w.Code != http.StatusAccepted {
		t.Fatal(w.Code)
	}
	if e := fake.ingest.Events[0]; e.EventType != "identify" || e.UserId != callerID || e.AnonymousId != "a_9" {
		t.Fatalf("%+v", e)
	}
	if w := do(r, "POST", "/events/identify", `{}`); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
	anon, _ := analyticsRouter("", 0)
	if w := do(anon, "POST", "/events/identify", `{"anonymous_id":"a"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("identify needs a user: %d", w.Code)
	}
}

func TestIPHashRotatesDaily(t *testing.T) {
	_, _ = analyticsRouter("", 0)
	h := NewTrackingHandler(client.NewClientManager(), nil, "k", zap.NewNop())
	a := h.ipHash("1.2.3.4")
	if a != h.ipHash("1.2.3.4") || a == h.ipHash("1.2.3.5") || strings.Contains(a, "1.2.3.4") {
		t.Fatal("hash must be stable per ip within a day and not contain the ip")
	}
	h.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	if h.ipHash("1.2.3.4") == a {
		t.Fatal("the hash must change the next day (unlinkable across days)")
	}
}
