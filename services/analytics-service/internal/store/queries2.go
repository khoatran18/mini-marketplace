package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func rangeParams(r Range) map[string]string {
	return map[string]string{"from": Time(r.From), "to": Time(r.To)}
}

// topList runs a "name, n" query and returns [{key, count}].
func (q *Queries) topList(ctx context.Context, sql string, p map[string]string, keyCol string) ([]map[string]any, error) {
	rows, err := q.DB.Query(ctx, sql, p)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"key": r.Str(keyCol), "count": r.Int("n")})
	}
	return out, nil
}

// Traffic (platform only): page views, sessions, unique visitors, new vs returning, series, top lists.
func (q *Queries) Traffic(ctx context.Context, r Range, g string) (map[string]any, error) {
	if !ValidGranularity(g) {
		return nil, errors.New("granularity must be hour|day|week|month")
	}
	p := rangeParams(r)
	const win = "ts >= {from:DateTime64(3,'UTC')} AND ts < {to:DateTime64(3,'UTC')}"
	tot, err := q.DB.Query(ctx, `SELECT countIf(event_type = 'page_view') AS page_views,
  uniqIf(session_id, session_id != '') AS sessions, uniqIf(anonymous_id, anonymous_id != '') AS visitors,
  countIf(event_type = 'error_client') AS client_errors
FROM events_raw FINAL WHERE `+win+` AND event_type != 'identify'`, p)
	if err != nil {
		return nil, err
	}
	newV, err := q.DB.Query(ctx, `SELECT count() AS n FROM (
  SELECT anonymous_id, min(ts) AS first_seen FROM events_raw WHERE anonymous_id != '' GROUP BY anonymous_id
  HAVING first_seen >= {from:DateTime64(3,'UTC')} AND first_seen < {to:DateTime64(3,'UTC')})`, p)
	if err != nil {
		return nil, err
	}
	series, err := q.DB.Query(ctx, `SELECT `+bucketExpr("ts", g)+` AS bucket, countIf(event_type = 'page_view') AS page_views,
  uniqIf(session_id, session_id != '') AS sessions, uniqIf(anonymous_id, anonymous_id != '') AS visitors
FROM events_raw FINAL WHERE `+win+` AND event_type != 'identify' GROUP BY bucket ORDER BY bucket`, p)
	if err != nil {
		return nil, err
	}
	by := map[string]map[string]any{}
	for _, s := range series {
		by[s.Str("bucket")] = map[string]any{"bucket": s.Str("bucket"), "page_views": s.Int("page_views"), "sessions": s.Int("sessions"), "visitors": s.Int("visitors")}
	}
	var filled []map[string]any
	for _, b := range buckets(r, g) {
		if v, ok := by[b]; ok {
			filled = append(filled, v)
		} else {
			filled = append(filled, map[string]any{"bucket": b, "page_views": 0, "sessions": 0, "visitors": 0})
		}
	}
	out := map[string]any{"series": filled}
	if len(tot) > 0 {
		out["page_views"], out["sessions"], out["visitors"], out["client_errors"] =
			tot[0].Int("page_views"), tot[0].Int("sessions"), tot[0].Int("visitors"), tot[0].Int("client_errors")
		visitors := tot[0].Int("visitors")
		var nv int64
		if len(newV) > 0 {
			nv = newV[0].Int("n")
		}
		out["new_visitors"] = nv
		if visitors >= nv {
			out["returning_visitors"] = visitors - nv
		} else {
			out["returning_visitors"] = int64(0)
		}
	}
	for name, col := range map[string]string{"top_paths": "path", "top_referrers": "referrer", "devices": "device_type", "countries": "country"} {
		extra := ""
		if name == "top_paths" {
			extra = " AND event_type = 'page_view'"
		}
		lst, err := q.topList(ctx, `SELECT `+col+` AS k, count() AS n FROM events_raw FINAL
WHERE `+win+` AND `+col+` != ''`+extra+` AND event_type != 'identify' GROUP BY k ORDER BY n DESC, k LIMIT 10`, p, "k")
		if err != nil {
			return nil, err
		}
		out[name] = lst
	}
	return out, nil
}

