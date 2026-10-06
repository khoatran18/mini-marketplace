import { formatByKind, formatByKindDelta, type Kpi } from '../../lib/analytics';
import { DefinitionTooltip } from './DefinitionTooltip';

export function KpiRow({ kpis, definitions = {} }: { kpis: Kpi[]; definitions?: Record<string, string> }) {
  if (kpis.length === 0) return null;
  return (
    <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      {kpis.map((kpi) => {
        const delta = kpi.delta;
        // for cancel/refund-like rates a decrease is good; keep colours neutral-by-meaning
        const goodWhenDown = /cancel|refund|fail|error|lag|reject|stock/i.test(kpi.key);
        const positive = delta !== null && delta !== undefined && (goodWhenDown ? delta < 0 : delta > 0);
        return (
          <div key={kpi.key} className="rounded-xl border border-line bg-surface2 p-4">
            <dt className="flex items-center gap-1 text-xs font-semibold uppercase tracking-wide text-muted">
              {kpi.label}
              {definitions[kpi.key] ? <DefinitionTooltip>{definitions[kpi.key]}</DefinitionTooltip> : null}
            </dt>
            <dd className="mt-1 text-2xl font-bold text-text">{formatByKind(kpi.value, kpi.kind)}</dd>
            {delta !== null && delta !== undefined ? (
              <dd className={`text-xs font-semibold ${delta === 0 ? 'text-muted' : positive ? 'text-success' : 'text-danger'}`}>
                {formatByKindDelta(delta)} so với kỳ trước
              </dd>
            ) : null}
          </div>
        );
      })}
    </dl>
  );
}
