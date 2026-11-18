'use client';

import { useCallback, useEffect, useState } from 'react';
import { useAuth } from '../../../components/auth/AuthProvider';
import { ApiError } from '../../../lib/api';
import { formatDateTime } from '../../../lib/format';
import { getAiKill, setAiKill, type AiKillScope, type AiKillState } from '../../../lib/rca';

const SCOPES: { id: AiKillScope; label: string }[] = [
  { id: 'buyer', label: 'Chat người mua' },
  { id: 'seller', label: 'Chat người bán' },
  { id: 'admin', label: 'Chat admin' },
  { id: 'operator', label: 'Chat operator' },
  { id: 'rca', label: 'Điều tra RCA' },
];

/**
 * Nút dừng khẩn: tắt ngay mọi đường của AI (chat mới, tool của agent, đề xuất, bộ theo dõi RCA) mà không cần deploy.
 * Dưới đó là 5 công tắc riêng (chat 4 vai và RCA) để chỉ dừng đúng phần nghi bị lừa; công tắc toàn cục thắng mọi công tắc riêng.
 * Người dùng vẫn mua bán bình thường. Phiên chat/điều tra đang chạy sẽ lỗi ở lần gọi kế tiếp. Mỗi lần bật/tắt được ghi vào sổ kiểm toán của ops-service (kèm log gateway).
 */
export function AiKillSwitch() {
  const { getValidAccessToken } = useAuth();
  const [state, setState] = useState<AiKillState | null>(null);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      setState(await getAiKill((await getValidAccessToken()) ?? ''));
      setError('');
    } catch (err) {
      setError(err instanceof ApiError ? `Không đọc được trạng thái (HTTP ${err.status}).` : 'Không đọc được trạng thái.');
    }
  }, [getValidAccessToken]);

  useEffect(() => {
    void load();
    const t = setInterval(() => void load(), 15000);
    return () => clearInterval(t);
  }, [load]);

  const flip = async (paused: boolean) => {
    if (paused && !window.confirm('Tắt toàn bộ AI ngay bây giờ? Chat mới, tool của agent, đề xuất và điều tra tự động sẽ dừng.')) return;
    setBusy(true);
    setError('');
    try {
      setState(await setAiKill((await getValidAccessToken()) ?? '', paused, reason.trim()));
      if (!paused) setReason('');
    } catch (err) {
      setError(err instanceof ApiError ? `${err.message} (HTTP ${err.status})` : 'Không đổi được công tắc.');
    } finally {
      setBusy(false);
    }
  };

  const flipScope = async (scope: AiKillScope, label: string, paused: boolean) => {
    if (paused && !window.confirm(`Tắt ${label.toLowerCase()} ngay bây giờ? Các phạm vi khác vẫn chạy.`)) return;
    setBusy(true);
    setError('');
    try {
      const token = (await getValidAccessToken()) ?? '';
      await setAiKill(token, paused, reason.trim(), scope);
      setState(await getAiKill(token));
      if (!paused) setReason('');
    } catch (err) {
      setError(err instanceof ApiError ? `${err.message} (HTTP ${err.status})` : 'Không đổi được công tắc.');
    } finally {
      setBusy(false);
    }
  };

  if (state && !state.available) return null; // gateway chạy không có Redis: không có công tắc

  const paused = state?.paused === true;
  return (
    <section className={`card flex flex-wrap items-center justify-between gap-3 ${paused ? 'border-danger' : ''}`} aria-label="Dừng khẩn AI">
      <div className="grid gap-1">
        <p className="text-sm font-semibold">
          {state === null ? 'Đang đọc trạng thái AI…' : paused ? 'AI đang bị TẮT KHẨN' : 'AI đang hoạt động'}
        </p>
        {paused ? (
          <p className="text-xs text-muted">
            {state?.unknown
              ? 'Không đọc được công tắc (Redis lỗi), nên gateway cũng từ chối AI cho an toàn.'
              : `Bởi ${state?.by ?? '?'}${state?.at ? ` lúc ${formatDateTime(state.at)}` : ''}${state?.reason ? `: ${state.reason}` : ''}`}
          </p>
        ) : (
          <p className="text-xs text-muted">Dùng khi nghi agent bị lừa, chi phí tăng bất thường hoặc nghi lộ khoá. Không ảnh hưởng việc mua bán.</p>
        )}
        {state?.audited === false ? (
          <p role="status" className="text-xs text-warning">
            Công tắc đã đổi nhưng chưa ghi được vào sổ kiểm toán của ops-service (gateway vẫn có log).
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-xs text-danger">
            {error}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {!paused ? (
          <input
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            maxLength={200}
            placeholder="Lý do (không bắt buộc)"
            aria-label="Lý do tắt AI"
            className="h-9 w-56 rounded-lg text-sm"
          />
        ) : null}
        <button
          type="button"
          disabled={busy || state === null}
          onClick={() => void flip(!paused)}
          className={`btn h-9 px-3 text-xs ${paused ? '' : 'text-danger'}`}
        >
          {paused ? 'Bật lại AI' : 'Tắt khẩn AI'}
        </button>
      </div>
      {state?.scopes && !state.unknown ? (
        <ul className="grid w-full gap-2 sm:grid-cols-2 lg:grid-cols-5" aria-label="Công tắc theo phạm vi">
          {SCOPES.map(({ id, label }) => {
            const sc = state.scopes?.[id];
            const off = sc?.paused === true;
            return (
              <li key={id} className={`flex items-center justify-between gap-2 rounded-lg border px-3 py-2 ${off ? 'border-danger' : ''}`}>
                <div className="grid">
                  <span className="text-xs font-semibold">{label}</span>
                  <span className="text-xs text-muted">
                    {paused ? 'dừng (toàn cục)' : off ? `TẮT${sc?.by ? ` bởi ${sc.by}` : ''}${sc?.reason ? `: ${sc.reason}` : ''}` : 'đang chạy'}
                  </span>
                </div>
                <button
                  type="button"
                  disabled={busy || paused}
                  onClick={() => void flipScope(id, label, !off)}
                  className={`btn h-8 px-2 text-xs ${off ? '' : 'text-danger'}`}
                  aria-label={`${off ? 'Bật lại' : 'Tắt'} ${label}`}
                >
                  {off ? 'Bật lại' : 'Tắt'}
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
    </section>
  );
}
