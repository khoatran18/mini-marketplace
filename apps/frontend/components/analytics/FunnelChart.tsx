import { funnelLabels, type FunnelStep } from '../../lib/analytics';
import { formatNumber, formatPercent } from '../../lib/format';

/** Horizontal bars, each relative to the first measurable step, with the step-to-step conversion. A null count = not measurable. */
export function FunnelChart({ steps }: { steps: FunnelStep[] }) {
  if (steps.length === 0) return null;
  const top = Math.max(steps.find((step) => step.count !== null)?.count ?? 0, 1);
  return (
    <ol className="grid gap-3" aria-label="Phễu chuyển đổi">
      {steps.map((step, index) => {
        const previous = index > 0 ? steps[index - 1].count : null;
        return (
          <li key={step.step} className="grid gap-1">
            <div className="flex flex-wrap items-baseline justify-between gap-2 text-sm">
              <span className="font-semibold text-text">{funnelLabels[step.step] ?? step.step}</span>
              <span className="text-text">
                {step.count === null ? <span className="text-muted">Không đo được</span> : formatNumber(step.count)}
                {step.count !== null && previous ? <span className="ml-2 text-xs text-muted">({formatPercent(step.count / previous)} so với bước trước)</span> : null}
              </span>
            </div>
            <div className="h-3 overflow-hidden rounded-full bg-surface2" role="presentation">
              <div className="h-full rounded-full bg-brand-solid" style={{ width: `${step.count === null ? 0 : Math.max(2, (step.count / top) * 100)}%` }} />
            </div>
          </li>
        );
      })}
    </ol>
  );
}
