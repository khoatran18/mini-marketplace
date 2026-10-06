import { DefinitionTooltip } from './DefinitionTooltip';

export interface Kpi {
  key: string;
  label: string;
  /** already formatted */
  value: string;
  /** relative change vs the previous period (fraction) */
  delta?: number | null;
  /** for cancel/refund-like metrics a decrease is the good direction */
  goodWhenDown?: boolean;
  definition?: string;
}

function formatDelta(delta: number): string {
  const sign = delta > 0 ? '▲ +' : delta < 0 ? '▼ ' : '';
  return `${sign}${(delta * 100).toFixed(1).replace('.', ',')}%`;
}

export function KpiRow({ kpis }: { kpis: Kpi[] }) {
  if (kpis.length === 0) return null;
  return (
    <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      {kpis.map((kpi) => {
        const delta = kpi.delta;
        const good = delta !== null && delta !== undefined && (kpi.goodWhenDown ? delta < 0 : delta > 0);
        return (
          <div key={kpi.key} className="rounded-xl border border-line bg-surface2 p-4">
            <dt className="flex items-center gap-1 text-xs font-semibold uppercase tracking-wide text-muted">
              {kpi.label}
              {kpi.definition ? <DefinitionTooltip>{kpi.definition}</DefinitionTooltip> : null}
            </dt>
            <dd className="mt-1 text-2xl font-bold text-text">{kpi.value}</dd>
            {delta !== null && delta !== undefined ? (
              <dd className={`text-xs font-semibold ${delta === 0 ? 'text-muted' : good ? 'text-success' : 'text-danger'}`}>{formatDelta(delta)} so với kỳ trước</dd>
            ) : null}
          </div>
        );
      })}
    </dl>
  );
}
