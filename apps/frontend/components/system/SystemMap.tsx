'use client';

import { Background, MarkerType, BackgroundVariant, Controls, Handle, Position, ReactFlow, type Edge, type Node, type NodeProps } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { memo, useMemo } from 'react';
import { edgeHealth, nodeStatus, type EdgeHealth } from '../../lib/systemMapModel';
import { formatRate, STATUS_DOT, STATUS_LABEL, type NodeStatus } from '../../lib/systemStatus';
import { EDGES, NODES, type EdgeLayer, type NodeKind, type TopoNode } from '../../lib/topology';
import type { SystemMapData } from '../../lib/useSystemMap';

interface MapNodeData extends Record<string, unknown> {
  label: string;
  kind: NodeKind;
  status: NodeStatus;
  sub?: string;
}
type MapNode = Node<MapNodeData, 'svc'>;

const kindStyle: Record<NodeKind, string> = {
  client: 'border-dashed border-line bg-surface2 text-muted',
  edge: 'border-line bg-surface text-text',
  gateway: 'border-brand/50 bg-brand-soft text-text',
  service: 'border-line bg-surface text-text',
  data: 'border-line bg-surface2 text-text',
  obs: 'border-line bg-surface2 text-text'
};

const ServiceNode = memo(function ServiceNode({ data, selected }: NodeProps<MapNode>) {
  return (
    <div
      className={`w-[176px] rounded-lg border px-3 py-2 shadow-sm transition ${kindStyle[data.kind]} ${selected ? 'ring-2 ring-brand' : 'hover:border-brand/60'}`}
      title={`${data.label}: ${STATUS_LABEL[data.status]}`}
    >
      <Handle id="tl" type="target" position={Position.Left} className="!h-1.5 !w-1.5 !border-0 !bg-muted/60" />
      <Handle id="sl" type="source" position={Position.Left} className="!h-1.5 !w-1.5 !border-0 !bg-transparent" />
      <div className="flex items-center gap-2">
        <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${STATUS_DOT[data.status]}`} aria-hidden="true" />
        <span className="truncate text-[13px] font-semibold">{data.label}</span>
      </div>
      {data.sub ? <p className="tabular mt-0.5 truncate pl-[18px] text-[11px] text-muted">{data.sub}</p> : null}
      <Handle id="sr" type="source" position={Position.Right} className="!h-1.5 !w-1.5 !border-0 !bg-muted/60" />
      <Handle id="tr" type="target" position={Position.Right} className="!h-1.5 !w-1.5 !border-0 !bg-transparent" />
    </div>
  );
});

const nodeTypes = { svc: ServiceNode };

const edgeColor: Record<EdgeHealth, string> = {
  ok: 'rgb(var(--success))',
  idle: 'rgb(var(--muted) / 0.55)',
  slow: 'rgb(var(--warning))',
  error: 'rgb(var(--danger))',
  unknown: 'rgb(var(--muted) / 0.35)'
};

interface Props {
  data: SystemMapData;
  layers: Set<EdgeLayer>;
  selectedId: string | null;
  onSelect: (id: string | null) => void;
}

export function SystemMap({ data, layers, selectedId, onSelect }: Props) {
  const nodeMap = useMemo(() => new Map(NODES.map((n) => [n.id, n])), []);

  const nodes = useMemo<MapNode[]>(
    () =>
      NODES.map((n: TopoNode) => {
        const m = data.metrics[n.id];
        let sub: string | undefined;
        if (n.app && m?.replicas !== undefined) {
          sub = `${m.replicasUp ?? 0}/${m.replicas} replica`;
          if (m.rate !== undefined && n.id !== 'api-gateway') sub += ` · ${formatRate(m.rate)}`;
          else if (m.rate !== undefined) sub += ` · ${formatRate(m.rate)} HTTP`;
        }
        else if (n.kind === 'client') sub = 'bên ngoài hệ thống';
        else if (!n.app && nodeStatus(n, data) === 'unknown') sub = 'không có metric';
        return {
          id: n.id,
          type: 'svc',
          position: { x: n.x, y: n.y },
          selected: n.id === selectedId,
          data: { label: n.label, kind: n.kind, status: nodeStatus(n, data), sub }
        };
      }),
    [data, selectedId]
  );

  const edges = useMemo<Edge[]>(
    () =>
      EDGES.filter((e) => layers.has(e.layer)).filter((e) => {
        // a Kafka producer and consumer pair (service<->kafka) is drawn once, with arrows at both ends
        const back = EDGES.find((o) => o.from === e.to && o.to === e.from && o.layer === e.layer);
        return !back || e.from < e.to;
      }).map((e) => {
        const twoWay = EDGES.some((o) => o.from === e.to && o.to === e.from && o.layer === e.layer);
        const fx = nodeMap.get(e.from)?.x ?? 0;
        const tx = nodeMap.get(e.to)?.x ?? 0;
        const backwards = fx > tx; // target sits left of the source: leave from the left side, arrive on the right side
        const health = edgeHealth(e, nodeMap, data);
        const m = e.protocol === 'grpc' ? data.metrics[e.to] : undefined;
        const rate = m?.rate;
        const width = 1.4 + Math.min(2.6, Math.log10(1 + (rate ?? 0)) * 1.6);
        const focus = selectedId === e.from || selectedId === e.to;
        const dim = selectedId !== null && !focus;
        return {
          id: e.id,
          source: e.from,
          target: e.to,
          type: 'default',
          sourceHandle: backwards ? 'sl' : 'sr',
          targetHandle: backwards ? 'tr' : 'tl',
          markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14, color: edgeColor[health] },
          markerStart: twoWay ? { type: MarkerType.ArrowClosed, width: 14, height: 14, color: edgeColor[health] } : undefined,
          animated: false,
          className: health === 'ok' && (rate ?? 0) > 0 ? 'mm-edge-flow' : undefined,
          style: { stroke: edgeColor[health], strokeWidth: focus ? width + 1 : width, opacity: dim ? 0.18 : 1 },
          interactionWidth: 14
        };
      }),
    [data, layers, nodeMap, selectedId]
  );

  return (
    <div className="h-[calc(100vh-19rem)] min-h-[520px] w-full overflow-hidden rounded-xl border border-line bg-surface" role="region" aria-label="Bản đồ hệ thống">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.12 }}
        minZoom={0.35}
        maxZoom={1.6}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable
        onNodeClick={(_, node) => onSelect(node.id === selectedId ? null : node.id)}
        onPaneClick={() => onSelect(null)}
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={22} size={1.2} color="rgb(var(--line))" />
        <Controls showInteractive={false} position="bottom-right" />
      </ReactFlow>
    </div>
  );
}
