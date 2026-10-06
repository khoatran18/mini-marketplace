package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"analytics-service/internal/ch"
)

// Scope limits what a report may see. Platform = every store (admin); otherwise only StoreID's data. The gateway
// derives it from the JWT; analytics-service refuses a store scope without a store id.
type Scope struct {
	Platform bool
	StoreID  uint64
}

// Validate checks the scope is usable.
func (s Scope) Validate() error {
	if !s.Platform && s.StoreID == 0 {
		return errors.New("store scope needs a store id")
	}
	return nil
}

// Range is a half-open time interval [From, To).
type Range struct{ From, To time.Time }

// MaxRange is the longest interval a report may cover.
const MaxRange = 400 * 24 * time.Hour

// Validate checks order and length.
func (r Range) Validate() error {
	if !r.To.After(r.From) {
		return errors.New("`to` must be after `from`")
	}
	if r.To.Sub(r.From) > MaxRange {
		return errors.New("range is longer than 400 days")
	}
	return nil
}

// Loc is the platform timezone (Asia/Ho_Chi_Minh, UTC+7, no DST).
func Loc() *time.Location {
	loc, err := time.LoadLocation(Timezone)
	if err != nil {
		return time.FixedZone("ICT", 7*3600)
	}
	return loc
}

// Queries runs the report SQL against ClickHouse. All SQL is static text; user input travels as typed parameters.
type Queries struct {
	DB  *ch.Client
	Now func() time.Time
}

func (q *Queries) now() time.Time {
	if q.Now != nil {
		return q.Now()
	}
	return time.Now()
}

// money converts minor units (1/100 VND) to VND.
func money(minor int64) float64 { return float64(minor) / 100 }

// ---- shared SQL fragments ----------------------------------------------------------------------

// ordersCTE aggregates fact_order_status to one row per order. %s is the optional store filter.
// rec_at = the moment revenue is recognised: PAID for online methods, DELIVERED for COD.
const ordersCTE = `orders AS (
  SELECT order_id,
    any(store_id) AS sid, any(payment_method) AS pm, any(subtotal_minor) AS subtotal, any(total_minor) AS total,
    min(at) AS created_at,
    minIf(at, status = 'PAID') AS paid_at, minIf(at, status = 'DELIVERED') AS delivered_at,
    countIf(status = 'PAID') > 0 AS has_paid, countIf(status = 'DELIVERED') > 0 AS has_delivered,
    countIf(status = 'CANCELED') > 0 AS canceled, countIf(status = 'EXPIRED') > 0 AS expired,
    countIf(status = 'FAILED') > 0 AS failed, countIf(status = 'REFUNDED') > 0 AS refunded,
    argMax(status, at) AS cur,
    if(pm = 'COD', has_delivered, has_paid) AS recognized,
    if(pm = 'COD', delivered_at, paid_at) AS rec_at
  FROM fact_order_status FINAL
  %s
  GROUP BY order_id
)`

const (
	inRec     = "recognized AND rec_at >= {from:DateTime64(3,'UTC')} AND rec_at < {to:DateTime64(3,'UTC')}"
	inCreated = "created_at >= {from:DateTime64(3,'UTC')} AND created_at < {to:DateTime64(3,'UTC')}"
)

func (s Scope) orderFilter() string {
	if s.Platform {
		return ""
	}
	return "WHERE store_id = {store:UInt64}"
}

func (s Scope) params(r Range) map[string]string {
	p := map[string]string{"from": Time(r.From), "to": Time(r.To)}
	if !s.Platform {
		p["store"] = fmt.Sprint(s.StoreID)
	}
	return p
}

// productFilter restricts events on products to the scope's store ("" for the platform).
func (s Scope) productFilter() string {
	if s.Platform {
		return ""
	}
	return "AND product_id IN (SELECT product_id FROM dim_products FINAL WHERE store_id = {store:UInt64})"
}

func (s Scope) refundFilter() string {
	if s.Platform {
		return ""
	}
	return "AND order_id IN (SELECT order_id FROM fact_order_status WHERE store_id = {store:UInt64})"
}

// ---- summary -------------------------------------------------------------------------------------

