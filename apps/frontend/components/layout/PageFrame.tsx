'use client';

import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import { ChatWidget } from '../ai/ChatWidget';
import { NavBar } from '../NavBar';

/** Consoles bring their own shell (sidebar + top bar) and use the full width; the storefront keeps a centred container. */
export function isConsolePath(pathname: string): boolean {
  return pathname === '/admin' || pathname.startsWith('/admin/') || pathname === '/seller' || pathname.startsWith('/seller/');
}

export function PageFrame({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (isConsolePath(pathname)) {
    return <main className="min-h-screen">{children}</main>;
  }
  return (
    <div className="storefront-bg min-h-screen">
      <NavBar />
      <main className="page">{children}</main>
      <ChatWidget mode="floating" />
    </div>
  );
}
