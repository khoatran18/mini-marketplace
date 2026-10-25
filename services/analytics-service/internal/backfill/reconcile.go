package backfill

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"

	"analytics-service/internal/ch"
	"analytics-service/internal/store"
)

// Line compares one metric of one day (Day "*" = the whole range).
type Line struct {
	Day        string  `json:"day"`
	Metric     string  `json:"metric"`
	Postgres   int64   `json:"postgres"`
	ClickHouse int64   `json:"clickhouse"`
	Diff       int64   `json:"diff"`  // clickhouse - postgres
	Ratio      float64 `json:"ratio"` // |diff| / max(|postgres|, 1)
	Mismatch   bool    `json:"mismatch"`
}

// Report is the result of Reconcile; it is a plain struct so the data-health report can embed it later
// (metric mm_reconcile_diff_ratio = MaxRatio).
type Report struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Tolerance  float64 `json:"tolerance"`
	Totals     []Line  `json:"totals"` // one line per metric over the whole range
	Lines      []Line  `json:"lines"`  // per day and metric (only days where either side is non-zero)
	Checked    int     `json:"checked"`
	Mismatches int     `json:"mismatches"` // day-level lines over tolerance
	MaxRatio   float64 `json:"max_ratio"`
	OK         bool    `json:"ok"`
}

// series maps day (YYYY-MM-DD, Asia/Ho_Chi_Minh) to {rows, amount}.
type series map[string][2]int64

type check struct {
	name, sumName string // metrics: name.rows and name.sumName ("" = rows only)
	pg, ch        string
}

