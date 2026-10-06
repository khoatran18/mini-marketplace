// Fixed-locale formatting: Vietnamese, VND and the Asia/Ho_Chi_Minh time zone (UTC+7, no DST). There is deliberately
// no time zone picker (docs/platform/07-ui-design.md). Never use the browser's local zone for business dates.

export const TIME_ZONE = 'Asia/Ho_Chi_Minh';
export const LOCALE = 'vi-VN';

/** Money may arrive as a number, a numeric string ("125000") or `{ amount, currency }`. */
export type MoneyLike = number | string | { amount: number | string; currency?: string } | null | undefined;

export function toNumber(value: MoneyLike): number {
  if (value === null || value === undefined) return 0;
  if (typeof value === 'object') return toNumber(value.amount);
  const n = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(n) ? n : 0;
}

const vnd = new Intl.NumberFormat(LOCALE, { style: 'currency', currency: 'VND', maximumFractionDigits: 0 });
const plain = new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 2 });
const compact = new Intl.NumberFormat(LOCALE, { notation: 'compact', maximumFractionDigits: 1 });

export function formatVND(value: MoneyLike): string {
  return vnd.format(toNumber(value));
}

/** Short form for chart axes: 12,4 Tr ₫. */
export function formatVNDCompact(value: MoneyLike): string {
  return `${compact.format(toNumber(value))} ₫`;
}

export function formatNumber(value: number | string | null | undefined): string {
  return plain.format(toNumber(value));
}

export function formatCompact(value: number | string | null | undefined): string {
  return compact.format(toNumber(value));
}

export function formatPercent(fraction: number, digits = 1): string {
  return `${(fraction * 100).toFixed(digits).replace('.', ',')}%`;
}

function parse(value: string | number | Date | null | undefined): Date | null {
  if (value === null || value === undefined || value === '') return null;
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

const dateTime = new Intl.DateTimeFormat(LOCALE, {
  timeZone: TIME_ZONE,
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit'
});
const dateOnly = new Intl.DateTimeFormat(LOCALE, { timeZone: TIME_ZONE, day: '2-digit', month: '2-digit', year: 'numeric' });
const timeOnly = new Intl.DateTimeFormat(LOCALE, { timeZone: TIME_ZONE, hour: '2-digit', minute: '2-digit', second: '2-digit' });
const dayMonth = new Intl.DateTimeFormat(LOCALE, { timeZone: TIME_ZONE, day: '2-digit', month: '2-digit' });
const hourOnly = new Intl.DateTimeFormat(LOCALE, { timeZone: TIME_ZONE, day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });

export function formatDateTime(value: string | number | Date | null | undefined, fallback = '–'): string {
  const date = parse(value);
  return date ? dateTime.format(date) : fallback;
}

export function formatDate(value: string | number | Date | null | undefined, fallback = '–'): string {
  const date = parse(value);
  return date ? dateOnly.format(date) : fallback;
}

export function formatTime(value: string | number | Date | null | undefined, fallback = '–'): string {
  const date = parse(value);
  return date ? timeOnly.format(date) : fallback;
}

export function formatDayMonth(value: string | number | Date | null | undefined, fallback = '–'): string {
  const date = parse(value);
  return date ? dayMonth.format(date) : fallback;
}

export function formatDayHour(value: string | number | Date | null | undefined, fallback = '–'): string {
  const date = parse(value);
  return date ? hourOnly.format(date) : fallback;
}

/** `yyyy-mm-dd` of an instant in Asia/Ho_Chi_Minh (for <input type="date"> and analytics ranges). */
export function toLocalISODate(value: string | number | Date): string {
  const date = parse(value) ?? new Date();
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: TIME_ZONE, year: 'numeric', month: '2-digit', day: '2-digit' }).format(date);
  return parts;
}

/** "mm:ss" until `target`; negative remainders clamp to 00:00. */
export function formatCountdown(target: string | number | Date | null | undefined, now: number = Date.now()): string {
  const date = parse(target);
  if (!date) return '--:--';
  const total = Math.max(0, Math.floor((date.getTime() - now) / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const mm = String(m).padStart(2, '0');
  const ss = String(s).padStart(2, '0');
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}
