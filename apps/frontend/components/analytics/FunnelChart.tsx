import type { FunnelStep } from '../../lib/analytics';
import { formatNumber, formatPercent } from '../../lib/format';

/** Horizontal bars, each relative to the first step, with the step-to-step conversion. */
export function FunnelChart({ steps }: { steps: FunnelStep[] }) {
  if (steps.length === 0) return null;
  const top = Math.max(steps[0].value, 1);
  return (
    <ol className="grid gap-3" aria-label="Phễu chuyển đổi">
      {steps.map((step, index) => {
        const previous = index > 0 ? steps[index - 1].value : null;
        return (
          <li key={step.label} className="grid gap-1">
            <div className="flex flex-wrap items-baseline justify-between gap-2 text-sm">
              <span className="font-semibold text-text">{step.label}</span>
              <span className="text-text">
                {formatNumber(step.value)}
                {previous ? <span className="ml-2 text-xs text-muted">({formatPercent(step.value / previous)} so với bước trước)</span> : null}
              </span>
            </div>
            <div className="h-3 overflow-hidden rounded-full bg-surface2" role="presentation">
              <div className="h-full rounded-full bg-brand-solid" style={{ width: `${Math.max(2, (step.value / top) * 100)}%` }} />
            </div>
          </li>
        );
      })}
    </ol>
  );
}
