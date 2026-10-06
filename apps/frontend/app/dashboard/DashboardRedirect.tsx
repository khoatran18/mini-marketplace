'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useEffect } from 'react';
import { useAuth } from '../../components/auth/AuthProvider';

const homes = { buyer: '/orders', seller_admin: '/seller', seller_employee: '/seller', admin: '/admin' } as const;

export function DashboardRedirect() {
  const router = useRouter();
  const { ready, role } = useAuth();

  useEffect(() => {
    if (!ready) return;
    router.replace(role ? homes[role] : '/login');
  }, [ready, role, router]);

  return (
    <p className="text-sm text-muted">
      Đang chuyển hướng… <Link href="/" className="font-semibold text-brand">Về trang chủ</Link>
    </p>
  );
}
