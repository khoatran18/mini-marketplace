import type { ServiceStatus } from './types';

export type NodeStatus = ServiceStatus | 'unknown';

export const STATUS_LABEL: Record<NodeStatus, string> = {
  ready: 'Sẵn sàng',
  degraded: 'Suy giảm',
  not_ready: 'Chưa sẵn sàng',
  unreachable: 'Không kết nối được',
  unknown: 'Không có số liệu'
};

/** Dot / border colours as Tailwind classes (theme tokens, so they follow light/dark). */
export const STATUS_DOT: Record<NodeStatus, string> = {
  ready: 'bg-success',
  degraded: 'bg-warning',
  not_ready: 'bg-danger',
  unreachable: 'bg-danger',
  unknown: 'bg-muted/50'
};

export const STATUS_BADGE: Record<NodeStatus, string> = {
  ready: 'bg-success-soft text-success',
  degraded: 'bg-warning-soft text-warning',
  not_ready: 'bg-danger-soft text-danger',
  unreachable: 'bg-danger-soft text-danger',
  unknown: 'bg-surface2 text-muted'
};

export function isBad(status: NodeStatus): boolean {
  return status === 'not_ready' || status === 'unreachable';
}

export function formatUptime(seconds?: number): string {
  if (seconds === undefined) return '–';
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`;
}

export function formatRate(rate?: number): string {
  if (rate === undefined || !Number.isFinite(rate)) return '–';
  if (rate === 0) return '0/s';
  return rate < 10 ? `${rate.toFixed(2)}/s` : `${rate.toFixed(1)}/s`;
}

export function formatMs(seconds?: number): string {
  if (seconds === undefined || !Number.isFinite(seconds)) return '–';
  const ms = seconds * 1000;
  return ms < 10 ? `${ms.toFixed(1)} ms` : `${Math.round(ms)} ms`;
}
