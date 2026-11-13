'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { AuthShell } from '../../../components/auth/AuthShell';
import { PasswordInput } from '../../../components/auth/PasswordInput';
import { useAuth } from '../../../components/auth/AuthProvider';
import type { LoginInput } from '../../../lib/types';

export default function LoginPage() {
  const router = useRouter();
  const { login } = useAuth();
  const [formState, setFormState] = useState<LoginInput>({ username: '', password: '' });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await login(formState);
      router.push('/');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell
      title="Đăng nhập"
      subtitle="Đăng nhập bằng tên đăng nhập và mật khẩu."
      footer={
        <>
          Chưa có tài khoản?{' '}
          <Link href="/register" className="font-semibold text-brand hover:underline">
            Đăng ký
          </Link>
        </>
      }
    >
      <form className="grid gap-4" onSubmit={handleSubmit}>
        <label>
          Tên đăng nhập
          <input type="text" value={formState.username} autoComplete="username" onChange={(e) => setFormState((p) => ({ ...p, username: e.target.value }))} required />
        </label>
        <label>
          Mật khẩu
          <PasswordInput value={formState.password} autoComplete="current-password" onChange={(password) => setFormState((p) => ({ ...p, password }))} />
        </label>
        {error ? (
          <p role="alert" className="rounded-lg bg-danger-soft px-3 py-2 text-sm font-medium text-danger">
            {error}
          </p>
        ) : null}
        <button type="submit" disabled={loading} className="btn-primary h-11 w-full text-base">
          {loading ? 'Đang đăng nhập...' : 'Đăng nhập'}
        </button>
      </form>
    </AuthShell>
  );
}