// SummaryData is the headline numbers of one period. Money in VND.
type SummaryData struct {
	Revenue          float64 `json:"revenue"`     // recognised revenue (goods, before refunds)
	Refunds          float64 `json:"refunds"`     // refunds paid in the period
	NetRevenue       float64 `json:"net_revenue"` // Revenue - Refunds
	GMV              float64 `json:"gmv"`         // goods value of placed orders (excl. expired/failed/canceled-unpaid)
	OrdersPlaced     int64   `json:"orders_placed"`
	OrdersRecognized int64   `json:"orders_recognized"`
	AOV              float64 `json:"aov"` // Revenue / OrdersRecognized
	Canceled         int64   `json:"canceled"`
	Expired          int64   `json:"expired"`
	Refunded         int64   `json:"refunded"`
	CancelRate       float64 `json:"cancel_rate"` // (canceled+expired) / orders_placed
}

// Summary returns the period and the previous period of the same length.
func (q *Queries) Summary(ctx context.Context, sc Scope, r Range) (map[string]any, error) {
	cur, err := q.summaryOne(ctx, sc, r)
	if err != nil {
		return nil, err
	}
	d := r.To.Sub(r.From)
	prev, err := q.summaryOne(ctx, sc, Range{From: r.From.Add(-d), To: r.From})
	if err != nil {
		return nil, err
	}
	return map[string]any{"current": cur, "previous": prev, "from": r.From.UTC(), "to": r.To.UTC()}, nil
}

func (q *Queries) summaryOne(ctx context.Context, sc Scope, r Range) (SummaryData, error) {
	sql := "WITH " + fmt.Sprintf(ordersCTE, sc.orderFilter()) + `
SELECT
  sumIf(subtotal, ` + inRec + `) AS revenue,
  countIf(` + inRec + `) AS rec_orders,
  countIf(` + inCreated + `) AS placed,
  sumIf(subtotal, ` + inCreated + ` AND NOT ((canceled AND NOT has_paid) OR expired OR failed)) AS gmv,
  countIf(` + inCreated + ` AND canceled) AS n_canceled,
  countIf(` + inCreated + ` AND expired) AS n_expired,
  countIf(` + inCreated + ` AND refunded) AS n_refunded
FROM orders`
	rows, err := q.DB.Query(ctx, sql, sc.params(r))
	if err != nil {
		return SummaryData{}, err
	}
	var out SummaryData
	if len(rows) > 0 {
		row := rows[0]
		out.Revenue = money(row.Int("revenue"))
		out.OrdersRecognized = row.Int("rec_orders")
		out.OrdersPlaced = row.Int("placed")
		out.GMV = money(row.Int("gmv"))
		out.Canceled, out.Expired, out.Refunded = row.Int("n_canceled"), row.Int("n_expired"), row.Int("n_refunded")
	}
	rr, err := q.DB.Query(ctx, `SELECT sum(amount_minor) AS refunds FROM fact_refunds FINAL
WHERE at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} `+sc.refundFilter(), sc.params(r))
	if err != nil {
		return SummaryData{}, err
	}
	if len(rr) > 0 {
		out.Refunds = money(rr[0].Int("refunds"))
	}
	out.NetRevenue = out.Revenue - out.Refunds
	if out.OrdersRecognized > 0 {
		out.AOV = out.Revenue / float64(out.OrdersRecognized)
	}
	if out.OrdersPlaced > 0 {
		out.CancelRate = float64(out.Canceled+out.Expired) / float64(out.OrdersPlaced)
	}
	return out, nil
}

// ---- timeseries ----------------------------------------------------------------------------------

// Granularities accepted by Timeseries / Traffic.
var granularity = map[string]string{"hour": "HOUR", "day": "DAY", "week": "WEEK", "month": "MONTH"}

// ValidGranularity reports whether g is supported.
func ValidGranularity(g string) bool { _, ok := granularity[g]; return ok }

func bucketExpr(col, g string) string {
	return fmt.Sprintf("formatDateTime(toStartOfInterval(toTimeZone(%s, '%s'), INTERVAL 1 %s), '%%Y-%%m-%%d %%H:%%i:%%S')", col, Timezone, granularity[g])
}

// Point is one bucket of the revenue time series (bucket label is local time, Asia/Ho_Chi_Minh).
type Point struct {
	Bucket  string  `json:"bucket"`
	Revenue float64 `json:"revenue"`
	Refunds float64 `json:"refunds"`
	Orders  int64   `json:"orders"` // recognised
	Placed  int64   `json:"placed"`
}

