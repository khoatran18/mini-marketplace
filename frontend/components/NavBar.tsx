'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useAuth } from './auth/AuthProvider';
import { useCart } from './cart/CartProvider';
import type { Role } from '../lib/types';

interface NavLink {
  href: string;
  label: string;
  requiresAuth: boolean;
  hiddenForRoles?: Role[];
}

const baseLinks: NavLink[] = [
  { href: '/', label: 'Trang chủ', requiresAuth: false },
  { href: '/products', label: 'Sản phẩm', requiresAuth: false },
  {
    href: '/products/mine',
    label: 'Sản phẩm của tôi',
    requiresAuth: true,
    hiddenForRoles: ['buyer']
  },
  {
    href: '/orders',
    label: 'Đơn hàng',
    requiresAuth: true,
    hiddenForRoles: ['seller_admin', 'seller_employee']
  },
  { href: '/dashboard', label: 'Bảng điều khiển', requiresAuth: true, hiddenForRoles: ['buyer'] }
];

export function NavBar() {
  const pathname = usePathname();
  const { accessToken, username, role, logout } = useAuth();
  const { totalQuantity, clearCart } = useCart();

  const handleLogout = () => {
    clearCart();
    logout();
  };

  const normalizedRole = role ?? null;

  return (
    <header className="mx-auto flex w-full max-w-5xl items-center justify-between px-4 py-5 md:px-8">
      <Link href="/" className="text-xl font-bold text-slate-900">
        Mini Marketplace
      </Link>
      <nav className="flex flex-wrap items-center gap-3 text-sm font-medium text-slate-700">
        {baseLinks
          .filter((link) => {
            if (link.requiresAuth && !accessToken) {
              return false;
            }
            if (
              normalizedRole &&
              link.hiddenForRoles &&
              link.hiddenForRoles.length > 0 &&
              link.hiddenForRoles.includes(normalizedRole)
            ) {
              return false;
            }
            return true;
          })
          .map((link) => {
            const isActive = pathname === link.href;
            return (
              <Link
                key={link.href}
                href={link.href}
                className={`rounded-full px-4 py-2 transition ${
                  isActive
                    ? 'bg-indigo-100 font-semibold text-indigo-900'
                    : 'text-slate-700 hover:bg-slate-100'
                }`}
              >
                {link.label}
              </Link>
            );
          })}
        <Link
          href="/cart"
          className={`flex items-center gap-2 rounded-full px-4 py-2 font-semibold transition ${
            pathname === '/cart' ? 'bg-pink-100 text-pink-700' : 'bg-pink-50 text-pink-700 hover:bg-pink-100'
          }`}
        >
          Giỏ hàng
          <span className="flex min-w-6 items-center justify-center rounded-full bg-pink-600 px-2 text-xs font-semibold text-white">
            {totalQuantity}
          </span>
        </Link>
        {accessToken ? (
          <div className="flex items-center gap-3 text-sm">
            <span className="font-semibold text-slate-800">
              Xin chào, {username}
              {role ? ` (${role})` : ''}
            </span>
            <button
              type="button"
              className="rounded-xl bg-indigo-600 px-4 py-2 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600"
              onClick={handleLogout}
            >
              Đăng xuất
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-3">
            <Link
              href="/login"
              className="rounded-xl px-4 py-2 font-semibold text-indigo-600 transition hover:bg-indigo-50"
            >
              Đăng nhập
            </Link>
            <Link
              href="/register"
              className="rounded-xl px-4 py-2 font-semibold text-indigo-600 transition hover:bg-indigo-50"
            >
              Đăng ký
            </Link>
          </div>
        )}
      </nav>
    </header>
  );
}
