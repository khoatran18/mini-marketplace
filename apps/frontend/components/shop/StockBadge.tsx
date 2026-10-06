import { StatusBadge } from '../ui/StatusBadge';

/** Public stock level: none | low | ok (inventory above 20 is shown as "20+" by the gateway). */
export function StockBadge({ level, inventory }: { level?: string | null; inventory?: number }) {
  const showCount = level === 'low' && typeof inventory === 'number' && inventory > 0;
  return (
    <span className="inline-flex items-center gap-2">
      <StatusBadge kind="stock" value={level ?? (inventory === 0 ? 'none' : 'ok')} />
      {showCount ? <span className="text-xs text-muted">Chỉ còn {inventory}</span> : null}
    </span>
  );
}
