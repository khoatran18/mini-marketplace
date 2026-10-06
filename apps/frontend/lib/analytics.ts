// Helpers for the analytics endpoints (docs 05/06). The gateway endpoints are built in parallel and the exact JSON
// is not final, so everything here is tolerant: it accepts the documented envelope `{ as_of, tz, source, data }`
// and several reasonable shapes for `data`, and returns empty results instead of throwing.

import { formatCompact, formatNumber, formatPercent, formatVND, formatDateTime, formatDayMonth, formatDayHour, toNumber } from './format';
import type { AnalyticsEnvelope } from './types';

export type Period = 'today' | '7d' | '30d' | 'mtd' | 'custom';

export interface RangeValue {
  period: Period;
  from?: string; // yyyy-mm-dd, only for period=custom
  to?: string;
}

export const periodLabels: Record<Exclude<Period, 'custom'>, string> = { today: 'Hôm nay', '7d': '7 ngày', '30d': '30 ngày', mtd: 'Tháng này' };

export function rangeParams(range: RangeValue, compare = true): Record<string, string> {
  const params: Record<string, string> = { period: range.period };
  if (range.period === 'custom') {
    if (range.from) params.from = range.from;
    if (range.to) params.to = range.to;
  }
  if (compare) params.compare = 'prev';
  return params;
}

export function unwrap(envelope: AnalyticsEnvelope | null | undefined): { data: unknown; asOf?: string; source?: string; tz?: string } {
  if (!envelope || typeof envelope !== 'object') return { data: null };
  if ('data' in envelope && envelope.data !== undefined) {
    return { data: envelope.data, asOf: envelope.as_of, source: envelope.source, tz: envelope.tz };
  }
  const { as_of, source, tz, ...rest } = envelope;
  return { data: rest, asOf: as_of, source, tz };
}

type Json = Record<string, unknown>;
const isObject = (value: unknown): value is Json => typeof value === 'object' && value !== null && !Array.isArray(value);
const isScalar = (value: unknown): value is string | number | boolean | null => value === null || ['string', 'number', 'boolean'].includes(typeof value);
const numeric = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() !== '' && Number.isFinite(Number(value))) return Number(value);
  return null;
};

// ---- labels & value kinds ------------------------------------------------------------------------------------------

export const metricLabels: Record<string, string> = {
  revenue: 'Doanh thu',
  gmv: 'GMV',
  orders: 'Đơn hàng',
  order_count: 'Số đơn',
  units: 'Số lượng bán',
  units_sold: 'Số lượng bán',
  aov: 'Giá trị TB/đơn (AOV)',
  cancel_rate: 'Tỉ lệ huỷ',
  cancellation_rate: 'Tỉ lệ huỷ',
  refund_rate: 'Tỉ lệ hoàn',
  conversion: 'Chuyển đổi',
  conversion_rate: 'Tỉ lệ chuyển đổi',
  views: 'Lượt xem',
  page_views: 'Lượt xem trang',
  sessions: 'Phiên',
  unique_visitors: 'Người dùng duy nhất',
  uv: 'Người dùng duy nhất',
  active_users: 'Người dùng hoạt động',
  cart: 'Giỏ hàng',
  carts: 'Giỏ hàng',
  checkout: 'Thanh toán',
  checkouts: 'Bắt đầu thanh toán',
  paid: 'Đã thanh toán',
  name: 'Tên',
  product_id: 'Mã SP',
  product_name: 'Sản phẩm',
  success_rate: 'Tỉ lệ thành công',
  failure_code: 'Mã lỗi',
  failures: 'Số lần lỗi',
  count: 'Số lượng',
  method: 'Phương thức',
  status: 'Trạng thái',
  lag_seconds: 'Độ trễ (giây)',
  events_per_min: 'Event/phút',
  rejected_rate: 'Tỉ lệ bị loại',
  inventory: 'Tồn kho',
  days_of_stock: 'Ngày còn bán',
  stock: 'Tồn kho'
};

export function labelFor(key: string): string {
  if (metricLabels[key]) return metricLabels[key];
  const text = key.replace(/[_-]+/g, ' ').trim();
  return text.charAt(0).toUpperCase() + text.slice(1);
}

