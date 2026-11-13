'use client';

import { useEffect } from 'react';
import { nodeStatus } from '../../lib/systemMapModel';
import { formatMs, formatRate, formatUptime, STATUS_BADGE, STATUS_LABEL } from '../../lib/systemStatus';
import { EDGES, nodeById, PROTOCOL_LABEL } from '../../lib/topology';
import type { SystemMapData } from '../../lib/useSystemMap';
import { useChat } from '../ai/ChatProvider';
import { useAssistantEnabled } from '../ai/ChatWidget';
import { Sparkline } from './Sparkline';

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-lg border border-line bg-surface2/60 px-3 py-2">
      <dt className="text-[11px] font-semibold uppercase tracking-wide text-muted">{label}</dt>
      <dd className="tabular mt-0.5 text-base font-semibold text-text" title={hint}>
        {value}
      </dd>
    </div>
  );
}

const severityTone: Record<string, string> = {
  critical: 'bg-danger-soft text-danger',
  warning: 'bg-warning-soft text-warning',
  info: 'bg-info-soft text-info'
};

export function Overview({ data }: { data: SystemMapData }) {
  const apps = Object.values(data.health);
  const counts = { ready: 0, bad: 0, degraded: 0 };
  for (const s of apps) {
    if (s.status === 'ready') counts.ready++;
    else if (s.status === 'degraded') counts.degraded++;
    else counts.bad++;
  }
  return (
    <div className="grid gap-4">
      <div>
        <h2 className="text-base font-bold text-text">Toàn hệ thống</h2>
        <p className="text-sm text-muted">Bấm vào một service trên bản đồ để xem chi tiết.</p>
      </div>
      <dl className="grid grid-cols-2 gap-2">
        <Stat label="Sẵn sàng" value={`${counts.ready}/${apps.length}`} />
        <Stat label="Suy giảm" value={String(counts.degraded)} />
        <Stat label="Lỗi" value={String(counts.bad)} />
        <Stat label="Sự cố mở" value={String(data.incidents.length)} />
        <Stat label="Gateway" value={formatRate(data.metrics['api-gateway']?.rate)} hint="request HTTP/giây vào gateway" />
      </dl>
      {data.incidents.length > 0 ? (
        <ul className="grid gap-2">
          {data.incidents.slice(0, 5).map((i) => (
            <li key={i.id} className="rounded-lg border border-line px-3 py-2 text-sm">
              <span className={`mr-2 rounded-full px-2 py-0.5 text-xs font-semibold ${severityTone[i.severity] ?? 'bg-surface2 text-muted'}`}>{i.severity}</span>
              <span className="font-semibold text-text">{i.alert_name}</span>
              {i.summary ? <p className="mt-0.5 text-xs text-muted">{i.summary}</p> : null}
            </li>
          ))}
        </ul>
      ) : (
        <p className="rounded-lg bg-success-soft px-3 py-2 text-sm font-medium text-success">Không có sự cố nào đang mở.</p>
      )}
    </div>
  );
}

