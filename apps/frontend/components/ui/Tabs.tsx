export interface TabItem {
  value: string;
  label: string;
  count?: number;
}

export function Tabs({ items, value, onChange, label }: { items: TabItem[]; value: string; onChange: (value: string) => void; label: string }) {
  return (
    <div role="tablist" aria-label={label} className="flex flex-wrap gap-2">
      {items.map((item) => (
        <button
          key={item.value}
          type="button"
          role="tab"
          aria-selected={value === item.value}
          onClick={() => onChange(item.value)}
          className={`rounded-full px-4 py-1.5 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand ${
            value === item.value ? 'bg-brand-soft text-brand' : 'bg-surface2 text-muted hover:text-text'
          }`}
        >
          {item.label}
          {item.count !== undefined ? <span className="ml-1.5 text-xs opacity-80">({item.count})</span> : null}
        </button>
      ))}
    </div>
  );
}
