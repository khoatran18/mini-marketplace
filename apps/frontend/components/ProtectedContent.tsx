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
  const { accessToken, role, ready } = useAuth();

  if (!ready) {
    return <p className="text-sm text-muted">Đang tải…</p>;
  }

  if (!accessToken) {
    return (
      <div className="card text-center">
        {fallback ?? (
          <>
            <h2 className="text-xl font-semibold text-text">Yêu cầu đăng nhập</h2>
            <p className="text-sm text-muted">Vui lòng đăng nhập để sử dụng tính năng này.</p>
            <div className="mt-4 flex items-center justify-center gap-4 text-sm font-semibold text-brand">
              <Link href="/login" className="rounded-xl px-4 py-2 transition hover:bg-brand-soft">
                Đăng nhập
              </Link>
              <Link href="/register" className="rounded-xl px-4 py-2 transition hover:bg-brand-soft">
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
      <div className="card text-center">
        <h2 className="text-xl font-semibold text-text">Bạn không có quyền truy cập</h2>
        <p className="text-sm text-muted">
          Vui lòng đăng nhập với tài khoản có quyền phù hợp để sử dụng tính năng này.
        </p>
      </div>
    );
  }

  return <>{children}</>;
}
