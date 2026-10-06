'use client';

import { useEffect, useState } from 'react';
import { formatTime } from '../../../lib/format';
import { getTracker, setConsent, track, type DebugEntry, type DebugState } from '../../../lib/tracking';

const stateStyle: Record<DebugState, string> = {
  queued: 'bg-surface2 text-muted',
  sending: 'bg-info-soft text-info',
  sent: 'bg-success-soft text-success',
  failed: 'bg-danger-soft text-danger',
  dropped: 'bg-warning-soft text-warning'
};

/** Realtime view of the events produced by the tracking SDK in this browser (queue, delivery state, payloads). */
export function TrackingDebugClient() {
  const [, setVersion] = useState(0);
  const [mounted, setMounted] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const tracker = mounted ? getTracker() : null;

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    const instance = getTracker();
    if (!instance) return;
    const unsubscribe = instance.subscribe(() => setVersion((value) => value + 1));
    const timer = setInterval(() => setVersion((value) => value + 1), 1000); // refresh queue age / counters
    return () => {
      unsubscribe();
      clearInterval(timer);
    };
  }, []);

  const log: DebugEntry[] = tracker?.getLog() ?? [];
  const disabled = tracker?.isDisabled() ?? null;
  const counts = log.reduce<Record<string, number>>((acc, entry) => ({ ...acc, [entry.state]: (acc[entry.state] ?? 0) + 1 }), {});
  const chosen = log.find((entry) => entry.event.event_id === selected) ?? null;

  return (
    <div className="grid gap-6">
      <header className="card grid gap-3">
        <h1 className="text-2xl font-bold text-text">Tracking – debug</h1>
        <p className="text-sm text-muted">
          Event được gom theo lô (5 giây hoặc 20 event) và gửi tới <code>POST /events</code>. Nếu gateway chưa có endpoint (404) SDK tự tắt trong lần tải trang này.
        </p>
        <dl className="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <div><dt className="text-muted">anonymous_id</dt><dd className="font-mono text-text">{tracker?.anonymousId() ?? '–'}</dd></div>
          <div><dt className="text-muted">session_id</dt><dd className="font-mono text-text">{tracker?.sessionId() ?? '–'}</dd></div>
          <div><dt className="text-muted">Hàng đợi</dt><dd className="text-text">{tracker?.pending() ?? 0} event</dd></div>
          <div>
            <dt className="text-muted">Trạng thái SDK</dt>
            <dd className={disabled ? 'font-semibold text-warning' : 'font-semibold text-success'}>{disabled ? `Tắt: ${disabled}` : 'Đang bật'}</dd>
          </div>
        </dl>
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className="btn-primary" onClick={() => void tracker?.flush()}>Gửi ngay</button>
          <button type="button" className="btn" onClick={() => track('page_view', { source: 'dev_tracking_test' }, { surface: 'admin_console' })}>Tạo event thử</button>
          <button type="button" className="btn" onClick={() => tracker?.resume()} disabled={!disabled}>Bật lại SDK</button>
          <button type="button" className="btn" onClick={() => tracker?.clearLog()}>Xoá nhật ký</button>
          <label className="flex items-center gap-2 font-normal">
            <input type="checkbox" checked={tracker?.hasConsent() ?? true} onChange={(event) => setConsent(event.target.checked)} />
            Đồng ý analytics (consent)
          </label>
        </div>
        <p className="text-xs text-muted">
          {(['queued', 'sending', 'sent', 'failed', 'dropped'] as DebugState[]).map((state) => `${state}: ${counts[state] ?? 0}`).join(' · ')}
        </p>
      </header>

      <div className="grid gap-6 lg:grid-cols-[1fr_420px]">
        <section className="card overflow-x-auto p-0" aria-label="Nhật ký event">
          {log.length === 0 ? (
            <p className="p-6 text-sm text-muted">Chưa có event nào. Duyệt vài trang rồi quay lại đây.</p>
          ) : (
            <table className="w-full text-left text-sm">
              <thead className="text-muted">
                <tr>
                  <th className="px-3 py-2">Giờ</th>
                  <th className="px-3 py-2">Loại</th>
                  <th className="px-3 py-2">Surface</th>
                  <th className="px-3 py-2">Trang</th>
                  <th className="px-3 py-2">Trạng thái</th>
                </tr>
              </thead>
              <tbody>
                {log.map((entry) => (
                  <tr
                    key={entry.event.event_id}
                    className={`cursor-pointer border-t border-line ${selected === entry.event.event_id ? 'bg-brand-soft' : 'hover:bg-surface2'}`}
                    onClick={() => setSelected(entry.event.event_id)}
                  >
                    <td className="whitespace-nowrap px-3 py-2 text-muted">{formatTime(entry.event.ts_client)}</td>
                    <td className="px-3 py-2 font-semibold text-text">{entry.event.event_type}</td>
                    <td className="px-3 py-2 text-muted">{entry.event.surface ?? '–'}</td>
                    <td className="px-3 py-2 text-muted">{entry.event.page.path}</td>
                    <td className="px-3 py-2">
                      <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${stateStyle[entry.state]}`}>{entry.state}</span>
                      {entry.note ? <span className="ml-2 text-xs text-muted">{entry.note}</span> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
        <aside className="card min-w-0 p-4" aria-label="Chi tiết event">
          <h2 className="mb-2 text-sm font-bold uppercase tracking-wide text-muted">Envelope</h2>
          {chosen ? <pre className="max-h-[32rem] overflow-auto text-xs text-text">{JSON.stringify(chosen.event, null, 2)}</pre> : <p className="text-sm text-muted">Chọn một event để xem JSON gửi đi.</p>}
        </aside>
      </div>
    </div>
  );
}
