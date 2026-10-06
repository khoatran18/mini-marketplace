// Typed parsing of the analytics endpoints (gateway: /seller/analytics/*, /admin/analytics/*).
// The gateway wraps every report as { as_of, source, cached, timezone, data }. Money is VND (float), rates are
// fractions (0.12 = 12 %), buckets are local Asia/Ho_Chi_Minh time. Parsers never throw: a payload that does not have
// the expected top-level shape yields `null`, which the panels render as the "no data" state.

import { formatDayMonth, toLocalISODate } from './format';
import type { AnalyticsEnvelope } from './types';

export type Period = 'today' | '7d' | '30d' | 'mtd' | 'custom';

export interface RangeValue {
  period: Period;
  from?: string; // yyyy-mm-dd, only for period=custom
  to?: string;
}

export const periodLabels: Record<Exclude<Period, 'custom'>, string> = { today: 'Hôm nay', '7d': '7 ngày', '30d': '30 ngày', mtd: 'Tháng này' };

function shiftDay(iso: string, days: number): string {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(Date.UTC(y, m - 1, d + days)).toISOString().slice(0, 10);
}

/** from/to (yyyy-mm-dd in Asia/Ho_Chi_Minh, `to` inclusive) for a range selection. */
export function rangeParams(range: RangeValue, now: Date = new Date()): { from: string; to: string } {
  const today = toLocalISODate(now);
  switch (range.period) {
    case 'today':
      return { from: today, to: today };
    case '7d':
      return { from: shiftDay(today, -6), to: today };
    case '30d':
      return { from: shiftDay(today, -29), to: today };
    case 'mtd':
      return { from: `${today.slice(0, 8)}01`, to: today };
    default:
      return { from: range.from || today, to: range.to || today };
  }
}

// ---- envelope ----------------------------------------------------------------------------------------------------------

export interface Unwrapped {
  data: unknown;
  asOf?: string;
  source?: string;
  cached?: boolean;
}

export function unwrap(envelope: AnalyticsEnvelope | null | undefined): Unwrapped {
  if (!envelope || typeof envelope !== 'object') return { data: null };
  return { data: envelope.data ?? null, asOf: str(envelope.as_of) || undefined, source: str(envelope.source) || undefined, cached: envelope.cached === true };
}

// ---- tiny strict readers -----------------------------------------------------------------------------------------------

type Json = Record<string, unknown>;
const obj = (v: unknown): Json | null => (typeof v === 'object' && v !== null && !Array.isArray(v) ? (v as Json) : null);
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const numOrNull = (v: unknown): number | null => (typeof v === 'number' && Number.isFinite(v) ? v : null);
const str = (v: unknown): string => (typeof v === 'string' ? v : '');
const rows = <T>(v: unknown, map: (row: Json) => T): T[] => arr(v).flatMap((item) => {
  const row = obj(item);
  return row ? [map(row)] : [];
});

// ---- summary -----------------------------------------------------------------------------------------------------------

export interface SummaryBlock {
  revenue: number;
  refunds: number;
  net_revenue: number;
  gmv: number;
  orders_placed: number;
  orders_recognized: number;
  aov: number;
  canceled: number;
  expired: number;
  refunded: number;
  cancel_rate: number;
}

export interface Summary {
  current: SummaryBlock;
  previous: SummaryBlock | null;
  from: string;
  to: string;
}

function summaryBlock(v: unknown): SummaryBlock | null {
  const o = obj(v);
  if (!o) return null;
  return {
    revenue: num(o.revenue),
    refunds: num(o.refunds),
    net_revenue: num(o.net_revenue),
    gmv: num(o.gmv),
    orders_placed: num(o.orders_placed),
    orders_recognized: num(o.orders_recognized),
    aov: num(o.aov),
    canceled: num(o.canceled),
    expired: num(o.expired),
    refunded: num(o.refunded),
    cancel_rate: num(o.cancel_rate)
  };
}

export function parseSummary(data: unknown): Summary | null {
  const o = obj(data);
  const current = summaryBlock(o?.current);
  if (!o || !current) return null;
  return { current, previous: summaryBlock(o.previous), from: str(o.from), to: str(o.to) };
}

