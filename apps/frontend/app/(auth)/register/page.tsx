'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { AuthShell } from '../../../components/auth/AuthShell';
import { PasswordInput } from '../../../components/auth/PasswordInput';
import { RolePicker } from '../../../components/auth/RolePicker';
import { useAuth } from '../../../components/auth/AuthProvider';
import type { RegisterInput, RegisterableRole } from '../../../lib/types';

const roles: RegisterableRole[] = ['buyer', 'seller_admin', 'seller_employee'];

export default function RegisterPage() {
  const router = useRouter();
  const { register, login } = useAuth();
  const [formState, setFormState] = useState<RegisterInput>({ username: '', password: '', role: 'buyer' });
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setMessage(null);
    setFailed(false);
    try {
      const response = await register(formState);
      setMessage(response.message ?? 'Đăng ký thành công. Vui lòng hoàn thiện thông tin cá nhân.');
      await login({ username: formState.username, password: formState.password });
      router.push('/profile?setup=1');
    } catch (err) {
      setFailed(true);
      setMessage((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell
      title="Tạo tài khoản"
      subtitle="Mỗi tên đăng nhập gắn với một vai trò. Muốn đổi vai trò, hãy tạo tài khoản mới."
      footer={
        <>
          Đã có tài khoản?{' '}
          <Link href="/login" className="font-semibold text-brand hover:underline">
            Đăng nhập
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
          <PasswordInput value={formState.password} autoComplete="new-password" onChange={(password) => setFormState((p) => ({ ...p, password }))} />
        </label>
        <RolePicker legend="Vai trò (không đổi được sau này)" value={formState.role} options={roles} onChange={(role) => setFormState((p) => ({ ...p, role }))} />
        {message ? (
          <p role={failed ? 'alert' : 'status'} className={`rounded-lg px-3 py-2 text-sm font-medium ${failed ? 'bg-danger-soft text-danger' : 'bg-success-soft text-success'}`}>
            {message}
          </p>
        ) : null}
        <button type="submit" disabled={loading} className="btn-primary h-11 w-full text-base">
          {loading ? 'Đang xử lý...' : 'Đăng ký'}
        </button>
      </form>
    </AuthShell>
  );
}
