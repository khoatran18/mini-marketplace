'use client';

import { PageTitle } from '../../../components/layout/PageTitle';
import { useMemo, useState } from 'react';
import { ServicePanel, Overview } from '../../../components/system/ServicePanel';
import { SystemMap } from '../../../components/system/SystemMap';
import { formatTime } from '../../../lib/format';
import { formatMs, formatRate, formatUptime, STATUS_BADGE, STATUS_LABEL } from '../../../lib/systemStatus';
import { LAYER_LABEL, NODES, type EdgeLayer } from '../../../lib/topology';
import { REFRESH_MS, useSystemMap } from '../../../lib/useSystemMap';

type View = 'map' | 'table';

export function SystemHealthClient() {
  const data = useSystemMap();
  const [view, setView] = useState<View>('map');
  const [layers, setLayers] = useState<Set<EdgeLayer>>(new Set<EdgeLayer>(['sync', 'async']));
  const [selected, setSelected] = useState<string | null>(null);

  const toggleLayer = (layer: EdgeLayer) =>
    setLayers((current) => {
      const next = new Set(current);
      if (next.has(layer)) next.delete(layer);
      else next.add(layer);
      return next;
    });

  const services = useMemo(() => NODES.filter((n) => n.app).map((n) => ({ node: n, health: data.health[n.id], metrics: data.metrics[n.id] })), [data.health, data.metrics]);

  return (
    <div className="grid gap-5">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <PageTitle icon="map">Hệ thống</PageTitle>
          <p className="text-sm text-muted">
            Bản đồ các service và kết nối, làm mới mỗi {REFRESH_MS / 1000}s{data.asOf ? <> · cập nhật {formatTime(data.asOf)}</> : null}.
          </p>
        </div>
        <div role="group" aria-label="Kiểu hiển thị" className="inline-flex overflow-hidden rounded-lg border border-line text-sm">
          {(['map', 'table'] as const).map((v) => (
            <button
              key={v}
              type="button"
              aria-pressed={view === v}
              onClick={() => setView(v)}
              className={`px-3 py-1.5 font-medium transition ${view === v ? 'bg-brand-soft font-semibold text-brand' : 'bg-surface text-muted hover:bg-surface2'}`}
            >
              {v === 'map' ? 'Bản đồ' : 'Bảng'}
            </button>
          ))}
        </div>
      </header>

      {data.error ? (
        <p role="alert" className="card border-danger/30 bg-danger-soft text-sm font-medium text-danger">
          {data.error}
        </p>
      ) : null}
      {data.metricsUnavailable && !data.loading && !data.error ? (
        <p className="card border-warning/30 bg-warning-soft text-sm text-warning">
          Chưa lấy được số liệu Prometheus (replica, tần suất, p95). Đang chỉ hiển thị kết quả <code>/ready</code>.
        </p>
      ) : null}
      {data.loading && !data.error ? <p role="status" className="card text-sm text-muted">Đang tải…</p> : null}

      {!data.loading && view === 'map' ? (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="grid content-start gap-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              {(Object.keys(LAYER_LABEL) as EdgeLayer[]).map((layer) => (
                <label key={layer} className="inline-flex cursor-pointer items-center gap-2 rounded-lg border border-line bg-surface px-3 py-1.5 text-sm font-medium text-text">
                  <input type="checkbox" checked={layers.has(layer)} onChange={() => toggleLayer(layer)} className="!w-auto" />
                  {LAYER_LABEL[layer]}
                </label>
              ))}
              <span className="ml-auto flex flex-wrap items-center gap-3 text-xs text-muted">
                <i className="inline-block h-0.5 w-5 bg-success" /> bình thường
                <i className="inline-block h-0.5 w-5 bg-warning" /> chậm
                <i className="inline-block h-0.5 w-5 bg-danger" /> lỗi
              </span>
            </div>
            <SystemMap data={data} layers={layers} selectedId={selected} onSelect={setSelected} />
          </div>
          <aside className="card h-fit lg:sticky lg:top-20" aria-label="Chi tiết">
            {selected ? <ServicePanel id={selected} data={data} onClose={() => setSelected(null)} /> : <Overview data={data} />}
          </aside>
        </div>
      ) : null}

      {!data.loading && view === 'table' ? (
        <div className="card overflow-x-auto p-0 md:p-0">
          <table className="tabular w-full text-left text-sm">
            <thead className="border-b border-line text-xs uppercase tracking-wide text-muted">
              <tr>
                <th className="px-4 py-3">Service</th>
                <th className="px-4 py-3">Trạng thái</th>
                <th className="px-4 py-3">Replica</th>
                <th className="px-4 py-3">Phiên bản</th>
                <th className="px-4 py-3">Uptime</th>
                <th className="px-4 py-3">Ping</th>
                <th className="px-4 py-3">p95</th>
                <th className="px-4 py-3">Tần suất</th>
                <th className="px-4 py-3">Kiểm tra lỗi</th>
              </tr>
            </thead>
            <tbody>
              {services.map(({ node, health, metrics }) => {
                const failing = Object.entries(health?.checks ?? {}).filter(([, check]) => check.status === 'fail');
                const status = health?.status ?? 'unknown';
                return (
                  <tr key={node.id} className="border-t border-line hover:bg-surface2/60">
                    <td className="px-4 py-2.5 font-medium text-text">{node.label}</td>
                    <td className="px-4 py-2.5">
                      <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${STATUS_BADGE[status]}`}>{STATUS_LABEL[status]}</span>
                    </td>
                    <td className="px-4 py-2.5">{metrics?.replicas !== undefined ? `${metrics.replicasUp ?? 0}/${metrics.replicas}` : '–'}</td>
                    <td className="px-4 py-2.5">{health?.version ?? '–'}</td>
                    <td className="px-4 py-2.5">{formatUptime(health?.uptime_s)}</td>
                    <td className="px-4 py-2.5">{health ? `${health.latency_ms} ms` : '–'}</td>
                    <td className="px-4 py-2.5">{node.id === 'api-gateway' ? '–' : formatMs(metrics?.p95)}</td>
                    <td className="px-4 py-2.5">{formatRate(metrics?.rate)}</td>
                    <td className="px-4 py-2.5 text-muted">
                      {failing.length === 0 ? '–' : failing.map(([name, check]) => `${name}${check.error ? ` (${check.error})` : ''}`).join(', ')}
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
