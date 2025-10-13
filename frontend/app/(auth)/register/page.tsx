'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '../../../components/auth/AuthProvider';
import type { RegisterInput, Role } from '../../../lib/types';

const roles: Role[] = ['buyer', 'seller_admin', 'seller_employee'];

export default function RegisterPage() {
  const router = useRouter();
  const { register, login } = useAuth();
  const [formState, setFormState] = useState<RegisterInput>({
    username: '',
    password: '',
    role: 'buyer'
  });
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setMessage(null);
    try {
      const response = await register(formState);
      setMessage(response.message ?? 'Đăng ký thành công. Vui lòng hoàn thiện thông tin cá nhân.');
      await login({ ...formState });
      router.push('/profile?setup=1');
    } catch (err) {
      setMessage((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="card mx-auto grid max-w-md gap-4" onSubmit={handleSubmit}>
      <h1 className="text-3xl font-bold text-slate-900">Tạo tài khoản</h1>
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
        {loading ? 'Đang xử lý...' : 'Đăng ký'}
      </button>
      {message ? <p className="text-sm text-slate-700">{message}</p> : null}
    </form>
  );
}
