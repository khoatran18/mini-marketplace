'use client';

import Link from 'next/link';
import { useAuth } from '../components/auth/AuthProvider';

/** Hero buttons: the sign-up shortcut only shows to visitors who are not signed in. */
export function HeroActions() {
  const { accessToken } = useAuth();
  return (
    <div className="flex flex-wrap gap-3">
      <Link href="/products" className="rounded-xl bg-white px-5 py-2 font-semibold text-teal-800 shadow-sm transition hover:bg-teal-50">
        Xem sản phẩm
      </Link>
      {accessToken ? null : (
        <Link href="/register" className="rounded-xl border border-white/50 px-5 py-2 font-semibold text-white transition hover:bg-white/10">
          Tạo tài khoản
        </Link>
      )}
    </div>
  );
}
