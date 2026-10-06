// Liveness / readiness helpers for the frontend container (used by app/api/{health,healthz,ready,readyz}).
// Liveness never touches other services. Readiness reports the gateway as a non-critical dependency:
// the UI can still render (with friendly error pages) when the API is down, so the status is "degraded", not 503.


const GATEWAY_URL = process.env.GATEWAY_INTERNAL_URL ?? 'http://api-gateway:8080';

export function liveness() {
  return Response.json({ status: 'ok' }, { headers: { 'Cache-Control': 'no-store' } });
}

export async function readiness() {
  const started = Date.now();
  let gateway: { status: 'ok' | 'fail'; latency_ms: number; critical: false; error?: string };
  try {
    const res = await fetch(`${GATEWAY_URL}/health`, { cache: 'no-store', signal: AbortSignal.timeout(1000) });
    gateway = { status: res.ok ? 'ok' : 'fail', latency_ms: Date.now() - started, critical: false, ...(res.ok ? {} : { error: `http_${res.status}` }) };
  } catch {
    gateway = { status: 'fail', latency_ms: Date.now() - started, critical: false, error: 'unreachable' };
  }
  return Response.json(
    {
      status: gateway.status === 'ok' ? 'ready' : 'degraded',
      service: 'frontend',
      version: process.env.SERVICE_VERSION ?? 'dev',
      checks: { 'api-gateway': gateway }
    },
    { headers: { 'Cache-Control': 'no-store' } }
  );
}
