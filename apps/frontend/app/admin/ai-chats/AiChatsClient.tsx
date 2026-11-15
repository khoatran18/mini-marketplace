'use client';

import { useCallback, useEffect, useState } from 'react';
import { useAuth } from '../../../components/auth/AuthProvider';
import { fetchConversation, fetchConversations, type ConversationDetail, type ConversationSummary } from '../../../lib/ai-chat';
import { formatDateTime } from '../../../lib/format';

const ROLES: { value: string; label: string }[] = [
  { value: '', label: 'Mọi vai trò' },
  { value: 'buyer', label: 'Người mua' },
  { value: 'seller_admin', label: 'Chủ shop' },
  { value: 'seller_employee', label: 'Nhân viên shop' },
  { value: 'admin', label: 'Admin' },
  { value: 'operator', label: 'Operator' }
];
const ROLE_LABEL = Object.fromEntries(ROLES.map((r) => [r.value, r.label]));

export function AiChatsClient() {
  const { getValidAccessToken } = useAuth();
  const [role, setRole] = useState('');
  const [userId, setUserId] = useState('');
  const [items, setItems] = useState<ConversationSummary[] | null>(null);
  const [detail, setDetail] = useState<ConversationDetail | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    setItems(null);
    try {
      setItems(await fetchConversations(await getValidAccessToken(), 'admin', { role, userId: userId.trim(), limit: 100 }));
    } catch (err) {
      setError((err as Error).message || 'Không tải được danh sách.');
      setItems([]);
    }
  }, [getValidAccessToken, role, userId]);

  useEffect(() => {
    void load();
    // reload when the role filter changes; the user filter applies on submit
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [role]);

  const open = async (id: string) => {
    setError('');
    try {
      setDetail(await fetchConversation(await getValidAccessToken(), id, 'admin'));
    } catch (err) {
      setError((err as Error).message || 'Không mở được cuộc trò chuyện.');
    }
  };

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <section className="card grid content-start gap-3">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void load();
          }}
        >
          <label className="grid gap-1 text-xs font-semibold text-muted">
            Vai trò
            <select value={role} onChange={(e) => setRole(e.target.value)} className="h-9 rounded-lg text-sm">
              {ROLES.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-xs font-semibold text-muted">
            Mã người dùng
            <input value={userId} onChange={(e) => setUserId(e.target.value.replace(/\D/g, ''))} inputMode="numeric" placeholder="vd. 42" className="h-9 w-28 rounded-lg text-sm" />
          </label>
          <button type="submit" className="btn h-9 px-3">
            Lọc
          </button>
        </form>
        {error ? <p role="alert" className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-1.5 text-xs text-danger">{error}</p> : null}
        {items === null ? <p className="text-sm text-muted">Đang tải…</p> : null}
        {items?.length === 0 && !error ? <p className="text-sm text-muted">Không có cuộc trò chuyện nào.</p> : null}
        <ul className="grid gap-1.5">
          {items?.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                onClick={() => void open(c.id)}
                aria-current={detail?.id === c.id}
                className={`w-full rounded-lg border px-3 py-2 text-left transition hover:border-brand/50 hover:bg-brand-soft ${detail?.id === c.id ? 'border-brand/50 bg-brand-soft' : 'border-line bg-surface'}`}
              >
                <span className="line-clamp-2 text-sm font-medium">{c.title || '(không có nội dung)'}</span>
                <span className="mt-0.5 block text-[11px] text-muted">
                  {ROLE_LABEL[c.role] ?? c.role} · người dùng #{c.user_id} · {formatDateTime(c.updated_at * 1000)} · {c.messages} tin
                </span>
              </button>
            </li>
          ))}
        </ul>
      </section>

      <section className="card grid content-start gap-3" aria-live="polite">
        {detail ? (
          <>
            <p className="text-xs text-muted">
              {ROLE_LABEL[detail.role] ?? detail.role} · người dùng #{detail.user_id} · bắt đầu {formatDateTime(detail.created_at * 1000)}
            </p>
            <div className="grid gap-2">
              {detail.messages.map((m, i) => (
                <div key={i} className={`flex flex-col ${m.role === 'user' ? 'items-end' : 'items-start'}`}>
                  <div className={`max-w-[90%] whitespace-pre-wrap rounded-2xl px-3.5 py-2 text-sm leading-relaxed ${m.role === 'user' ? 'rounded-br-md bg-brand-solid text-white' : 'rounded-bl-md bg-surface2 text-text'}`}>{m.content}</div>
                  <span className="mt-0.5 text-[11px] text-muted">{formatDateTime(m.created_at * 1000)}</span>
                </div>
              ))}
            </div>
          </>
        ) : (
          <p className="text-sm text-muted">Chọn một cuộc trò chuyện bên trái để xem.</p>
        )}
      </section>
    </div>
  );
}
