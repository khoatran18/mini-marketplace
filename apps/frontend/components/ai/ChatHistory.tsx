'use client';

import { useEffect, useState } from 'react';
import { useAuth } from '../auth/AuthProvider';
import { fetchConversations, type ConversationSummary } from '../../lib/ai-chat';
import { formatDateTime } from '../../lib/format';

/** List of the signed-in user's earlier conversations; picking one reopens it in the chat panel. */
export function ChatHistory({ onPick, onClose }: { onPick: (id: string) => Promise<void>; onClose: () => void }) {
  const { getValidAccessToken } = useAuth();
  const [items, setItems] = useState<ConversationSummary[] | null>(null);
  const [error, setError] = useState('');
  const [opening, setOpening] = useState('');

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const rows = await fetchConversations(await getValidAccessToken(), 'mine', { limit: 30 });
        if (alive) setItems(rows);
      } catch (err) {
        if (alive) setError((err as Error).message || 'Không tải được lịch sử.');
      }
    })();
    return () => {
      alive = false;
    };
  }, [getValidAccessToken]);

  const pick = async (id: string) => {
    setOpening(id);
    try {
      await onPick(id);
      onClose();
    } catch (err) {
      setError((err as Error).message || 'Không mở được cuộc trò chuyện.');
      setOpening('');
    }
  };

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <p className="text-sm font-semibold">Cuộc trò chuyện gần đây</p>
        <button type="button" onClick={onClose} className="rounded-md px-2 py-1 text-xs font-semibold text-muted hover:bg-surface2">
          Quay lại
        </button>
      </div>
      <p className="text-xs text-muted">Chỉ lưu câu hỏi và câu trả lời (đã che số điện thoại, email), giữ 30 ngày.</p>
      {error ? <p role="alert" className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-1.5 text-xs text-danger">{error}</p> : null}
      {items === null && !error ? <p className="text-sm text-muted">Đang tải…</p> : null}
      {items?.length === 0 ? <p className="text-sm text-muted">Chưa có cuộc trò chuyện nào.</p> : null}
      <ul className="grid gap-1.5">
        {items?.map((c) => (
          <li key={c.id}>
            <button
              type="button"
              disabled={opening !== ''}
              onClick={() => void pick(c.id)}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-left transition hover:border-brand/50 hover:bg-brand-soft disabled:opacity-60"
            >
              <span className="line-clamp-2 text-sm font-medium">{c.title || '(không có nội dung)'}</span>
              <span className="mt-0.5 block text-[11px] text-muted">
                {formatDateTime(c.updated_at * 1000)} · {c.messages} tin
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
