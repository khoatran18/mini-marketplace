'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { AuthShell } from '../../../components/auth/AuthShell';
import { PasswordInput } from '../../../components/auth/PasswordInput';
import { RolePicker } from '../../../components/auth/RolePicker';
import { useAuth } from '../../../components/auth/AuthProvider';
import type { RegisterInput, RegisterableRole } from '../../../lib/types';

const roles: RegisterableRole[] = ['buyer', 'seller_admin'];
const roleLabels: Partial<Record<RegisterableRole, string>> = { buyer: 'Người mua', seller_admin: 'Người bán' };

function Check({ ok, okText, badText }: { ok: boolean; okText: string; badText: string }) {
  return (
    <span role="status" className={`mt-1 block text-xs font-medium ${ok ? 'text-success' : 'text-danger'}`}>
      {ok ? '✓ ' : '✕ '}
      {ok ? okText : badText}
    </span>
  );
}

export default function RegisterPage() {
  const router = useRouter();
  const { register, login } = useAuth();
  const [formState, setFormState] = useState<RegisterInput>({ username: '', password: '', role: 'buyer' });
  const [confirm, setConfirm] = useState('');
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  const passwordOk = formState.password.length >= 8;
  const matchOk = confirm === formState.password;

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!passwordOk || !matchOk) return;
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
      subtitle="Vai trò không đổi được sau này; muốn đổi hãy tạo tài khoản mới."
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
          {formState.password ? <Check ok={passwordOk} okText="Mật khẩu hợp lệ" badText="Cần ít nhất 8 ký tự" /> : null}
        </label>
        <label>
          Nhập lại mật khẩu
          <PasswordInput value={confirm} autoComplete="new-password" onChange={setConfirm} />
          {confirm ? <Check ok={matchOk} okText="Mật khẩu khớp" badText="Mật khẩu chưa khớp" /> : null}
        </label>
        <RolePicker legend="Bạn là" labels={roleLabels} value={formState.role as RegisterableRole} options={roles} onChange={(role) => setFormState((p) => ({ ...p, role }))} />
        {message ? (
          <p role={failed ? 'alert' : 'status'} className={`rounded-lg px-3 py-2 text-sm font-medium ${failed ? 'bg-danger-soft text-danger' : 'bg-success-soft text-success'}`}>
            {message}
          </p>
        ) : null}
        <button type="submit" disabled={loading || !passwordOk || !matchOk} className="btn-primary h-11 w-full text-base">
          {loading ? 'Đang xử lý...' : 'Đăng ký'}
        </button>
      </form>
    </AuthShell>
  );
}