/** Relative change vs the previous period as a fraction; null when there is nothing to compare with. */
export function change(current: number, previous: number | undefined | null): number | null {
  if (previous === undefined || previous === null || previous === 0) return null;
  return (current - previous) / Math.abs(previous);
}

// ---- timeseries --------------------------------------------------------------------------------------------------------

export interface RevenuePoint {
  bucket: string; // "YYYY-MM-DD HH:MM:SS", local time
  revenue: number;
  refunds: number;
  orders: number;
  placed: number;
}

export type RevenueMetric = 'revenue' | 'refunds' | 'orders' | 'placed';

export function parseTimeseries(data: unknown): RevenuePoint[] | null {
  if (!Array.isArray(data)) return null;
  return rows(data, (r) => ({ bucket: str(r.bucket), revenue: num(r.revenue), refunds: num(r.refunds), orders: num(r.orders), placed: num(r.placed) }));
}

/** "05/10" for day/week/month buckets, "05/10 14:00" for hours. The bucket is already local time: no Date parsing. */
export function formatBucket(bucket: string, granularity: string): string {
  const m = bucket.match(/^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2}))?/);
  if (!m) return bucket;
  if (granularity === 'hour' && m[4]) return `${m[3]}/${m[2]} ${m[4]}:${m[5]}`;
  if (granularity === 'month') return `${m[2]}/${m[1]}`;
  return formatDayMonth(`${m[1]}-${m[2]}-${m[3]}T12:00:00+07:00`);
}

// ---- top products ------------------------------------------------------------------------------------------------------

export interface TopProduct {
  product_id: number;
  name: string;
  status: string;
  units: number;
  revenue: number;
  impressions: number;
  clicks: number;
  views: number;
  carts: number;
  conversion: number;
}

export type TopSort = 'revenue' | 'units' | 'views' | 'conversion' | 'viewed_unsold';

export function parseTopProducts(data: unknown): TopProduct[] | null {
  if (!Array.isArray(data)) return null;
  return rows(data, (r) => ({
    product_id: num(r.product_id),
    name: str(r.name),
    status: str(r.status),
    units: num(r.units),
    revenue: num(r.revenue),
    impressions: num(r.impressions),
    clicks: num(r.clicks),
    views: num(r.views),
    carts: num(r.carts),
    conversion: num(r.conversion)
  }));
}

// ---- funnel ------------------------------------------------------------------------------------------------------------

export interface FunnelStep {
  step: 'view' | 'cart' | 'checkout' | 'paid' | string;
  count: number | null; // null = not measurable for this scope
}

export interface DeviceFunnel {
  device_type: string;
  views: number;
  carts: number;
  checkouts: number;
}

export interface Funnel {
  steps: FunnelStep[];
  unit: string;
  by_device: DeviceFunnel[];
}

export const funnelLabels: Record<string, string> = { view: 'Xem sản phẩm', cart: 'Thêm vào giỏ', checkout: 'Bắt đầu thanh toán', paid: 'Đã thanh toán' };

export function parseFunnel(data: unknown): Funnel | null {
  const o = obj(data);
  if (!o || !Array.isArray(o.steps)) return null;
  return {
    steps: rows(o.steps, (r) => ({ step: str(r.step), count: numOrNull(r.count) })),
    unit: str(o.unit),
    by_device: rows(o.by_device, (r) => ({ device_type: str(r.device_type), views: num(r.views), carts: num(r.carts), checkouts: num(r.checkouts) }))
  };
}

// ---- low stock ---------------------------------------------------------------------------------------------------------

export interface LowStockRow {
  product_id: number;
  name: string;
  available: number;
  reserved: number;
  level: string;
  units_per_day: number;
  days_left: number | null;
  store_id: number | null;
}

export function parseLowStock(data: unknown): LowStockRow[] | null {
  if (!Array.isArray(data)) return null;
  return rows(data, (r) => ({
    product_id: num(r.product_id),
    name: str(r.name),
    available: num(r.available),
    reserved: num(r.reserved),
    level: str(r.level),
    units_per_day: num(r.units_per_day),
    days_left: numOrNull(r.days_left),
    store_id: numOrNull(r.store_id)
  }));
}