// Timeseries returns revenue / orders / refunds per bucket; empty buckets are filled with zeros.
func (q *Queries) Timeseries(ctx context.Context, sc Scope, r Range, g string) ([]Point, error) {
	if !ValidGranularity(g) {
		return nil, errors.New("granularity must be hour|day|week|month")
	}
	sql := "WITH " + fmt.Sprintf(ordersCTE, sc.orderFilter()) + `
SELECT bucket, sum(revenue) AS revenue, sum(refunds) AS refunds, sum(orders) AS orders, sum(placed) AS placed FROM (
  SELECT ` + bucketExpr("rec_at", g) + ` AS bucket, toInt64(subtotal) AS revenue, toInt64(0) AS refunds, 1 AS orders, 0 AS placed
    FROM orders WHERE ` + inRec + `
  UNION ALL
  SELECT ` + bucketExpr("created_at", g) + `, toInt64(0), toInt64(0), 0, 1 FROM orders WHERE ` + inCreated + `
  UNION ALL
  SELECT ` + bucketExpr("at", g) + `, toInt64(0), amount_minor, 0, 0 FROM fact_refunds FINAL
    WHERE at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} ` + sc.refundFilter() + `
) GROUP BY bucket ORDER BY bucket`
	rows, err := q.DB.Query(ctx, sql, sc.params(r))
	if err != nil {
		return nil, err
	}
	byBucket := map[string]Point{}
	for _, row := range rows {
		b := row.Str("bucket")
		byBucket[b] = Point{Bucket: b, Revenue: money(row.Int("revenue")), Refunds: money(row.Int("refunds")),
			Orders: row.Int("orders"), Placed: row.Int("placed")}
	}
	var out []Point
	for _, b := range buckets(r, g) {
		if p, ok := byBucket[b]; ok {
			out = append(out, p)
		} else {
			out = append(out, Point{Bucket: b})
		}
	}
	return out, nil
}

// buckets lists the bucket labels covering r in local time (week = Monday start, month = 1st).
func buckets(r Range, g string) []string {
	loc := Loc()
	t := r.From.In(loc)
	switch g {
	case "hour":
		t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
	case "day":
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	case "week":
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		t = t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
	case "month":
		t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	}
	var out []string
	for t.Before(r.To) && len(out) < 20000 {
		out = append(out, t.Format("2006-01-02 15:04:05"))
		switch g {
		case "hour":
			t = t.Add(time.Hour)
		case "day":
			t = t.AddDate(0, 0, 1)
		case "week":
			t = t.AddDate(0, 0, 7)
		case "month":
			t = t.AddDate(0, 1, 0)
		}
	}
	return out
}

// ---- top products --------------------------------------------------------------------------------

// ProductStat is one row of the product ranking.
type ProductStat struct {
	ProductID   uint64  `json:"product_id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Units       int64   `json:"units"`
	Revenue     float64 `json:"revenue"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	Views       int64   `json:"views"`
	Carts       int64   `json:"carts"`
	Conversion  float64 `json:"conversion"` // units sold / product views
}

// Sort orders accepted by TopProducts.
var productSorts = map[string]string{
	"revenue":       "revenue DESC",
	"units":         "units DESC",
	"views":         "views DESC",
	"conversion":    "conversion DESC, views DESC",
	"viewed_unsold": "views DESC", // see the filter below: many views, no sale
}

// ValidProductSort reports whether s is a supported ranking.
func ValidProductSort(s string) bool { _, ok := productSorts[s]; return ok }

// TopProducts ranks products by sort (revenue|units|views|conversion|viewed_unsold).
func (q *Queries) TopProducts(ctx context.Context, sc Scope, r Range, sort string, limit int) ([]ProductStat, error) {
	order, ok := productSorts[sort]
	if !ok {
		return nil, errors.New("unknown sort")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	having := ""
	if sort == "viewed_unsold" {
		having = "WHERE units = 0 AND views > 0"
	}
	sql := "WITH " + fmt.Sprintf(ordersCTE, sc.orderFilter()) + `
SELECT product_id, dp.name AS name, dp.status AS status, units, revenue, impressions, clicks, views, carts,
       if(views > 0, units / views, 0) AS conversion
FROM (
  SELECT product_id, sum(units) AS units, sum(revenue) AS revenue, sum(impressions) AS impressions,
         sum(clicks) AS clicks, sum(views) AS views, sum(carts) AS carts
  FROM (
    SELECT product_id, sum(qty) AS units, sum(qty * unit_price_minor) AS revenue, 0 AS impressions, 0 AS clicks, 0 AS views, 0 AS carts
    FROM fact_order_items FINAL
    WHERE order_id IN (SELECT order_id FROM orders WHERE ` + inRec + `)
    GROUP BY product_id
    UNION ALL
    SELECT product_id, 0, 0, countIf(event_type = 'impression'), countIf(event_type = 'product_click'),
           countIf(event_type = 'product_view'), countIf(event_type = 'add_to_cart')
    FROM events_raw FINAL
    WHERE ts >= {from:DateTime64(3,'UTC')} AND ts < {to:DateTime64(3,'UTC')} AND product_id > 0 ` + sc.productFilter() + `
    GROUP BY product_id
  ) GROUP BY product_id
) AS m
LEFT JOIN (SELECT product_id, name, status FROM dim_products FINAL) AS dp USING (product_id)
` + having + `
ORDER BY ` + order + `, product_id
LIMIT {limit:UInt32}`
	p := sc.params(r)
	p["limit"] = fmt.Sprint(limit)
	rows, err := q.DB.Query(ctx, sql, p)
	if err != nil {
		return nil, err
	}
	out := make([]ProductStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProductStat{
			ProductID: uint64(row.Int("product_id")), Name: row.Str("name"), Status: row.Str("status"),
			Units: row.Int("units"), Revenue: money(row.Int("revenue")), Impressions: row.Int("impressions"),
			Clicks: row.Int("clicks"), Views: row.Int("views"), Carts: row.Int("carts"), Conversion: row.Float("conversion"),
		})
	}
	return out, nil
}

