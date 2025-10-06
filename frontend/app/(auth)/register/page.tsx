'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '../../../components/auth/AuthProvider';
import type { RegisterInput } from '../../../lib/types';

const roles = ['buyer', 'seller'];

export default function RegisterPage() {
  const router = useRouter();
  const { register } = useAuth();
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
      setMessage(response.message ?? 'Đăng ký thành công. Vui lòng đăng nhập.');
      setTimeout(() => {
        router.push('/(auth)/login');
      }, 800);
    } catch (err) {
      setMessage((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="card" onSubmit={handleSubmit} style={{ maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ margin: 0 }}>Tạo tài khoản</h1>
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
          onChange={(event) => setFormState((prev) => ({ ...prev, role: event.target.value }))}
        >
          {roles.map((role) => (
            <option key={role} value={role}>
              {role}
            </option>
          ))}
        </select>
      </label>
      <button className="primary" type="submit" disabled={loading}>
        {loading ? 'Đang xử lý...' : 'Đăng ký'}
      </button>
      {message ? <p style={{ margin: 0 }}>{message}</p> : null}
    </form>
  );
}
