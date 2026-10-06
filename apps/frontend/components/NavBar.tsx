'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useAuth } from './auth/AuthProvider';
import { useCart } from './cart/CartProvider';
import { ThemeToggle } from './layout/ThemeToggle';
import type { Role } from '../lib/types';

interface NavLink {
  href: string;
  label: string;
  requiresAuth: boolean;
  /** roles that see the link; omitted = everybody (hiding is a convenience: the gateway enforces access) */
  roles?: Role[];
  devOnly?: boolean;
}

// The one place that maps routes to roles.
const navLinks: NavLink[] = [
  { href: '/', label: 'Trang chủ', requiresAuth: false },
  { href: '/products', label: 'Sản phẩm', requiresAuth: false },
  { href: '/orders', label: 'Đơn hàng', requiresAuth: true, roles: ['buyer'] },
  { href: '/seller', label: 'Kênh người bán', requiresAuth: true, roles: ['seller_admin', 'seller_employee'] },
  { href: '/admin', label: 'Quản trị', requiresAuth: true, roles: ['admin'] },
  { href: '/profile', label: 'Thông tin cá nhân', requiresAuth: true, roles: ['buyer', 'seller_admin', 'seller_employee'] },
  { href: '/dev/tracking', label: 'Dev', requiresAuth: false, devOnly: true }
];

const isDev = process.env.NODE_ENV !== 'production' || process.env.NEXT_PUBLIC_APP_ENV === 'dev';

export function NavBar() {
  const pathname = usePathname();
  const router = useRouter();
  const { accessToken, username, role, logout } = useAuth();
  const { totalQuantity, clearCart } = useCart();

  const handleLogout = () => {
    clearCart();
    logout();
    router.push('/');
  };

  const showCartLink = !role || role === 'buyer';

  const isActive = (href: string) => (href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(`${href}/`));

  return (
    <header className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-5 md:px-8">
      <Link href="/" className="text-xl font-bold text-text">
        Mini Marketplace
      </Link>
      <nav aria-label="Điều hướng chính" className="flex flex-wrap items-center gap-2 text-sm font-medium text-muted">
        {navLinks
          .filter((link) => {
            if (link.devOnly && !isDev) return false;
            if (link.requiresAuth && !accessToken) return false;
            if (link.roles && (!role || !link.roles.includes(role))) return false;
            return true;
          })
          .map((link) => (
            <Link
              key={link.href}
              href={link.href}
              aria-current={isActive(link.href) ? 'page' : undefined}
              className={`rounded-full px-4 py-2 transition ${
                isActive(link.href) ? 'bg-brand-soft font-semibold text-brand' : 'text-muted hover:bg-surface2'
              }`}
            >
              {link.label}
            </Link>
          ))}
        {showCartLink ? (
          <Link
            href="/cart"
            className={`flex items-center gap-2 rounded-full px-4 py-2 font-semibold transition ${
              pathname === '/cart' ? 'bg-brand-soft text-brand' : 'bg-surface2 text-text hover:bg-brand-soft'
            }`}
          >
            Giỏ hàng
            <span className="flex min-w-6 items-center justify-center rounded-full bg-brand-solid px-2 text-xs font-semibold text-white">
              {totalQuantity}
            </span>
          </Link>
        ) : null}
        <ThemeToggle />
        {accessToken ? (
          <div className="flex items-center gap-3 text-sm">
            <span className="font-semibold text-text">
              Xin chào, {username}
              {role ? ` (${role})` : ''}
            </span>
            <button type="button" className="btn-primary" onClick={handleLogout}>
              Đăng xuất
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <Link href="/login" className="rounded-xl px-4 py-2 font-semibold text-brand transition hover:bg-brand-soft">
              Đăng nhập
            </Link>
            <Link href="/register" className="rounded-xl px-4 py-2 font-semibold text-brand transition hover:bg-brand-soft">
              Đăng ký
            </Link>
          </div>
        )}
      </nav>
    </header>
  );
}
