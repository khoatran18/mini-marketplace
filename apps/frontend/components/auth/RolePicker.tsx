import { ROLE_HINT, ROLE_LABEL } from '../../lib/roles';

/** Radio-card group for choosing a role. Generic over the role union so login and register can share it. */
export function RolePicker<R extends string>({ value, options, onChange, legend = 'Bạn là', labels }: { value: R; options: R[]; onChange: (role: R) => void; legend?: string; labels?: Partial<Record<R, string>> }) {
  return (
    <fieldset className="grid gap-2">
      <legend className="mb-1 text-sm font-semibold text-muted">{legend}</legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {options.map((role) => {
          const selected = role === value;
          const label = labels?.[role] ?? ROLE_LABEL[role as keyof typeof ROLE_LABEL] ?? role;
          const hint = labels ? undefined : ROLE_HINT[role as keyof typeof ROLE_HINT];
          return (
            <label
              key={role}
              className={`flex cursor-pointer items-center gap-3 rounded-xl border px-3 py-2.5 transition [&:last-child:nth-child(odd)]:sm:col-span-2 focus-within:outline focus-within:outline-2 focus-within:outline-brand ${
                selected ? 'border-brand bg-brand-soft' : 'border-line bg-surface hover:bg-surface2'
              }`}
            >
              <input type="radio" name="role" value={role} checked={selected} onChange={() => onChange(role)} className="sr-only" />
              <span aria-hidden="true" className={`grid h-4 w-4 shrink-0 place-items-center rounded-full border ${selected ? 'border-brand' : 'border-muted/60'}`}>
                {selected ? <span className="h-2 w-2 rounded-full bg-brand" /> : null}
              </span>
              <span className="grid text-left">
                <span className={`text-sm font-semibold ${selected ? 'text-brand' : 'text-text'}`}>{label}</span>
                {hint ? <span className="text-xs font-normal text-muted">{hint}</span> : null}
              </span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
