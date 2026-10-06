'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import type { Role } from '../../lib/types';
import { ProtectedContent } from '../ProtectedContent';
import { ThemeToggle } from './ThemeToggle';

export interface ConsoleLink {
  href: string;
  label: string;
  exact?: boolean;
}

interface Props {
  title: string;
  roles: Role[];
  links: ConsoleLink[];
  children: ReactNode;
}

/** Sidebar + content shell shared by the seller and admin consoles. Access is enforced by the gateway; the guard only hides the UI. */
export function ConsoleLayout({ title, roles, links, children }: Props) {
  const pathname = usePathname();
  const active = (link: ConsoleLink) => (link.exact ? pathname === link.href : pathname === link.href || pathname.startsWith(`${link.href}/`));
  return (
    <ProtectedContent allowedRoles={roles}>
      <div className="grid gap-6 lg:grid-cols-[220px_1fr]">
        <aside className="grid content-start gap-3 lg:sticky lg:top-4 lg:self-start">
          <div className="card flex items-center justify-between gap-2 p-3">
            <span className="text-sm font-bold text-text">{title}</span>
            <ThemeToggle />
          </div>
          <nav aria-label={title} className="card flex gap-1 overflow-x-auto p-2 lg:grid">
            {links.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                aria-current={active(link) ? 'page' : undefined}
                className={`whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition ${active(link) ? 'bg-brand-soft font-semibold text-brand' : 'text-muted hover:bg-surface2 hover:text-text'}`}
              >
                {link.label}
              </Link>
            ))}
          </nav>
        </aside>
        <div className="grid min-w-0 content-start gap-6">{children}</div>
      </div>
    </ProtectedContent>
  );
}
