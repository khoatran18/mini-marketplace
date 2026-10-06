import { isObject, isScalar, labelFor, toKpis, toRows } from '../../lib/analytics';
import { DataTable } from './DataTable';
import { KpiRow } from './KpiRow';

/** Renders unknown JSON sensibly: scalars as KPI tiles, arrays as tables, nested objects as sub-sections. */
export function AutoData({ data, depth = 0 }: { data: unknown; depth?: number }) {
  if (Array.isArray(data)) {
    const rows = toRows(data);
    return rows.length > 0 ? <DataTable rows={rows} caption="Dữ liệu" /> : <p className="text-sm text-muted">Không có dữ liệu.</p>;
  }
  if (!isObject(data)) return null;
  const scalars: Record<string, unknown> = {};
  const nested: [string, unknown][] = [];
  for (const [key, value] of Object.entries(data)) {
    if (['as_of', 'tz', 'source'].includes(key)) continue;
    if (isScalar(value) || (isObject(value) && ('amount' in value || 'value' in value))) scalars[key] = value;
    else nested.push([key, value]);
  }
  return (
    <div className="grid gap-4">
      <KpiRow kpis={toKpis(scalars)} />
      {nested.map(([key, value]) => (
        <section key={key} className="grid gap-2">
          <h3 className="text-sm font-semibold text-text">{labelFor(key)}</h3>
          {depth < 2 ? <AutoData data={value} depth={depth + 1} /> : <pre className="overflow-x-auto text-xs text-muted">{JSON.stringify(value, null, 2)}</pre>}
        </section>
      ))}
    </div>
  );
}