// Money columns are minor units (1/100 VND). Every SELECT returns (d, n, s).
var checks = []check{
	{"order_status", "subtotal_minor",
		`SELECT to_char(h.at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(ROUND(o.subtotal*100)),0)::bigint AS s
FROM (SELECT order_id, to_status, min(at) AS at FROM order_status_histories WHERE to_status <> '' GROUP BY order_id, to_status) h
JOIN orders o ON o.id = h.order_id WHERE h.at >= $1 AND h.at < $2 GROUP BY d`,
		`SELECT toString(toDate(at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(subtotal_minor) AS s FROM fact_order_status FINAL
WHERE at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	{"order_items", "line_minor",
		`SELECT to_char(x.at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(i.quantity*ROUND(i.price*100)),0)::bigint AS s
FROM orders o JOIN order_items i ON i.order_id = o.id
JOIN LATERAL (SELECT COALESCE((SELECT max(at) FROM order_status_histories WHERE order_id = o.id), o.updated_at, o.created_at) AS at) x ON true
WHERE x.at >= $1 AND x.at < $2 GROUP BY d`,
		`SELECT toString(toDate(at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(toInt64(qty) * unit_price_minor) AS s FROM fact_order_items FINAL
WHERE at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	{"payments_succeeded", "amount_minor",
		`SELECT to_char(paid_at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(amount_minor),0)::bigint AS s
FROM payments WHERE paid_at IS NOT NULL AND paid_at >= $1 AND paid_at < $2 GROUP BY d`,
		`SELECT toString(toDate(at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(amount_minor) AS s FROM fact_payment_events FINAL
WHERE kind = 'succeeded' AND at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	{"payments_failed", "",
		`SELECT to_char(at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, 0::bigint AS s
FROM attempts a JOIN payments p ON p.id = a.payment_id WHERE a.status = 'FAILED' AND at >= $1 AND at < $2 GROUP BY d`,
		`SELECT toString(toDate(at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, 0 AS s FROM fact_payment_events FINAL
WHERE kind = 'failed' AND at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	{"refunds", "amount_minor",
		`SELECT to_char(f.at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(f.amount_minor),0)::bigint AS s
FROM refunds f JOIN payments p ON p.id = f.payment_id WHERE f.status = 'SUCCEEDED' AND f.at >= $1 AND f.at < $2 GROUP BY d`,
		`SELECT toString(toDate(at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(amount_minor) AS s FROM fact_refunds FINAL
WHERE at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	{"products", "price_minor",
		`SELECT to_char(ts AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(ROUND(price*100)),0)::bigint AS s
FROM (SELECT price, COALESCE(deleted_at, updated_at, created_at) AS ts FROM products) p WHERE ts >= $1 AND ts < $2 GROUP BY d`,
		`SELECT toString(toDate(updated_at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(price_minor) AS s FROM dim_products FINAL
WHERE updated_at >= {from:DateTime64(3,'UTC')} AND updated_at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	{"inventory", "delta",
		`SELECT to_char(l.at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(l.delta),0)::bigint AS s
FROM inventory_ledgers l JOIN products p ON p.id = l.product_id WHERE l.at >= $1 AND l.at < $2 GROUP BY d`,
		`SELECT toString(toDate(at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(delta) AS s FROM fact_inventory FINAL
WHERE at >= {from:DateTime64(3,'UTC')} AND at < {to:DateTime64(3,'UTC')} GROUP BY d`},
	// ADR-22: revenue = subtotal of orders PAID (online) / DELIVERED (COD), on the day it was recognised.
	{"revenue", "subtotal_minor",
		`SELECT to_char(rec_at AT TIME ZONE 'Asia/Ho_Chi_Minh','YYYY-MM-DD') AS d, count(*)::bigint AS n, COALESCE(sum(sub),0)::bigint AS s
FROM (SELECT o.id, ROUND(o.subtotal*100) AS sub, min(h.at) AS rec_at FROM orders o
      JOIN order_status_histories h ON h.order_id = o.id AND h.to_status = CASE WHEN o.payment_method = 'COD' THEN 'DELIVERED' ELSE 'PAID' END
      GROUP BY o.id, o.subtotal) r WHERE rec_at >= $1 AND rec_at < $2 GROUP BY d`,
		`WITH ` + store.OrdersCTE("") + `
SELECT toString(toDate(rec_at,'Asia/Ho_Chi_Minh')) AS d, count() AS n, sum(subtotal) AS s FROM orders
WHERE recognized AND rec_at >= {from:DateTime64(3,'UTC')} AND rec_at < {to:DateTime64(3,'UTC')} GROUP BY d`},
}

func loadPG(ctx context.Context, pg *sql.DB, q string, r Range) (series, error) {
	rows, err := pg.QueryContext(ctx, q, r.From, r.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := series{}
	for rows.Next() {
		var d string
		var n, s int64
		if err := rows.Scan(&d, &n, &s); err != nil {
			return nil, err
		}
		out[d] = [2]int64{n, s}
	}
	return out, rows.Err()
}

func loadCH(ctx context.Context, db *ch.Client, q string, r Range) (series, error) {
	rows, err := db.Query(ctx, q, map[string]string{"from": store.Time(r.From), "to": store.Time(r.To)})
	if err != nil {
		return nil, err
	}
	out := series{}
	for _, row := range rows {
		out[row.Str("d")] = [2]int64{row.Int("n"), row.Int("s")}
	}
	return out, nil
}

func mkLine(day, metric string, p, c int64, tol float64) Line {
	l := Line{Day: day, Metric: metric, Postgres: p, ClickHouse: c, Diff: c - p}
	l.Ratio = math.Abs(float64(l.Diff)) / math.Max(math.Abs(float64(p)), 1)
	l.Mismatch = l.Diff != 0 && l.Ratio > tol
	return l
}

// Reconcile compares Postgres and ClickHouse (FINAL) per day over the range. `pg` should be a read-only
// connection. tolerance is a ratio (0 = exact).
func Reconcile(ctx context.Context, pg *sql.DB, db *ch.Client, r Range, tolerance float64) (*Report, error) {
	rep := &Report{From: r.From.In(mustLoc()).Format("2006-01-02"), To: r.To.AddDate(0, 0, -1).In(mustLoc()).Format("2006-01-02"), Tolerance: tolerance}
	pgS := map[string]series{}
	chS := map[string]series{}
	for _, c := range checks {
		p, err := loadPG(ctx, pg, c.pg, r)
		if err != nil {
			return nil, fmt.Errorf("postgres %s: %w", c.name, err)
		}
		k, err := loadCH(ctx, db, c.ch, r)
		if err != nil {
			return nil, fmt.Errorf("clickhouse %s: %w", c.name, err)
		}
		pgS[c.name], chS[c.name] = p, k
	}
	// derived: net revenue = revenue - refunds (refunds count on the day they were paid back)
	for _, side := range []map[string]series{pgS, chS} {
		net := series{}
		for d, v := range side["revenue"] {
			net[d] = [2]int64{v[0], v[1]}
		}
		for d, v := range side["refunds"] {
			x := net[d]
			x[1] -= v[1]
			net[d] = x
		}
		side["net_revenue"] = net
	}
	all := append([]check(nil), checks...)
	all = append(all, check{name: "net_revenue", sumName: "subtotal_minor_minus_refunds"})

	for _, c := range all {
		type metric struct {
			name string
			idx  int
		}
		ms := []metric{{c.name + ".rows", 0}}
		if c.sumName != "" {
			ms = append(ms, metric{c.name + "." + c.sumName, 1})
		}
		if c.name == "net_revenue" {
			ms = ms[1:] // row count of the derived series is meaningless
		}
		days := map[string]bool{}
		for d := range pgS[c.name] {
			days[d] = true
		}
		for d := range chS[c.name] {
			days[d] = true
		}
		for _, m := range ms {
			var tp, tc int64
			for d := range days {
				p, k := pgS[c.name][d][m.idx], chS[c.name][d][m.idx]
				tp, tc = tp+p, tc+k
				if p == 0 && k == 0 {
					continue
				}
				l := mkLine(d, m.name, p, k, tolerance)
				rep.Lines = append(rep.Lines, l)
				rep.Checked++
				if l.Mismatch {
					rep.Mismatches++
				}
				if l.Ratio > rep.MaxRatio {
					rep.MaxRatio = l.Ratio
				}
			}
			rep.Totals = append(rep.Totals, mkLine("*", m.name, tp, tc, tolerance))
		}
	}
	sort.Slice(rep.Lines, func(i, j int) bool {
		if rep.Lines[i].Day != rep.Lines[j].Day {
			return rep.Lines[i].Day < rep.Lines[j].Day
		}
		return rep.Lines[i].Metric < rep.Lines[j].Metric
	})
	rep.OK = rep.Mismatches == 0
	return rep, nil
}
