'use client';

import Link from 'next/link';
import { ReactNode } from 'react';
import { useAuth } from './auth/AuthProvider';
import type { Role } from '../lib/types';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  allowedRoles?: Role[];
}

export function ProtectedContent({ children, fallback, allowedRoles }: Props) {
  const { accessToken, role } = useAuth();

  if (!accessToken) {
    return (
      <div className="card" style={{ textAlign: 'center' }}>
        {fallback ?? (
          <>
            <h2>Yêu cầu đăng nhập</h2>
            <p>Vui lòng đăng nhập để sử dụng tính năng này.</p>
            <div style={{ display: 'flex', justifyContent: 'center', gap: '1rem' }}>
              <Link href="/login" style={{ color: '#4338ca', fontWeight: 600 }}>
                Đăng nhập
              </Link>
              <Link href="/register" style={{ color: '#4338ca', fontWeight: 600 }}>
                Đăng ký tài khoản
              </Link>
            </div>
          </>
        )}
      </div>
    );
  }

  if (allowedRoles && allowedRoles.length > 0 && (!role || !allowedRoles.includes(role))) {
    return (
      <div className="card" style={{ textAlign: 'center' }}>
        <h2>Bạn không có quyền truy cập</h2>
        <p>Vui lòng đăng nhập với tài khoản có quyền phù hợp để sử dụng tính năng này.</p>
      </div>
    );
  }

  return <>{children}</>;
}