// Payments (platform only): attempt outcomes by method, failure codes, refunds, orders awaiting payment.
func (q *Queries) Payments(ctx context.Context, r Range) (map[string]any, error) {
	p := rangeParams(r)
	const win = "at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')}"
	rows, err := q.DB.Query(ctx, `SELECT method, countIf(kind = 'succeeded') AS ok, countIf(kind = 'failed') AS failed,
  sumIf(amount_minor, kind = 'succeeded') AS amount
FROM fact_payment_events FINAL WHERE `+win+` GROUP BY method ORDER BY method`, p)
	if err != nil {
		return nil, err
	}
	var methods []map[string]any
	var totOK, totFail, totAmount int64
	for _, m := range rows {
		ok, f := m.Int("ok"), m.Int("failed")
		rate := 0.0
		if ok+f > 0 {
			rate = float64(ok) / float64(ok+f)
		}
		methods = append(methods, map[string]any{"method": m.Str("method"), "succeeded": ok, "failed": f, "success_rate": rate, "amount": money(m.Int("amount"))})
		totOK, totFail, totAmount = totOK+ok, totFail+f, totAmount+m.Int("amount")
	}
	rate := 0.0
	if totOK+totFail > 0 {
		rate = float64(totOK) / float64(totOK+totFail)
	}
	codes, err := q.topList(ctx, `SELECT failure_code AS k, count() AS n FROM fact_payment_events FINAL
WHERE `+win+` AND kind = 'failed' AND failure_code != '' GROUP BY k ORDER BY n DESC, k LIMIT 10`, p, "k")
	if err != nil {
		return nil, err
	}
	ref, err := q.DB.Query(ctx, `SELECT count() AS n, sum(amount_minor) AS amount FROM fact_refunds FINAL WHERE `+win, p)
	if err != nil {
		return nil, err
	}
	await, err := q.DB.Query(ctx, `SELECT count() AS n FROM (SELECT order_id, argMax(status, at) AS cur FROM fact_order_status FINAL GROUP BY order_id HAVING cur = 'AWAITING_PAYMENT')`, nil)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"succeeded": totOK, "failed": totFail, "success_rate": rate, "amount_succeeded": money(totAmount),
		"by_method": methods, "failure_codes": codes,
	}
	if len(ref) > 0 {
		out["refunds"] = map[string]any{"count": ref[0].Int("n"), "amount": money(ref[0].Int("amount"))}
	}
	if len(await) > 0 {
		out["orders_awaiting_payment"] = await[0].Int("n")
	}
	return out, nil
}

// LowStock lists products that are out/low, or would sell out within 7 days at the 14-day sales rate.
func (q *Queries) LowStock(ctx context.Context, sc Scope, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	storeInv, storeOrd := "", ""
	if !sc.Platform {
		storeInv = "WHERE store_id = {store:UInt64}"
		storeOrd = "WHERE store_id = {store:UInt64}"
	}
	now := q.now().UTC()
	p := map[string]string{"since": Time(now.Add(-14 * 24 * time.Hour)), "limit": fmt.Sprint(limit)}
	if !sc.Platform {
		p["store"] = fmt.Sprint(sc.StoreID)
	}
	sql := `
WITH ord AS (
  SELECT order_id, countIf(status IN ('CANCELED','EXPIRED','FAILED')) > 0 AS dead, min(at) AS created_at
  FROM fact_order_status FINAL ` + storeOrd + ` GROUP BY order_id
),
rate AS (
  SELECT product_id, sum(qty) / 14 AS per_day FROM fact_order_items FINAL
  WHERE order_id IN (SELECT order_id FROM ord WHERE NOT dead AND created_at >= {since:DateTime64(3,'UTC')})
  GROUP BY product_id
),
inv AS (
  SELECT product_id, any(store_id) AS sid, argMax(available, at) AS available, argMax(reserved, at) AS reserved,
         argMax(level, at) AS level, max(at) AS last_change
  FROM fact_inventory FINAL ` + storeInv + ` GROUP BY product_id
)
SELECT product_id, dp.name AS name, dp.status AS status, sid, available, reserved, level,
       r.per_day AS per_day, if(r.per_day > 0, available / r.per_day, -1) AS days_left, last_change
FROM inv
LEFT JOIN (SELECT product_id, name, status FROM dim_products FINAL) AS dp USING (product_id)
LEFT JOIN rate AS r USING (product_id)
WHERE (dp.status IN ('active', '') OR dp.status IS NULL) AND (level IN ('none', 'low') OR (r.per_day > 0 AND available / r.per_day <= 7))
ORDER BY if(days_left < 0, 1e9, days_left), product_id
LIMIT {limit:UInt32}`
	rows, err := q.DB.Query(ctx, sql, p)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		item := map[string]any{
			"product_id": r.Int("product_id"), "name": r.Str("name"), "available": r.Int("available"),
			"reserved": r.Int("reserved"), "level": r.Str("level"), "units_per_day": round2(r.Float("per_day")),
		}
		if d := r.Float("days_left"); d >= 0 {
			item["days_left"] = round1(d)
		} else {
			item["days_left"] = nil // no sales in the last 14 days: cannot estimate
		}
		if sc.Platform {
			item["store_id"] = r.Int("sid")
		}
		out = append(out, item)
	}
	return out, nil
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

