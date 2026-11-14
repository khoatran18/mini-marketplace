'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useAuth } from '../components/auth/AuthProvider';
import { getSystemHealthRequest, listOpsIncidentsRequest, runOpsQueryRequest } from './api';
import type { OpsIncident, OpsQueryResult, ServiceHealth } from './types';

export const REFRESH_MS = 10_000;
const HISTORY = 30; // samples kept per metric = 5 minutes at 10 s

export interface ServiceMetrics {
  replicas?: number;
  replicasUp?: number;
  /** calls/s the gateway makes to this service (gateway itself: HTTP req/s) */
  rate?: number;
  /** failed gateway→service calls/s */
  errorRate?: number;
  /** p95 of gateway→service gRPC calls, seconds */
  p95?: number;
}

export interface SystemMapData {
  asOf: string | null;
  health: Record<string, ServiceHealth>;
  metrics: Record<string, ServiceMetrics>;
  /** Prometheus `up` of infrastructure jobs (1 = up). Missing key = unknown. */
  infraUp: Record<string, boolean>;
  incidents: OpsIncident[];
  /** per node id: recent samples for sparklines (since this page was opened) */
  history: Record<string, { rate: number[]; p95: number[] }>;
  /** true when the Prometheus-backed queries failed (the map then shows health only) */
  metricsUnavailable: boolean;
  error: string | null;
  loading: boolean;
}

const empty: SystemMapData = {
  asOf: null,
  health: {},
  metrics: {},
  infraUp: {},
  incidents: [],
  history: {},
  metricsUnavailable: false,
  error: null,
  loading: true
};

function bySeries(result: PromiseSettledResult<OpsQueryResult>, label: string): Record<string, number> {
  const out: Record<string, number> = {};
  if (result.status !== 'fulfilled') return out;
  for (const s of result.value.series ?? []) {
    const key = s.labels[label];
    const value = Number(s.value);
    if (key && Number.isFinite(value)) out[key] = value;
  }
  return out;
}

/** Polls health, Prometheus-backed queries and open incidents every 10 s. Each part may fail independently. */
export function useSystemMap(): SystemMapData & { reload: () => void } {
  const { getValidAccessToken } = useAuth();
  const [data, setData] = useState<SystemMapData>(empty);
  const historyRef = useRef<SystemMapData['history']>({});

  const load = useCallback(async () => {
    try {
      const token = await getValidAccessToken();
      if (!token) throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      const q = (name: string, params: Record<string, string> = {}) => runOpsQueryRequest(token, name, params);
      const [health, replicas, replicasUp, grpcRate, grpcErr, grpcP95, gwRate, infra, incidents] = await Promise.allSettled([
        getSystemHealthRequest(token),
        q('service_replicas'),
        q('service_replicas_up'),
        q('grpc_request_rate'),
        q('grpc_error_rate'),
        q('grpc_latency_p95'),
        q('gateway_request_rate'),
        q('infra_up'),
        listOpsIncidentsRequest(token, 'open')
      ]);

      if (health.status === 'rejected') throw health.reason;

      const nReplicas = bySeries(replicas, 'swarm_service');
      const nUp = bySeries(replicasUp, 'swarm_service');
      const rate = bySeries(grpcRate, 'service');
      const err = bySeries(grpcErr, 'service');
      const p95 = bySeries(grpcP95, 'service');
      const gwSeries = gwRate.status === 'fulfilled' ? Number(gwRate.value.series?.[0]?.value) : NaN;
      const promOk = [replicas, replicasUp, grpcRate, infra].some((r) => r.status === 'fulfilled');

      const names = Array.from(new Set([...Object.keys(nReplicas), ...Object.keys(rate), ...Object.keys(p95), 'api-gateway']));
      const metrics: Record<string, ServiceMetrics> = {};
      for (const name of names) {
        metrics[name] = {
          replicas: nReplicas[name],
          replicasUp: nUp[name],
          rate: name === 'api-gateway' ? (Number.isFinite(gwSeries) ? gwSeries : undefined) : rate[name],
          errorRate: err[name],
          p95: p95[name]
        };
      }

      const infraUp: Record<string, boolean> = {};
      if (infra.status === 'fulfilled') {
        for (const s of infra.value.series ?? []) {
          const job = s.labels.job;
          if (!job) continue;
          // several targets per job: the job is up only when all of them are
          infraUp[job] = (infraUp[job] ?? true) && Number(s.value) === 1;
        }
      }
      if (promOk) infraUp.prometheus = true; // we just got an answer from it through ops-service

      for (const [name, m] of Object.entries(metrics)) {
        const h = historyRef.current[name] ?? { rate: [], p95: [] };
        if (m.rate !== undefined) h.rate = [...h.rate, m.rate].slice(-HISTORY);
        if (m.p95 !== undefined && Number.isFinite(m.p95)) h.p95 = [...h.p95, m.p95 * 1000].slice(-HISTORY);
        historyRef.current[name] = h;
      }

      setData({
        asOf: health.value.as_of,
        health: Object.fromEntries(health.value.services.map((s) => [s.name, s])),
        metrics,
        infraUp,
        incidents: incidents.status === 'fulfilled' ? incidents.value.incidents ?? [] : [],
        history: { ...historyRef.current },
        metricsUnavailable: !promOk,
        error: null,
        loading: false
      });
    } catch (err) {
      setData((prev) => ({ ...prev, error: (err as Error).message, loading: false }));
    }
  }, [getValidAccessToken]);

  useEffect(() => {
    void load();
    const id = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  return { ...data, reload: () => void load() };
}
