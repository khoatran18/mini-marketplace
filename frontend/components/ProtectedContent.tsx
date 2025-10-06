'use client';

import Link from 'next/link';
import { ReactNode } from 'react';
import { useAuth } from './auth/AuthProvider';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

export function ProtectedContent({ children, fallback }: Props) {
  const { accessToken } = useAuth();

  if (!accessToken) {
    return (
      <div className="card" style={{ textAlign: 'center' }}>
        {fallback ?? (
          <>
            <h2>Yêu cầu đăng nhập</h2>
            <p>Vui lòng đăng nhập để sử dụng tính năng này.</p>
            <div style={{ display: 'flex', justifyContent: 'center', gap: '1rem' }}>
              <Link href="/(auth)/login" style={{ color: '#4338ca', fontWeight: 600 }}>
                Đăng nhập
              </Link>
              <Link href="/(auth)/register" style={{ color: '#4338ca', fontWeight: 600 }}>
                Đăng ký tài khoản
              </Link>
            </div>
          </>
        )}
      </div>
    );
  }

  return <>{children}</>;
}