// SearchTerms (platform): most used queries and queries that returned nothing.
func (q *Queries) SearchTerms(ctx context.Context, r Range, limit int) (map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	p := rangeParams(r)
	p["limit"] = fmt.Sprint(limit)
	const base = `FROM events_raw FINAL WHERE ts >= {from:DateTime64(3,'UTC')} AND ts < {to:DateTime64(3,'UTC')}
  AND event_type = 'search' AND JSONExtractString(props, 'q') != ''`
	top, err := q.DB.Query(ctx, `SELECT lowerUTF8(JSONExtractString(props, 'q')) AS term, count() AS n,
  avg(JSONExtractInt(props, 'result_count')) AS avg_results `+base+` GROUP BY term ORDER BY n DESC, term LIMIT {limit:UInt32}`, p)
	if err != nil {
		return nil, err
	}
	zero, err := q.DB.Query(ctx, `SELECT lowerUTF8(JSONExtractString(props, 'q')) AS term, count() AS n `+base+`
  AND JSONExtractInt(props, 'result_count') = 0 GROUP BY term ORDER BY n DESC, term LIMIT {limit:UInt32}`, p)
	if err != nil {
		return nil, err
	}
	var topOut, zeroOut []map[string]any
	for _, r := range top {
		topOut = append(topOut, map[string]any{"term": r.Str("term"), "count": r.Int("n"), "avg_results": round1(r.Float("avg_results"))})
	}
	for _, r := range zero {
		zeroOut = append(zeroOut, map[string]any{"term": r.Str("term"), "count": r.Int("n")})
	}
	return map[string]any{"top": topOut, "zero_results": zeroOut}, nil
}

// DataHealth (platform): rows, freshness and ingestion lag per table, plus the last hour of events by type.
func (q *Queries) DataHealth(ctx context.Context) (map[string]any, error) {
	var tables []map[string]any
	for _, t := range Tables {
		rows, err := q.DB.Query(ctx, fmt.Sprintf(`SELECT count() AS n, toString(max(%[1]s)) AS last_at,
  toUnixTimestamp64Milli(max(%[1]s)) AS last_ms,
  avgIf(dateDiff('millisecond', %[1]s, ingested_at), ingested_at > now64(3) - INTERVAL 15 MINUTE) AS lag_ms
FROM %[2]s`, t.TimeCol, t.Name), nil)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"table": t.Name}
		if len(rows) > 0 {
			entry["rows"] = rows[0].Int("n")
			if rows[0].Int("n") > 0 {
				entry["last_event_at"] = time.UnixMilli(rows[0].Int("last_ms")).UTC()
			}
			entry["avg_lag_seconds_15m"] = round2(rows[0].Float("lag_ms") / 1000)
		}
		tables = append(tables, entry)
	}
	byType, err := q.topList(ctx, `SELECT event_type AS k, count() AS n FROM events_raw FINAL
WHERE ts_server >= now64(3) - INTERVAL 1 HOUR GROUP BY k ORDER BY n DESC`, nil, "k")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"tables": tables, "events_last_hour_by_type": byType,
		"reconciliation": "not_implemented", // nightly Postgres ↔ ClickHouse comparison is planned, see docs 05 §1.3
	}, nil
}
