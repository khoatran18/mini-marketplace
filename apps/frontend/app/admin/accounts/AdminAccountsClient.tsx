'use client';

import { useState } from 'react';
import { useAuth } from '../../../components/auth/AuthProvider';
import { PasswordInput } from '../../../components/auth/PasswordInput';
import { Icon } from '../../../components/layout/Icon';
import { PageTitle } from '../../../components/layout/PageTitle';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../../../components/ui/StateBlock';
import { createStaffAccountRequest, deleteStaffAccountRequest, listAccountsRequest, resetStaffPasswordRequest } from '../../../lib/api';
import { useApiData, useAuthedAction } from '../../../lib/hooks';
import { ROLE_LABEL } from '../../../lib/roles';
import type { AccountInfo } from '../../../lib/types';

const filters: { value: string; label: string }[] = [
  { value: '', label: 'Tất cả' },
  { value: 'admin', label: 'Quản trị' },
  { value: 'dev', label: 'Dev' },
  { value: 'seller_admin', label: 'Người bán' },
  { value: 'seller_employee', label: 'Nhân viên' },
  { value: 'buyer', label: 'Người mua' }
];

const roleTone: Record<string, string> = {
  admin: 'bg-brand-soft text-brand',
  dev: 'bg-warning-soft text-warning'
};

const isStaff = (role: string) => role === 'admin' || role === 'dev';