export function ServicePanel({ id, data, onClose }: { id: string; data: SystemMapData; onClose: () => void }) {
  const node = nodeById(id);
  const { setService, openChat } = useChat();
  const assistant = useAssistantEnabled();

  // the assistant gets the selected service as context while the panel is open
  useEffect(() => {
    setService(id);
    return () => setService(undefined);
  }, [id, setService]);

  if (!node) return null;
  const status = nodeStatus(node, data);
  const health = data.health[id];
  const m = data.metrics[id];
  const history = data.history[id];
  const failing = Object.entries(health?.checks ?? {});
  const incidents = data.incidents.filter(
    (i) => i.component === id || i.labels?.service === id || i.labels?.swarm_service === id || i.labels?.job === node.job
  );
  const links = EDGES.filter((e) => e.from === id || e.to === id);

  return (
    <div className="grid gap-4">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-base font-bold text-text">{node.label}</h2>
          <span className={`mt-1 inline-flex rounded-full px-2.5 py-0.5 text-xs font-semibold ${STATUS_BADGE[status]}`}>{STATUS_LABEL[status]}</span>
        </div>
        <button type="button" onClick={onClose} aria-label="Bỏ chọn" className="rounded-md px-2 py-1 text-lg leading-none text-muted hover:bg-surface2">
          ×
        </button>
      </div>
      <p className="text-sm text-muted">{node.desc}</p>

      {node.app ? (
        <>
          <dl className="grid grid-cols-2 gap-2">
            <Stat label="Replica sẵn sàng" value={m?.replicas !== undefined ? `${m.replicasUp ?? 0}/${m.replicas}` : '–'} hint="số target scrape được / tổng target (Prometheus)" />
            <Stat label="Uptime" value={formatUptime(health?.uptime_s)} />
            <Stat label={id === 'api-gateway' ? 'Request/giây' : 'Gọi/giây từ gateway'} value={formatRate(m?.rate)} />
            <Stat label="Độ trễ p95" value={id === 'api-gateway' ? '–' : formatMs(m?.p95)} hint="gRPC gateway → service, cửa sổ 5 phút" />
            <Stat label="Lỗi/giây" value={id === 'api-gateway' ? '–' : formatRate(m?.errorRate)} />
            <Stat label="Ping /ready" value={health ? `${health.latency_ms} ms` : '–'} />
            <Stat label="Phiên bản" value={health?.version ?? '–'} />
          </dl>
          {history ? (
            <div className="grid gap-3">
              <div>
                <p className="mb-1 text-xs font-semibold text-muted">Tần suất (từ khi mở trang)</p>
                <Sparkline values={history.rate} label={`Tần suất ${id}`} />
              </div>
              {id !== 'api-gateway' ? (
                <div>
                  <p className="mb-1 text-xs font-semibold text-muted">p95 (ms)</p>
                  <Sparkline values={history.p95} label={`p95 ${id}`} className="text-info" />
                </div>
              ) : null}
            </div>
          ) : null}
          {failing.length > 0 ? (
            <div>
              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">Kiểm tra phụ thuộc</h3>
              <ul className="grid gap-1 text-sm">
                {failing.map(([name, check]) => (
                  <li key={name} className="flex items-center justify-between gap-2">
                    <span className="text-text">{name}</span>
                    <span className={check.status === 'ok' ? 'text-success' : 'font-semibold text-danger'}>
                      {check.status === 'ok' ? `ok${check.latency_ms !== undefined ? ` · ${check.latency_ms} ms` : ''}` : check.error ?? 'lỗi'}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </>
      ) : null}

      <div>
        <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">Kết nối</h3>
        <ul className="grid gap-1 text-sm">
          {links.map((e) => {
            const out = e.from === id;
            const other = nodeById(out ? e.to : e.from);
            return (
              <li key={e.id} className="flex items-center justify-between gap-2">
                <span className="text-text">
                  {out ? '→' : '←'} {other?.label ?? (out ? e.to : e.from)}
                </span>
                <span className="chip">{PROTOCOL_LABEL[e.protocol]}</span>
              </li>
            );
          })}
        </ul>
      </div>

      <div>
        <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">Sự cố đang mở</h3>
        {incidents.length === 0 ? (
          <p className="text-sm text-muted">Không có.</p>
        ) : (
          <ul className="grid gap-2">
            {incidents.map((i) => (
              <li key={i.id} className="rounded-lg border border-line px-3 py-2 text-sm">
                <span className={`mr-2 rounded-full px-2 py-0.5 text-xs font-semibold ${severityTone[i.severity] ?? 'bg-surface2 text-muted'}`}>{i.severity}</span>
                <span className="font-semibold text-text">{i.alert_name}</span>
                {i.summary ? <p className="mt-0.5 text-xs text-muted">{i.summary}</p> : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      {assistant ? (
        <button type="button" className="btn-primary justify-self-start" onClick={() => openChat(`Service ${id} đang có vấn đề gì không?`)}>
          Hỏi trợ lý về service này
        </button>
      ) : null}
    </div>
  );
}
