'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useAuth } from '../../../components/auth/AuthProvider';
import { getRcaEvents, type RcaEvent, type RunStatus } from '../../../lib/rca';

const POLL_MS = 2000;
const UNTRUSTED_TAG = /<\/?du_lieu_khong_tin_cay>/g;

function cap(text: string): string {
  const t = text.trim();
  return t ? t.charAt(0).toLocaleUpperCase('vi') + t.slice(1) : '';
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : v == null ? '' : JSON.stringify(v);
}

function clock(at: number): string {
  return new Date(at * 1000).toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function Line({ e }: { e: RcaEvent }) {
  const d = e.data;
  const time = <span className="shrink-0 text-[11px] text-muted">{clock(e.at)}</span>;
  switch (e.kind) {
    case 'tool_call':
      return (
        <div className="flex gap-2 text-sm">
          {time}
          <div>
            <span className="font-semibold">Gọi công cụ</span> <code className="text-xs">{e.title}</code>
            {d.args && Object.keys(d.args as object).length ? <code className="ml-2 break-all text-xs text-muted">{JSON.stringify(d.args)}</code> : null}
          </div>
        </div>
      );
    case 'tool_result': {
      const text = str(d.text).replace(UNTRUSTED_TAG, '');
      const failed = d.error === true;
      return (
        <details className="text-sm">
          <summary className="flex cursor-pointer gap-2">
            {time}
            <span className={failed ? 'font-semibold text-danger' : 'font-semibold'}>{failed ? 'Công cụ lỗi' : 'Kết quả'}</span>
            <code className="text-xs">{e.title}</code>
            <span className="truncate text-xs text-muted">{text.slice(0, 90)}</span>
          </summary>
          <pre className="mt-1 max-h-60 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface2 p-2 text-xs">{text}</pre>
        </details>
      );
    }
    case 'note':
      return (
        <div className="flex gap-2 text-sm">
          {time}
          <p className="rounded-xl bg-brand-soft/40 px-3 py-1.5">
            <span className="font-semibold">Trợ lý:</span> {cap(str(d.text))}
          </p>
        </div>
      );
    case 'hypotheses': {
      const items = (d.items as { cause: string; confidence: number }[]) ?? [];
      return (
        <div className="flex gap-2 text-sm">
          {time}
          <div>
            <span className="font-semibold">Giả thuyết</span>
            <ul className="ml-4 list-disc">
              {items.map((h, i) => (
                <li key={i}>
                  <strong>{Math.round(h.confidence * 100)}%</strong> {cap(h.cause)}
                </li>
              ))}
            </ul>
          </div>
        </div>
      );
    }
    case 'draft': {
      const dr = (d.draft ?? {}) as { action_id?: string; params?: unknown; rationale?: string };
      return (
        <div className="flex gap-2 text-sm">
          {time}
          <div>
            <span className="font-semibold">Nháp đề xuất:</span> <code className="text-xs">{dr.action_id}</code> {dr.params && Object.keys(dr.params as object).length ? <code className="text-xs">{JSON.stringify(dr.params)}</code> : null}
            {dr.rationale ? <p className="text-muted">{cap(dr.rationale)}</p> : null}
          </div>
        </div>
      );
    }
    case 'report': {
      const r = (d.report ?? {}) as { summary?: string };
      return (
        <div className="flex gap-2 text-sm">
          {time}
          <p>
            <span className="font-semibold">Báo cáo:</span> {cap(str(r.summary))}
          </p>
        </div>
      );
    }
    case 'wait':
      return (
        <div className="flex gap-2 text-sm font-medium text-warning">
          {time}
          <span>{cap(e.title)}</span>
        </div>
      );
    case 'error':
      return (
        <div className="flex gap-2 text-sm text-danger">
          {time}
          <span>
            {cap(e.title)}
            {d.detail ? `: ${str(d.detail)}` : ''}
          </span>
        </div>
      );
    default:
      return (
        <div className="flex gap-2 text-sm text-muted">
          {time}
          <span>
            {e.title}
            {d.note ? `: ${cap(str(d.note))}` : ''}
          </span>
        </div>
      );
  }
}

/** Quá trình điều tra như một đoạn chat: gọi công cụ nào, kết quả ra sao, trợ lý nhận định gì. Hỏi server mỗi 2 giây khi phiên đang chạy. */
export function RunLog({ runId, status }: { runId: number; status: RunStatus }) {
  const { getValidAccessToken } = useAuth();
  const [events, setEvents] = useState<RcaEvent[]>([]);
  const [error, setError] = useState('');
  const after = useRef(0);
  const bottom = useRef<HTMLDivElement | null>(null);
  const stick = useRef(true);

  const pull = useCallback(async () => {
    try {
      const token = (await getValidAccessToken()) ?? '';
      const out = await getRcaEvents(token, runId, after.current);
      if (out.items.length) {
        after.current = out.items[out.items.length - 1].id;
        setEvents((cur) => [...cur, ...out.items]);
      }
      setError('');
    } catch (err) {
      setError(cap((err as Error)?.message || 'Không tải được nhật ký điều tra.'));
    }
  }, [getValidAccessToken, runId]);

  useEffect(() => {
    void pull();
  }, [pull, status]); // đổi trạng thái (chuyển sang chờ, xong) thì lấy nốt phần cuối

  useEffect(() => {
    if (status !== 'running') return;
    const t = setInterval(() => void pull(), POLL_MS);
    return () => clearInterval(t);
  }, [status, pull]);

  useEffect(() => {
    if (stick.current && status === 'running') bottom.current?.scrollIntoView({ block: 'nearest' });
  }, [events, status]);

  return (
    <details className="card grid gap-2" open={status === 'running' || events.length > 0}>
      <summary className="cursor-pointer text-sm font-bold">
        Quá trình điều tra ({events.length}){status === 'running' ? ' · đang chạy…' : ''}
      </summary>
      {error ? <p className="text-xs text-danger">{error}</p> : null}
      {events.length === 0 && !error ? <p className="text-xs text-muted">{status === 'running' ? 'Đang chờ sự kiện đầu tiên…' : 'Phiên này không có nhật ký (chạy trước khi có tính năng này).'}</p> : null}
      <div
        className="grid max-h-[28rem] gap-1.5 overflow-auto"
        onScroll={(ev) => {
          const el = ev.currentTarget;
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
        }}
      >
        {events.map((e) => (
          <Line key={e.id} e={e} />
        ))}
        <div ref={bottom} />
      </div>
    </details>
  );
}