export type ValueKind = 'money' | 'percent' | 'number' | 'date' | 'text';

export function kindFor(key: string, sample?: unknown): ValueKind {
  if (/(^|_)(revenue|gmv|aov|amount|sales|price|total_vnd|refunded)(_|$)/i.test(key) && !/count|rate/i.test(key)) return 'money';
  if (/(rate|ratio|conversion|pct|percent|share)/i.test(key)) return 'percent';
  if (/(^|_)(at|ts|time|date|day|hour|bucket)$/i.test(key) && typeof sample === 'string') return 'date';
  if (numeric(sample) !== null) return 'number';
  return 'text';
}

export function formatByKind(value: unknown, kind: ValueKind): string {
  if (value === null || value === undefined || value === '') return '–';
  if (isObject(value) && 'amount' in value) return formatVND(value as unknown as { amount: number | string });
  switch (kind) {
    case 'money':
      return formatVND(value as number | string);
    case 'percent': {
      const n = toNumber(value as number | string);
      return n !== 0 && Math.abs(n) <= 1 ? formatPercent(n) : `${formatNumber(n)}%`;
    }
    case 'number':
      return Math.abs(toNumber(value as number | string)) >= 100000 ? formatCompact(value as number | string) : formatNumber(value as number | string);
    case 'date':
      return formatDateTime(String(value));
    default:
      return typeof value === 'object' ? JSON.stringify(value) : String(value);
  }
}

export function formatByKindDelta(delta: number): string {
  const sign = delta > 0 ? '▲ +' : delta < 0 ? '▼ ' : '';
  return `${sign}${(delta * 100).toFixed(1).replace('.', ',')}%`;
}

// ---- KPIs ------------------------------------------------------------------------------------------------------------

export interface Kpi {
  key: string;
  label: string;
  value: unknown;
  kind: ValueKind;
  previous?: number | null;
  /** relative change vs the previous period as a fraction (0.12 = +12 %) */
  delta?: number | null;
}

function scalarOf(value: unknown): unknown {
  if (isObject(value)) {
    if ('amount' in value) return value; // money object
    if ('value' in value) return value.value;
  }
  return value;
}

export function toKpis(data: unknown): Kpi[] {
  if (Array.isArray(data)) {
    return data
      .filter(isObject)
      .map((row, index) => {
        const key = String(row.key ?? row.metric ?? row.name ?? `m${index}`);
        return buildKpi(key, row.value ?? row.current, row.previous ?? row.prev, row.delta ?? row.change, row.label as string | undefined);
      });
  }
  if (!isObject(data)) return [];
  const current = isObject(data.current) ? data.current : data;
  const previous = isObject(data.previous) ? data.previous : isObject(data.prev) ? data.prev : null;
  const kpis: Kpi[] = [];
  for (const [key, raw] of Object.entries(current)) {
    if (['as_of', 'tz', 'source', 'period', 'from', 'to', 'currency'].includes(key)) continue;
    const value = scalarOf(raw);
    if (!isScalar(value) && !(isObject(value) && 'amount' in value)) continue;
    const obj = isObject(raw) ? raw : null;
    const prevRaw = obj ? obj.previous ?? obj.prev : previous ? previous[key] : undefined;
    kpis.push(buildKpi(key, value, scalarOf(prevRaw), obj?.delta ?? obj?.change ?? obj?.change_pct));
  }
  return kpis;
}

function buildKpi(key: string, value: unknown, prev: unknown, delta: unknown, label?: string): Kpi {
  const kind = kindFor(key, isObject(value) ? 1 : value);
  const previous = numeric(isObject(prev) ? prev.amount : prev);
  const current = numeric(isObject(value) ? value.amount : value);
  let change = numeric(delta);
  if (change === null && previous !== null && current !== null && previous !== 0) change = (current - previous) / Math.abs(previous);
  return { key, label: label ?? labelFor(key), value, kind: isObject(value) ? 'money' : kind === 'text' && current !== null ? 'number' : kind, previous, delta: change };
}

// ---- series ----------------------------------------------------------------------------------------------------------

export interface SeriesPoint {
  x: string;
  y: number;
  prev?: number;
}