// ---- traffic -----------------------------------------------------------------------------------------------------------

export interface KeyCount {
  key: string;
  count: number;
}

export interface TrafficPoint {
  bucket: string;
  page_views: number;
  sessions: number;
  visitors: number;
}

export interface Traffic {
  page_views: number;
  sessions: number;
  visitors: number;
  client_errors: number;
  new_visitors: number;
  returning_visitors: number;
  series: TrafficPoint[];
  top_paths: KeyCount[];
  top_referrers: KeyCount[];
  devices: KeyCount[];
  countries: KeyCount[];
}

const keyCounts = (v: unknown): KeyCount[] => rows(v, (r) => ({ key: str(r.key), count: num(r.count) }));

export function parseTraffic(data: unknown): Traffic | null {
  const o = obj(data);
  if (!o) return null;
  return {
    page_views: num(o.page_views),
    sessions: num(o.sessions),
    visitors: num(o.visitors),
    client_errors: num(o.client_errors),
    new_visitors: num(o.new_visitors),
    returning_visitors: num(o.returning_visitors),
    series: rows(o.series, (r) => ({ bucket: str(r.bucket), page_views: num(r.page_views), sessions: num(r.sessions), visitors: num(r.visitors) })),
    top_paths: keyCounts(o.top_paths),
    top_referrers: keyCounts(o.top_referrers),
    devices: keyCounts(o.devices),
    countries: keyCounts(o.countries)
  };
}

// ---- payments ----------------------------------------------------------------------------------------------------------

export interface PaymentMethodStat {
  method: string;
  succeeded: number;
  failed: number;
  success_rate: number;
  amount: number;
}

export interface PaymentsReport {
  succeeded: number;
  failed: number;
  success_rate: number;
  amount_succeeded: number;
  by_method: PaymentMethodStat[];
  failure_codes: KeyCount[];
  refunds: { count: number; amount: number };
  orders_awaiting_payment: number;
}

export function parsePayments(data: unknown): PaymentsReport | null {
  const o = obj(data);
  if (!o) return null;
  const refunds = obj(o.refunds);
  return {
    succeeded: num(o.succeeded),
    failed: num(o.failed),
    success_rate: num(o.success_rate),
    amount_succeeded: num(o.amount_succeeded),
    by_method: rows(o.by_method, (r) => ({ method: str(r.method), succeeded: num(r.succeeded), failed: num(r.failed), success_rate: num(r.success_rate), amount: num(r.amount) })),
    failure_codes: keyCounts(o.failure_codes),
    refunds: { count: num(refunds?.count), amount: num(refunds?.amount) },
    orders_awaiting_payment: num(o.orders_awaiting_payment)
  };
}

// ---- search terms ------------------------------------------------------------------------------------------------------

export interface SearchTerms {
  top: { term: string; count: number; avg_results: number }[];
  zero_results: { term: string; count: number }[];
}

export function parseSearchTerms(data: unknown): SearchTerms | null {
  const o = obj(data);
  if (!o) return null;
  return {
    top: rows(o.top, (r) => ({ term: str(r.term), count: num(r.count), avg_results: num(r.avg_results) })),
    zero_results: rows(o.zero_results, (r) => ({ term: str(r.term), count: num(r.count) }))
  };
}

// ---- data health -------------------------------------------------------------------------------------------------------

export interface DataHealth {
  tables: { table: string; rows: number; last_event_at: string; avg_lag_seconds_15m: number | null }[];
  events_last_hour_by_type: KeyCount[];
  reconciliation: string;
}

export function parseDataHealth(data: unknown): DataHealth | null {
  const o = obj(data);
  if (!o) return null;
  return {
    tables: rows(o.tables, (r) => ({ table: str(r.table), rows: num(r.rows), last_event_at: str(r.last_event_at), avg_lag_seconds_15m: numOrNull(r.avg_lag_seconds_15m) })),
    events_last_hour_by_type: keyCounts(o.events_last_hour_by_type),
    reconciliation: str(o.reconciliation)
  };
}
