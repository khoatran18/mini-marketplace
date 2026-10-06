package ingest

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"

	"analytics-service/internal/store"
)

// ClientEventTypes are the event types a browser may send (docs/platform/03-data-model.md §2.2). Everything
// else — notably order_placed / payment_succeeded, which come from the business outbox — is rejected.
var ClientEventTypes = map[string]bool{
	"page_view": true, "impression": true, "product_click": true, "product_view": true, "dwell": true,
	"search": true, "search_click": true, "filter_apply": true, "sort_change": true,
	"add_to_cart": true, "remove_from_cart": true, "cart_update": true,
	"wishlist_add": true, "wishlist_remove": true,
	"checkout_start": true, "checkout_submit": true, "payment_page_view": true, "error_client": true,
}

// TypeIdentify is written only by the gateway's POST /events/identify (links anonymous_id to the logged-in user).
const TypeIdentify = "identify"

// anonymousTypes are the only types kept when the visitor refused analytics consent (stored without identifiers).
var anonymousTypes = map[string]bool{"page_view": true, "error_client": true}

var surfaces = map[string]bool{
	"home_trending": true, "home_new": true, "category_page": true, "search_results": true, "pdp": true,
	"cart": true, "checkout": true, "orders": true, "seller_console": true, "admin_console": true,
}

var devices = map[string]bool{"desktop": true, "mobile": true, "tablet": true}

// Limits.
const (
	maxID    = 64
	maxPath  = 512
	maxProps = 2048
	maxSkew  = 24 * time.Hour
)

// Raw is one tracking event as handed over by the gateway: client fields + the fields the gateway attached from
// the JWT / request (UserID, Role, StoreID, IPHash, UAFamily, Country). Client-supplied values for the latter are
// never copied by the gateway.
type Raw struct {
	EventID, EventType, TsClient, AnonymousID, SessionID string
	Surface, Path, Referrer, PropsJSON                   string
	ProductID                                            uint64
	Position                                             uint32
	DeviceType, OS, AppVersion                           string
	AnalyticsConsent                                     bool
	UserID                                               uint64
	Role                                                 string
	StoreID                                              uint64
	IPHash, UAFamily, Country                            string
}

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe = regexp.MustCompile(`\+?\d[\d .\-]{7,}\d`)
)

// ScrubPII masks e-mail addresses and phone-like numbers in free text (search queries).
func ScrubPII(s string) string {
	s = emailRe.ReplaceAllString(s, "[email]")
	return phoneRe.ReplaceAllString(s, "[phone]")
}

// Normalize validates and sanitises one event. A non-empty reason means "rejected".
func Normalize(r Raw, now time.Time) (store.EventRow, string) {
	t := strings.ToLower(strings.TrimSpace(r.EventType))
	if t == TypeIdentify {
		if r.UserID == 0 || r.AnonymousID == "" {
			return store.EventRow{}, "identify_needs_user_and_anonymous_id"
		}
	} else if !ClientEventTypes[t] {
		return store.EventRow{}, "unknown_or_server_only_type"
	}
	if len(r.EventID) > maxID || len(r.AnonymousID) > maxID || len(r.SessionID) > maxID {
		return store.EventRow{}, "id_too_long"
	}
	if r.EventID == "" {
		r.EventID = newID()
	}
	if len(r.PropsJSON) > maxProps {
		return store.EventRow{}, "props_too_large"
	}
	props := "{}"
	if r.PropsJSON != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(r.PropsJSON), &m); err != nil {
			return store.EventRow{}, "props_not_json_object"
		}
		scrubProps(m)
		b, _ := json.Marshal(m)
		props = string(b)
	}

	ts := now
	if r.TsClient != "" {
		if c, err := time.Parse(time.RFC3339Nano, r.TsClient); err == nil && c.After(now.Add(-maxSkew)) && c.Before(now.Add(maxSkew)) {
			ts = c // trust the client clock only within ±24 h (order of events inside a session)
		}
	}
	row := store.EventRow{
		EventID: r.EventID, EventType: t, Ts: store.Time(ts), TsServer: store.Time(now),
		AnonymousID: r.AnonymousID, SessionID: r.SessionID, UserID: r.UserID, Role: r.Role, StoreID: r.StoreID,
		Path: cleanPath(r.Path), Referrer: cleanReferrer(r.Referrer), ProductID: r.ProductID,
		Position: uint16(min(r.Position, 65535)), Props: props, OS: trunc(r.OS, 32), AppVersion: trunc(r.AppVersion, 32),
		IPHash: trunc(r.IPHash, 64), UAFamily: trunc(r.UAFamily, 32), Country: trunc(r.Country, 2),
	}
	if surfaces[r.Surface] {
		row.Surface = r.Surface
	}
	if devices[r.DeviceType] {
		row.DeviceType = r.DeviceType
	} else {
		row.DeviceType = "other"
	}
	if r.AnalyticsConsent {
		row.AnalyticsConsent = 1
	} else if t != TypeIdentify {
		// no consent: keep only anonymous page views / errors, drop every identifier
		if !anonymousTypes[t] {
			return store.EventRow{}, "no_consent"
		}
		row.AnonymousID, row.SessionID, row.UserID, row.IPHash, row.ProductID = "", "", 0, "", 0
		row.Props = "{}"
	}
	return row, ""
}

func scrubProps(m map[string]any) {
	for k, v := range m {
		switch x := v.(type) {
		case string:
			m[k] = ScrubPII(trunc(x, 200))
		case map[string]any:
			scrubProps(x)
		}
	}
}

func cleanPath(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return trunc(p, maxPath)
}

// cleanReferrer keeps host+path only (query strings may carry tokens or e-mail addresses).
func cleanReferrer(ref string) string {
	if ref == "" {
		return ""
	}
	if u, err := url.Parse(ref); err == nil && u.Host != "" {
		return trunc(u.Host+u.Path, maxPath)
	}
	return cleanPath(ref)
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
