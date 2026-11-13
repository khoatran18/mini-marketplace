'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useEffect, useState, type ReactNode } from 'react';
import type { Role } from '../../lib/types';
import { AssistantButton, ChatWidget } from '../ai/ChatWidget';
import { useAuth } from '../auth/AuthProvider';
import { useCart } from '../cart/CartProvider';
import { ProtectedContent } from '../ProtectedContent';
import { Icon } from './Icon';
import { ThemeToggle } from './ThemeToggle';

export interface ShellLink {
  href: string;
  label: string;
  icon: string;
  exact?: boolean;
  /** roles that see the link; omitted = every role allowed in the console (the gateway enforces access) */
  roles?: Role[];
}

interface Props {
  title: string;
  roles: Role[];
  links: ShellLink[];
  children: ReactNode;
}

const COLLAPSE_KEY = 'mm_console_collapsed';

/**
 * Console shell shared by the admin and seller areas: sticky top bar (breadcrumb, assistant, theme, user) and a
 * collapsible sidebar; content uses the full width. Access is enforced by the gateway; the guard only hides the UI.
 */
export function AppShell({ title, roles, links, children }: Props) {
  const pathname = usePathname();
  const router = useRouter();
  const { accessToken, username, role, logout } = useAuth();
  const { clearCart } = useCart();
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    try {
      setCollapsed(window.localStorage.getItem(COLLAPSE_KEY) === '1');
    } catch {
      // storage blocked: stay expanded
    }
  }, []);

  const toggleCollapsed = () => {
    setCollapsed((value) => {
      try {
        window.localStorage.setItem(COLLAPSE_KEY, value ? '0' : '1');
      } catch {
        // ignore
      }
      return !value;
    });
  };

  const visible = links.filter((link) => !link.roles || (role !== null && link.roles.includes(role)));
  const active = (link: ShellLink) => (link.exact ? pathname === link.href : pathname === link.href || pathname.startsWith(`${link.href}/`));
  const current = visible.find(active);

  const handleLogout = () => {
    clearCart();
    logout();
    router.push('/');
  };

  return (
    <div className="min-h-screen bg-bg">
      <header className="sticky top-0 z-30 flex h-14 items-center gap-3 bar px-3 md:px-5">
        <button
          type="button"
          onClick={toggleCollapsed}
          aria-label={collapsed ? 'Mở rộng menu' : 'Thu gọn menu'}
          aria-pressed={collapsed}
          className="hidden h-9 w-9 place-items-center rounded-lg text-muted transition hover:bg-surface2 hover:text-text md:grid"
        >
          <Icon name="menu" />
        </button>
        <Link href="/" className="whitespace-nowrap text-base font-bold tracking-tight text-text">
          Mini Marketplace
        </Link>
        <nav aria-label="Vị trí" className="hidden min-w-0 items-center gap-2 text-sm text-muted sm:flex">
          <span aria-hidden="true">/</span>
          <span>{title}</span>
          {current ? (
            <>
              <span aria-hidden="true">/</span>
              <span className="truncate font-semibold text-text">{current.label}</span>
            </>
          ) : null}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <Link href="/" className="hidden rounded-lg px-3 py-2 text-sm font-medium text-muted transition hover:bg-surface2 hover:text-text lg:inline">
            Về trang bán hàng
          </Link>
          <AssistantButton />
          <ThemeToggle />
          {accessToken ? (
            <div className="flex items-center gap-2">
              <span className="hidden text-sm text-muted md:inline">
                <span className="font-semibold text-text">{username}</span> <span className="chip align-middle">{role}</span>
              </span>
              <button type="button" className="btn h-9 px-3" onClick={handleLogout}>
                Đăng xuất
              </button>
            </div>
          ) : (
            <Link href="/login" className="btn-primary h-9 px-3">
              Đăng nhập
            </Link>
          )}
        </div>
      </header>

      <div className="flex">
        <aside
          className={`sticky top-14 hidden h-[calc(100vh-3.5rem)] shrink-0 overflow-y-auto border-r border-line bg-surface py-3 transition-[width] md:block ${collapsed ? 'w-[68px]' : 'w-60'}`}
        >
          <nav aria-label={title} className="grid gap-1 px-2">
            {visible.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                title={collapsed ? link.label : undefined}
                aria-current={active(link) ? 'page' : undefined}
                className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition ${collapsed ? 'justify-center' : ''} ${
                  active(link) ? 'bg-brand-solid font-semibold text-white shadow-sm' : 'text-muted hover:bg-surface2 hover:text-text'
                }`}
              >
                <Icon name={link.icon} />
                {collapsed ? <span className="sr-only">{link.label}</span> : <span className="truncate">{link.label}</span>}
              </Link>
            ))}
          </nav>
        </aside>

        <div className="min-w-0 flex-1">
          {/* phones: the sidebar becomes a scrolling strip */}
          <nav aria-label={`${title} (di động)`} className="flex gap-1 overflow-x-auto border-b border-line bg-surface px-3 py-2 md:hidden">
            {visible.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                aria-current={active(link) ? 'page' : undefined}
                className={`flex items-center gap-2 whitespace-nowrap rounded-lg px-3 py-1.5 text-sm font-medium ${
                  active(link) ? 'bg-brand-solid font-semibold text-white shadow-sm' : 'text-muted'
                }`}
              >
                <Icon name={link.icon} className="h-4 w-4" />
                {link.label}
              </Link>
            ))}
          </nav>
          <main className="mx-auto w-full max-w-[1600px] px-4 py-6 md:px-8 md:py-8">
            <ProtectedContent allowedRoles={roles}>
              <div className="grid min-w-0 content-start gap-6">{children}</div>
            </ProtectedContent>
          </main>
        </div>
      </div>
      <ChatWidget mode="drawer" />
    </div>
  );
}
