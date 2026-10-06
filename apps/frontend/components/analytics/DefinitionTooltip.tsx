/** ⓘ with a plain-language definition (focusable, so it works with keyboard and touch). */
export function DefinitionTooltip({ children, label = 'Định nghĩa' }: { children: string; label?: string }) {
  return (
    <span className="group relative inline-flex align-middle">
      <button type="button" aria-label={`${label}: ${children}`} className="rounded-full text-xs text-muted hover:text-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand">
        <span aria-hidden="true">ⓘ</span>
      </button>
      <span role="tooltip" className="pointer-events-none absolute left-0 top-full z-20 mt-1 hidden w-64 rounded-lg border border-line bg-surface p-2 text-xs font-normal normal-case text-text shadow-lg group-focus-within:block group-hover:block">
        {children}
      </span>
    </span>
  );
}
