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
    <header
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '1.25rem 2rem',
        maxWidth: '1100px',
        margin: '0 auto'
      }}
    >
      <Link href="/" style={{ fontSize: '1.25rem', fontWeight: 700 }}>
        Mini Marketplace
      </Link>
      <nav style={{ display: 'flex', gap: '1rem', alignItems: 'center' }}>
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
                style={{
                  padding: '0.5rem 1rem',
                  borderRadius: '999px',
                  backgroundColor: isActive ? '#e0e7ff' : 'transparent',
                  color: isActive ? '#312e81' : '#1f2937',
                  fontWeight: isActive ? 700 : 500
                }}
              >
                {link.label}
              </Link>
            );
          })}
        <Link
          href="/cart"
          style={{
            padding: '0.5rem 1rem',
            borderRadius: '999px',
            backgroundColor: pathname === '/cart' ? '#fce7f3' : '#f9fafb',
            color: '#be185d',
            fontWeight: 600,
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem'
          }}
        >
          Giỏ hàng
          <span
            style={{
              backgroundColor: '#be185d',
              color: 'white',
              borderRadius: '999px',
              padding: '0.1rem 0.6rem',
              fontSize: '0.75rem',
              minWidth: '1.5rem',
              textAlign: 'center'
            }}
          >
            {totalQuantity}
          </span>
        </Link>
        {accessToken ? (
          <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center' }}>
            <span style={{ fontWeight: 600 }}>
              Xin chào, {username}
              {role ? ` (${role})` : ''}
            </span>
            <button className="primary" onClick={handleLogout} style={{ padding: '0.5rem 1.25rem' }}>
              Đăng xuất
            </button>
          </div>
        ) : (
          <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center' }}>
            <Link
              href="/login"
              style={{ padding: '0.5rem 1.25rem', fontWeight: 600, color: '#4338ca' }}
            >
              Đăng nhập
            </Link>
            <Link
              href="/register"
              style={{ padding: '0.5rem 1.25rem', fontWeight: 600, color: '#4338ca' }}
            >
              Đăng ký
            </Link>
          </div>
        )}
      </nav>
    </header>
  );
}
