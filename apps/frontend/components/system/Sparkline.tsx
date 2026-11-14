/** Minimal inline SVG line (no chart dependency). `values` oldest → newest. */
export function Sparkline({ values, label, className = 'text-brand' }: { values: number[]; label: string; className?: string }) {
  if (values.length < 2) {
    return <p className="text-xs text-muted">Đang thu thập… ({values.length}/2 mẫu)</p>;
  }
  const w = 220;
  const h = 44;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const points = values.map((v, i) => `${((i / (values.length - 1)) * w).toFixed(1)},${(h - 4 - ((v - min) / span) * (h - 8)).toFixed(1)}`).join(' ');
  return (
    <svg viewBox={`0 0 ${w} ${h}`} role="img" aria-label={label} className={`h-11 w-full ${className}`} preserveAspectRatio="none">
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
