import type { ReactNode } from 'react';

/** The four data states of a region (07-ui-design.md §1): loading, empty, error, stale. */
export function LoadingBlock({ label = 'Đang tải…' }: { label?: string }) {
  return (
    <p role="status" aria-live="polite" className="card text-sm text-muted">
      {label}
    </p>
  );
}

export function ErrorBlock({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="card flex flex-wrap items-center justify-between gap-3 border-danger/30 bg-danger-soft text-sm font-medium text-danger">
      <span>
        <strong className="font-semibold">Lỗi:</strong> {message}
      </span>
      {onRetry ? (
        <button type="button" className="btn" onClick={onRetry}>
          Thử lại
        </button>
      ) : null}
    </div>
  );
}

export function EmptyBlock({ children }: { children: ReactNode }) {
  return <div className="card text-sm text-muted">{children}</div>;
}

export function Notice({ tone, children }: { tone: 'success' | 'error' | 'info' | 'warning'; children: ReactNode }) {
  const cls = {
    success: 'border-success/30 bg-success-soft text-success',
    error: 'border-danger/30 bg-danger-soft text-danger',
    info: 'border-info/30 bg-info-soft text-info',
    warning: 'border-warning/30 bg-warning-soft text-warning'
  }[tone];
  return (
    <div role={tone === 'error' ? 'alert' : 'status'} aria-live="polite" className={`rounded-xl border px-4 py-3 text-sm font-medium ${cls}`}>
      {children}
    </div>
  );
}

/** Label for anything simulated (payments, shipping): always visible, never subtle. */
export function SimulatedBadge({ children = 'MÔ PHỎNG' }: { children?: ReactNode }) {
  return <span className="rounded-md border border-warning/40 bg-warning-soft px-1.5 py-0.5 text-[11px] font-bold tracking-wide text-warning">{children}</span>;
}
