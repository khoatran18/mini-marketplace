import type { ReactNode } from 'react';
import { Icon } from './Icon';

/** Page heading with a small icon chip. */
export function PageTitle({ icon, children }: { icon: string; children: ReactNode }) {
  return (
    <h1 className="flex items-center gap-2.5 text-2xl font-bold text-text">
      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-brand-soft text-brand">
        <Icon name={icon} className="h-5 w-5" />
      </span>
      {children}
    </h1>
  );
}