// ---- funnel --------------------------------------------------------------------------------------

// Step is one funnel stage.
type Step struct {
	Step  string `json:"step"`
	Count *int64 `json:"count"` // nil = not available for this scope
}

// Funnel counts distinct sessions per stage (view → cart → checkout) and recognised orders (paid). A store scope
// counts only views/carts of its own products; the checkout stage is not attributable to one store (nil).
func (q *Queries) Funnel(ctx context.Context, sc Scope, r Range) (map[string]any, error) {
	base := `SELECT uniqIf(session_id, event_type = 'product_view') AS views,
  uniqIf(session_id, event_type = 'add_to_cart') AS carts,
  uniqIf(session_id, event_type = 'checkout_start') AS checkouts
FROM events_raw FINAL
WHERE ts >= {from:DateTime64(3,'UTC')} AND ts < {to:DateTime64(3,'UTC')} AND session_id != ''`
	if !sc.Platform {
		base = `SELECT uniqIf(session_id, event_type = 'product_view') AS views,
  uniqIf(session_id, event_type = 'add_to_cart') AS carts, 0 AS checkouts
FROM events_raw FINAL
WHERE ts >= {from:DateTime64(3,'UTC')} AND ts < {to:DateTime64(3,'UTC')} AND session_id != '' ` + sc.productFilter()
	}
	rows, err := q.DB.Query(ctx, base, sc.params(r))
	if err != nil {
		return nil, err
	}
	var views, carts, checkouts int64
	if len(rows) > 0 {
		views, carts, checkouts = rows[0].Int("views"), rows[0].Int("carts"), rows[0].Int("checkouts")
	}
	paidRows, err := q.DB.Query(ctx, "WITH "+fmt.Sprintf(ordersCTE, sc.orderFilter())+" SELECT count() AS n FROM orders WHERE "+inRec, sc.params(r))
	if err != nil {
		return nil, err
	}
	var paid int64
	if len(paidRows) > 0 {
		paid = paidRows[0].Int("n")
	}
	ptr := func(v int64) *int64 { return &v }
	steps := []Step{{"view", ptr(views)}, {"cart", ptr(carts)}, {"checkout", nil}, {"paid", ptr(paid)}}
	if sc.Platform {
		steps[2].Count = ptr(checkouts)
	}
	out := map[string]any{"steps": steps, "unit": "sessions (paid = recognised orders)"}
	if sc.Platform {
		dev, err := q.DB.Query(ctx, `SELECT device_type, uniqIf(session_id, event_type = 'product_view') AS views,
  uniqIf(session_id, event_type = 'add_to_cart') AS carts, uniqIf(session_id, event_type = 'checkout_start') AS checkouts
FROM events_raw FINAL WHERE ts >= {from:DateTime64(3,'UTC')} AND ts < {to:DateTime64(3,'UTC')} AND session_id != ''
GROUP BY device_type ORDER BY views DESC`, sc.params(r))
		if err != nil {
			return nil, err
		}
		var by []map[string]any
		for _, d := range dev {
			by = append(by, map[string]any{"device_type": d.Str("device_type"), "views": d.Int("views"), "carts": d.Int("carts"), "checkouts": d.Int("checkouts")})
		}
		out["by_device"] = by
	}
	return out, nil
}
