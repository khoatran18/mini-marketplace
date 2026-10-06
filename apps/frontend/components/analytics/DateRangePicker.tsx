'use client';

import { periodLabels, type Period, type RangeValue } from '../../lib/analytics';
import { toLocalISODate } from '../../lib/format';

export function DateRangePicker({ value, onChange }: { value: RangeValue; onChange: (value: RangeValue) => void }) {
  const today = toLocalISODate(new Date());
  return (
    <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Khoảng thời gian">
      {(Object.keys(periodLabels) as Exclude<Period, 'custom'>[]).map((period) => (
        <button
          key={period}
          type="button"
          aria-pressed={value.period === period}
          onClick={() => onChange({ period })}
          className={`rounded-full px-4 py-1.5 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand ${
            value.period === period ? 'bg-brand-soft text-brand' : 'bg-surface2 text-muted hover:text-text'
          }`}
        >
          {periodLabels[period]}
        </button>
      ))}
      <button
        type="button"
        aria-pressed={value.period === 'custom'}
        onClick={() => onChange({ period: 'custom', from: value.from ?? today, to: value.to ?? today })}
        className={`rounded-full px-4 py-1.5 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand ${
          value.period === 'custom' ? 'bg-brand-soft text-brand' : 'bg-surface2 text-muted hover:text-text'
        }`}
      >
        Tuỳ chọn
      </button>
      {value.period === 'custom' ? (
        <span className="flex flex-wrap items-center gap-2 text-sm">
          <input type="date" aria-label="Từ ngày" className="w-auto py-1" max={value.to ?? today} value={value.from ?? ''} onChange={(event) => onChange({ ...value, from: event.target.value })} />
          <span className="text-muted">→</span>
          <input type="date" aria-label="Đến ngày" className="w-auto py-1" max={today} min={value.from} value={value.to ?? ''} onChange={(event) => onChange({ ...value, to: event.target.value })} />
        </span>
      ) : null}
      <span className="text-xs text-muted">Múi giờ cố định: Asia/Ho_Chi_Minh (UTC+7)</span>
    </div>
  );
}
