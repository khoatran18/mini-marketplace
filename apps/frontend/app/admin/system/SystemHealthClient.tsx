'use client';

import { useCallback, useEffect, useState } from 'react';
import { useAuth } from '../../../components/auth/AuthProvider';
import { getSystemHealthRequest } from '../../../lib/api';
import { formatTime } from '../../../lib/format';
import type { ServiceStatus, SystemHealthOutput } from '../../../lib/types';

const REFRESH_MS = 10_000;

const statusStyle: Record<ServiceStatus, string> = {
  ready: 'bg-success-soft text-success',
  degraded: 'bg-warning-soft text-warning',
  not_ready: 'bg-danger-soft text-danger',
  unreachable: 'bg-surface2 text-muted'
};

function formatUptime(seconds?: number) {
  if (seconds === undefined) return '–';
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

export function SystemHealthClient() {
  const { getValidAccessToken } = useAuth();
  const [data, setData] = useState<SystemHealthOutput | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const token = await getValidAccessToken();
      if (!token) throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      setData(await getSystemHealthRequest(token));
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, [getValidAccessToken]);

  useEffect(() => {
    void load();
    const id = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  return (
    <div className="grid gap-6">
      <header className="card grid gap-2">
        <h1 className="text-2xl font-bold text-text">Tình trạng hệ thống</h1>
        <p className="text-sm text-muted">
          Tổng hợp <code>/ready</code> của từng service (tự làm mới mỗi {REFRESH_MS / 1000}s).
          {data ? <> Cập nhật lúc {formatTime(data.as_of)}.</> : null}
        </p>
      </header>

      {error ? <p className="card text-sm text-danger">{error}</p> : null}
      {!data && !error ? <p className="card text-sm text-muted">Đang tải…</p> : null}

      {data ? (
        <div className="card overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-muted">
              <tr>
                <th className="py-2 pr-4">Service</th>
                <th className="py-2 pr-4">Trạng thái</th>
                <th className="py-2 pr-4">Phiên bản</th>
                <th className="py-2 pr-4">Uptime</th>
                <th className="py-2 pr-4">Độ trễ</th>
                <th className="py-2">Kiểm tra lỗi</th>
              </tr>
            </thead>
            <tbody>
              {data.services.map((service) => {
                const failing = Object.entries(service.checks ?? {}).filter(([, check]) => check.status === 'fail');
                return (
                  <tr key={service.name} className="border-t border-line">
                    <td className="py-2 pr-4 font-medium text-text">{service.name}</td>
                    <td className="py-2 pr-4">
                      <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${statusStyle[service.status]}`}>{service.status}</span>
                    </td>
                    <td className="py-2 pr-4">{service.version ?? '–'}</td>
                    <td className="py-2 pr-4">{formatUptime(service.uptime_s)}</td>
                    <td className="py-2 pr-4">{service.latency_ms} ms</td>
                    <td className="py-2 text-muted">
                      {failing.length === 0
                        ? '–'
                        : failing.map(([name, check]) => `${name}${check.error ? ` (${check.error})` : ''}`).join(', ')}
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
