// Package store holds the ClickHouse schema (versioned migrations), the batching writer and the report queries.
package store

import (
	"context"
	"fmt"
	"strings"

	"analytics-service/internal/ch"
)

// Timezone is the single display/grouping timezone of the platform (ADR: fixed, no DST, no selector).
const Timezone = "Asia/Ho_Chi_Minh"

const ts = "DateTime64(3, 'UTC')"

// migrations are applied in order, once each, and recorded in schema_migrations. NEVER edit an applied one:
// append a new migration instead.
func migrations(retentionMonths int) []string {
	if retentionMonths <= 0 {
		retentionMonths = 13
	}
	return []string{
		// 1 · raw clickstream (browser events + server-attached fields)
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS events_raw (
  event_id String, event_type LowCardinality(String),
  ts %[1]s, ts_server %[1]s,
  anonymous_id String, session_id String, user_id UInt64, role LowCardinality(String), store_id UInt64,
  surface LowCardinality(String), path String, referrer String,
  product_id UInt64, position UInt16, props String,
  device_type LowCardinality(String), os LowCardinality(String), country LowCardinality(String), ua_family LowCardinality(String),
  analytics_consent UInt8, ip_hash String, app_version LowCardinality(String),
  ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ts_server)
  PARTITION BY toYYYYMM(ts) ORDER BY (event_type, anonymous_id, ts, event_id)
  TTL toDateTime(ts) + INTERVAL %[2]d MONTH`, ts, retentionMonths),
		// 2 · one row per (order, status): the first time an order reached each status (ReplacingMergeTree by `at`)
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fact_order_status (
  order_id UInt64, status LowCardinality(String), checkout_id String, buyer_id UInt64, store_id UInt64,
  payment_method LowCardinality(String), payment_status LowCardinality(String),
  subtotal_minor Int64, shipping_fee_minor Int64, total_minor Int64,
  at %[1]s, ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(at) ORDER BY (order_id, status)`, ts),
		// 3 · order lines (immutable after checkout)
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fact_order_items (
  order_id UInt64, item_id UInt64, store_id UInt64, product_id UInt64, category_id UInt64,
  qty UInt32, unit_price_minor Int64, at %[1]s, ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(at) ORDER BY (order_id, item_id)`, ts),
		// 4 · payment outcomes (attempt level)
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fact_payment_events (
  payment_id UInt64, kind LowCardinality(String), checkout_id String, buyer_id UInt64,
  method LowCardinality(String), amount_minor Int64, failure_code LowCardinality(String), terminal UInt8,
  at %[1]s, ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(at) ORDER BY (payment_id, kind, at)`, ts),
		// 5 · refunds
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fact_refunds (
  refund_id UInt64, payment_id UInt64, checkout_id String, order_id UInt64,
  amount_minor Int64, reason String, at %[1]s, ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(at) ORDER BY refund_id`, ts),
		// 6 · product dimension (latest version wins)
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS dim_products (
  product_id UInt64, store_id UInt64, name String, sku String, category_id UInt64, brand String,
  price_minor Int64, status LowCardinality(String), updated_at %[1]s, ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(updated_at) ORDER BY product_id`, ts),
		// 7 · stock movements
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fact_inventory (
  product_id UInt64, store_id UInt64, delta Int64, available Int64, reserved Int64,
  level LowCardinality(String), reason LowCardinality(String), ref_type LowCardinality(String), ref_id String,
  at %[1]s, ingested_at %[1]s DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(at) ORDER BY (product_id, at, reason, ref_type, ref_id)`, ts),
	}
}

// Migrate creates the database and applies pending migrations. c must be bound to no database.
func Migrate(ctx context.Context, c *ch.Client, database string, retentionMonths int) error {
	if !validIdent(database) {
		return fmt.Errorf("invalid database name %q", database)
	}
	if err := c.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+database, nil); err != nil {
		return err
	}
	db := c.WithDatabase(database)
	if err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version UInt32, applied_at DateTime DEFAULT now()) ENGINE = MergeTree ORDER BY version`, nil); err != nil {
		return err
	}
	rows, err := db.Query(ctx, "SELECT max(version) AS v FROM schema_migrations", nil)
	if err != nil {
		return err
	}
	var applied int64
	if len(rows) > 0 {
		applied = rows[0].Int("v")
	}
	for i, ddl := range migrations(retentionMonths) {
		v := int64(i + 1)
		if v <= applied {
			continue
		}
		if err := db.Exec(ctx, ddl, nil); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
		if err := db.Exec(ctx, fmt.Sprintf("INSERT INTO schema_migrations (version) VALUES (%d)", v), nil); err != nil {
			return err
		}
	}
	return nil
}

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range strings.ToLower(s) {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// Tables lists the tables written by the pipeline (used by the data-health report).
var Tables = []struct{ Name, TimeCol string }{
	{"events_raw", "ts_server"},
	{"fact_order_status", "at"},
	{"fact_order_items", "at"},
	{"fact_payment_events", "at"},
	{"fact_refunds", "at"},
	{"dim_products", "updated_at"},
	{"fact_inventory", "at"},
}