export function AdminAccountsClient() {
  const run = useAuthedAction();
  const { username: me } = useAuth();
  const [role, setRole] = useState('');
  const [query, setQuery] = useState('');
  const list = useApiData((token) => listAccountsRequest(token as string, { role, q: query.trim() }), [role, query]);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({ username: '', password: '', confirm: '', role: 'dev' as 'admin' | 'dev' });
  const [resetFor, setResetFor] = useState<AccountInfo | null>(null);
  const [newPw, setNewPw] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);
  const accounts = list.data?.accounts ?? [];

  const pwOk = form.password.length >= 8;
  const matchOk = form.confirm === form.password;
  const userOk = /^[a-zA-Z0-9_]{3,16}$/.test(form.username);

  const act = async (fn: (token: string) => Promise<unknown>, done: string) => {
    setBusy(true);
    setMessage(null);
    try {
      await run(fn);
      setMessage({ tone: 'success', text: done });
      list.reload();
      return true;
    } catch (err) {
      setMessage({ tone: 'error', text: (err as Error).message });
      return false;
    } finally {
      setBusy(false);
    }
  };

  const create = async () => {
    if (!userOk || !pwOk || !matchOk) return;
    const ok = await act((token) => createStaffAccountRequest({ username: form.username, password: form.password, role: form.role }, token), `Đã tạo tài khoản ${form.username}.`);
    if (ok) {
      setCreating(false);
      setForm({ username: '', password: '', confirm: '', role: 'dev' });
    }
  };

  const remove = (account: AccountInfo) => {
    if (!window.confirm(`Xoá tài khoản ${account.username} (${ROLE_LABEL[account.role]})? Không hoàn tác được.`)) return;
    void act((token) => deleteStaffAccountRequest(account.id, token), `Đã xoá ${account.username}.`);
  };

  const reset = async () => {
    if (!resetFor || newPw.length < 8) return;
    const ok = await act((token) => resetStaffPasswordRequest(resetFor.id, newPw, token), `Đã đặt lại mật khẩu cho ${resetFor.username}; phiên cũ bị thu hồi.`);
    if (ok) {
      setResetFor(null);
      setNewPw('');
    }
  };

  return (
    <div className="grid gap-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="grid gap-1">
          <PageTitle icon="users">Tài khoản</PageTitle>
          <p className="text-sm text-muted">Tạo, xoá, đặt lại mật khẩu cho admin và dev. Tài khoản người mua/người bán chỉ xem.</p>
        </div>
        <button type="button" className="btn-primary" onClick={() => setCreating((v) => !v)}>
          + Tạo admin / dev
        </button>
      </header>

      {message ? <Notice tone={message.tone}>{message.text}</Notice> : null}

      {creating ? (
        <form
          className="card grid gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            void create();
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <label>
              Tên đăng nhập
              <input value={form.username} onChange={(e) => setForm((p) => ({ ...p, username: e.target.value }))} autoComplete="off" required />
              {form.username && !userOk ? <span className="mt-1 block text-xs font-medium text-danger">3–16 ký tự: chữ, số, gạch dưới</span> : null}
            </label>
            <label>
              Vai trò
              <select value={form.role} onChange={(e) => setForm((p) => ({ ...p, role: e.target.value as 'admin' | 'dev' }))}>
                <option value="dev">Dev (vận hành)</option>
                <option value="admin">Admin (quản trị)</option>
              </select>
            </label>
            <label>
              Mật khẩu
              <PasswordInput value={form.password} autoComplete="new-password" onChange={(password) => setForm((p) => ({ ...p, password }))} />
              {form.password && !pwOk ? <span className="mt-1 block text-xs font-medium text-danger">Cần ít nhất 8 ký tự</span> : null}
            </label>
            <label>
              Nhập lại mật khẩu
              <PasswordInput value={form.confirm} autoComplete="new-password" onChange={(confirm) => setForm((p) => ({ ...p, confirm }))} />
              {form.confirm && !matchOk ? <span className="mt-1 block text-xs font-medium text-danger">Mật khẩu chưa khớp</span> : null}
            </label>
          </div>
          <div className="flex gap-2">
            <button type="submit" className="btn-primary" disabled={busy || !userOk || !pwOk || !matchOk}>
              {busy ? 'Đang tạo…' : 'Tạo tài khoản'}
            </button>
            <button type="button" className="btn" onClick={() => setCreating(false)}>
              Huỷ
            </button>
          </div>
        </form>
      ) : null}

      {resetFor ? (
        <form
          className="card flex flex-wrap items-end gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void reset();
          }}
        >
          <label className="min-w-[240px] flex-1">
            Mật khẩu mới cho <b>{resetFor.username}</b>
            <PasswordInput value={newPw} autoComplete="new-password" onChange={setNewPw} />
            {newPw && newPw.length < 8 ? <span className="mt-1 block text-xs font-medium text-danger">Cần ít nhất 8 ký tự</span> : null}
          </label>
          <button type="submit" className="btn-primary" disabled={busy || newPw.length < 8}>
            Đặt lại
          </button>
          <button type="button" className="btn" onClick={() => { setResetFor(null); setNewPw(''); }}>
            Huỷ
          </button>
        </form>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        {filters.map((f) => (
          <button key={f.value} type="button" aria-pressed={role === f.value} onClick={() => setRole(f.value)} className={`rounded-full border px-3 py-1.5 text-sm font-semibold transition ${role === f.value ? 'border-brand bg-brand-soft text-brand' : 'border-line bg-surface text-muted hover:bg-surface2'}`}>
            {f.label}
          </button>
        ))}
        <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Tìm tên đăng nhập" aria-label="Tìm tên đăng nhập" className="ml-auto h-9 w-56 max-w-full" />
      </div>

      {list.loading ? <LoadingBlock /> : null}
      {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
      {!list.loading && !list.error && accounts.length === 0 ? <EmptyBlock>Không có tài khoản nào.</EmptyBlock> : null}
      {accounts.length > 0 ? (
        <div className="card overflow-x-auto p-0">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Tài khoản</caption>
            <thead className="text-muted">
              <tr>
                <th className="px-3 py-3">ID</th>
                <th className="px-3 py-3">Tên đăng nhập</th>
                <th className="px-3 py-3">Vai trò</th>
                <th className="px-3 py-3" />
              </tr>
            </thead>
            <tbody>
              {accounts.map((a) => {
                const self = a.username === me;
                return (
                  <tr key={a.id} className="border-t border-line">
                    <td className="px-3 py-2 font-mono text-xs text-muted">{a.id}</td>
                    <td className="px-3 py-2 font-semibold text-text">
                      {a.username} {self ? <span className="chip align-middle">bạn</span> : null}
                    </td>
                    <td className="px-3 py-2">
                      <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${roleTone[a.role] ?? 'bg-surface2 text-muted'}`}>{ROLE_LABEL[a.role] ?? a.role}</span>
                    </td>
                    <td className="px-3 py-2">
                      {isStaff(a.role) && !self ? (
                        <div className="flex justify-end gap-2">
                          <button type="button" className="btn inline-flex items-center gap-1 py-1 text-xs" onClick={() => setResetFor(a)}>
                            <Icon name="key" className="h-3.5 w-3.5" /> Đặt lại mật khẩu
                          </button>
                          <button type="button" className="btn inline-flex items-center gap-1 py-1 text-xs text-danger" disabled={busy} onClick={() => remove(a)}>
                            <Icon name="trash" className="h-3.5 w-3.5" /> Xoá
                          </button>
                        </div>
                      ) : null}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
