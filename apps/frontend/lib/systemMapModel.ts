import type { SystemMapData } from './useSystemMap';
import type { NodeStatus } from './systemStatus';
import type { TopoEdge, TopoNode } from './topology';

type Snapshot = Pick<SystemMapData, 'health' | 'infraUp' | 'metrics'>;

/** Status of one node: /ready for app services, Prometheus `up` for infrastructure, unknown for the rest. */
export function nodeStatus(node: TopoNode, data: Snapshot): NodeStatus {
  if (node.app) {
    const health = data.health[node.id];
    return health ? health.status : 'unknown';
  }
  const key = node.job ?? (node.id === 'prometheus' ? 'prometheus' : undefined);
  if (key && key in data.infraUp) return data.infraUp[key] ? 'ready' : 'not_ready';
  return 'unknown';
}

export type EdgeHealth = 'ok' | 'idle' | 'slow' | 'error' | 'unknown';

/** Colour state of an edge, judged on its target (and, for gateway→service, on the gRPC metrics). */
export function edgeHealth(edge: TopoEdge, nodes: Map<string, TopoNode>, data: Snapshot): EdgeHealth {
  const target = nodes.get(edge.to);
  const source = nodes.get(edge.from);
  if (!target || !source) return 'unknown';
  const sourceStatus = nodeStatus(source, data);
  const targetStatus = nodeStatus(target, data);
  if (targetStatus === 'not_ready' || targetStatus === 'unreachable' || sourceStatus === 'unreachable') return 'error';
  const m = edge.protocol === 'grpc' ? data.metrics[edge.to] : undefined;
  if (m) {
    if ((m.errorRate ?? 0) > 0.01) return 'error';
    if ((m.p95 ?? 0) > 0.5 || targetStatus === 'degraded') return 'slow';
    return (m.rate ?? 0) > 0 ? 'ok' : 'idle';
  }
  if (targetStatus === 'degraded') return 'slow';
  if (targetStatus === 'unknown') return 'unknown';
  return 'ok';
}
