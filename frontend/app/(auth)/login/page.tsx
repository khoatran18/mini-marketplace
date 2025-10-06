'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '../../../components/auth/AuthProvider';
import type { LoginInput, Role } from '../../../lib/types';

const roles: Role[] = ['buyer', 'seller_admin', 'seller_employee'];

export default function LoginPage() {
  const router = useRouter();
  const { login } = useAuth();
  const [formState, setFormState] = useState<LoginInput>({
    username: '',
    password: '',
    role: 'buyer'
  });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await login(formState);
      router.push('/dashboard');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="card mx-auto grid max-w-md gap-4" onSubmit={handleSubmit}>
      <h1 className="text-3xl font-bold text-slate-900">Đăng nhập</h1>
      <p className="text-sm text-slate-600">Sử dụng thông tin tài khoản được tạo thông qua API Gateway.</p>
      <label>
        Tên đăng nhập
        <input
          type="text"
          value={formState.username}
          onChange={(event) => setFormState((prev) => ({ ...prev, username: event.target.value }))}
          required
        />
      </label>
      <label>
        Mật khẩu
        <input
          type="password"
          value={formState.password}
          onChange={(event) => setFormState((prev) => ({ ...prev, password: event.target.value }))}
          required
        />
      </label>
      <label>
        Vai trò
        <select
          value={formState.role}
          onChange={(event) => setFormState((prev) => ({ ...prev, role: event.target.value as Role }))}
          className="w-full rounded-xl border border-slate-300 px-3 py-2 text-base shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
        >
          {roles.map((role) => (
            <option key={role} value={role}>
              {role}
            </option>
          ))}
        </select>
      </label>
      <button
        type="submit"
        disabled={loading}
        className="rounded-xl bg-indigo-600 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-70"
      >
        {loading ? 'Đang đăng nhập...' : 'Đăng nhập'}
      </button>
      {error ? <p className="text-sm font-medium text-rose-600">{error}</p> : null}
    </form>
  );
}