const xKeys = ['ts', 't', 'time', 'bucket', 'date', 'day', 'hour', 'period', 'x', 'label'];
const yKeys = ['value', 'v', 'y'];
const prevKeys = ['prev', 'previous', 'prev_value', 'compare', 'previous_value'];

export function toSeries(data: unknown, metric?: string): SeriesPoint[] {
  let rows: unknown = data;
  if (isObject(data)) {
    if (Array.isArray(data.labels) && Array.isArray(data.values)) {
      const previous = Array.isArray(data.previous) ? data.previous : [];
      const values = data.values as unknown[];
      return (data.labels as unknown[]).map((label, index) => ({
        x: String(label),
        y: toNumber(values[index] as number),
        prev: previous[index] !== undefined ? toNumber(previous[index] as number) : undefined
      }));
    }
    rows = data.points ?? data.series ?? data.buckets ?? data.items ?? data.rows ?? data.data ?? [];
  }
  if (!Array.isArray(rows)) return [];
  const points: SeriesPoint[] = [];
  for (const row of rows) {
    if (!isObject(row)) continue;
    const xKey = xKeys.find((key) => key in row);
    let yKey = [metric, ...yKeys].find((key): key is string => Boolean(key) && key! in row);
    if (!yKey) yKey = Object.keys(row).find((key) => key !== xKey && !prevKeys.includes(key) && numeric(scalarOf(row[key])) !== null);
    if (!xKey || !yKey) continue;
    const prevKey = prevKeys.find((key) => key in row);
    points.push({
      x: String(row[xKey]),
      y: toNumber(scalarOf(row[yKey]) as number),
      prev: prevKey ? toNumber(scalarOf(row[prevKey]) as number) : undefined
    });
  }
  return points;
}

export function formatX(x: string, granularity?: string): string {
  const parsed = new Date(x);
  if (Number.isNaN(parsed.getTime()) || !/\d{4}-\d{2}-\d{2}/.test(x)) return x;
  return granularity === 'hour' || /T\d{2}:/.test(x) ? formatDayHour(x) : formatDayMonth(x);
}

// ---- rows & funnel ---------------------------------------------------------------------------------------------------

export function toRows(data: unknown): Json[] {
  if (Array.isArray(data)) return data.filter(isObject);
  if (isObject(data)) {
    for (const key of ['items', 'rows', 'products', 'top', 'results', 'data', 'list']) {
      if (Array.isArray(data[key])) return (data[key] as unknown[]).filter(isObject);
    }
  }
  return [];
}

export interface FunnelStep {
  label: string;
  value: number;
}

const funnelLabelKeys = ['step', 'stage', 'name', 'label', 'event'];
const funnelValueKeys = ['count', 'value', 'users', 'sessions', 'total'];

export function toFunnel(data: unknown): FunnelStep[] {
  const rows = isObject(data) && Array.isArray(data.steps) ? data.steps : isObject(data) && Array.isArray(data.stages) ? data.stages : null;
  if (Array.isArray(data) || rows) {
    const list = (rows ?? (data as unknown[])) as unknown[];
    return list.filter(isObject).flatMap((row) => {
      const labelKey = funnelLabelKeys.find((key) => key in row);
      const valueKey = funnelValueKeys.find((key) => key in row);
      return labelKey && valueKey ? [{ label: labelFor(String(row[labelKey])), value: toNumber(row[valueKey] as number) }] : [];
    });
  }
  if (isObject(data)) {
    const order = ['impressions', 'views', 'product_views', 'view', 'carts', 'cart', 'add_to_cart', 'checkouts', 'checkout', 'paid', 'orders'];
    const entries = Object.entries(data).filter(([, value]) => numeric(value) !== null);
    entries.sort(([a], [b]) => (order.indexOf(a) === -1 ? 99 : order.indexOf(a)) - (order.indexOf(b) === -1 ? 99 : order.indexOf(b)));
    return entries.map(([key, value]) => ({ label: labelFor(key), value: toNumber(value as number) }));
  }
  return [];
}

export function isEmptyData(data: unknown): boolean {
  if (data === null || data === undefined) return true;
  if (Array.isArray(data)) return data.length === 0;
  if (isObject(data)) return Object.keys(data).length === 0;
  return false;
}

export { isObject, isScalar };
