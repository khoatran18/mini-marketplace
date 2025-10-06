'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useAuth } from './auth/AuthProvider';

const navLinks = [
  { href: '/', label: 'Trang chủ' },
  { href: '/products', label: 'Sản phẩm' },
  { href: '/dashboard', label: 'Bảng điều khiển' }
];

export function NavBar() {
  const pathname = usePathname();
  const { accessToken, username, logout } = useAuth();

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
        {navLinks.map((link) => {
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
        {accessToken ? (
          <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center' }}>
            <span style={{ fontWeight: 600 }}>Xin chào, {username}</span>
            <button className="primary" onClick={logout} style={{ padding: '0.5rem 1.25rem' }}>
              Đăng xuất
            </button>
          </div>
        ) : (
          <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center' }}>
            <Link
              href="/(auth)/login"
              style={{ padding: '0.5rem 1.25rem', fontWeight: 600, color: '#4338ca' }}
            >
              Đăng nhập
            </Link>
            <Link
              href="/(auth)/register"
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
