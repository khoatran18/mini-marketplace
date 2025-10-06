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
    <form className="card" onSubmit={handleSubmit} style={{ maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ margin: 0 }}>Đăng nhập</h1>
      <p style={{ margin: 0, color: '#6b7280' }}>
        Sử dụng thông tin tài khoản được tạo thông qua API Gateway.
      </p>
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
          onChange={(event) =>
            setFormState((prev) => ({ ...prev, role: event.target.value as Role }))
          }
        >
          {roles.map((role) => (
            <option key={role} value={role}>
              {role}
            </option>
          ))}
        </select>
      </label>
      <button className="primary" type="submit" disabled={loading}>
        {loading ? 'Đang đăng nhập...' : 'Đăng nhập'}
      </button>
      {error ? <p style={{ color: '#b91c1c', margin: 0 }}>{error}</p> : null}
    </form>
  );
}
