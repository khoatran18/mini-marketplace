'use client';

import { useState } from 'react';
import { formatX, type SeriesPoint, type ValueKind, formatByKind } from '../../lib/analytics';
import { formatCompact, formatVNDCompact } from '../../lib/format';

interface Props {
  points: SeriesPoint[];
  kind?: ValueKind;
  granularity?: string;
  label: string;
  height?: number;
}

const W = 640;
const PAD = { top: 12, right: 12, bottom: 28, left: 56 };

/** Dependency-free SVG line chart: one accent colour, previous period dashed grey, axes, hover tooltip and a data table. */
export function TimeSeriesChart({ points, kind = 'number', granularity, label, height = 240 }: Props) {
  const [hover, setHover] = useState<number | null>(null);
  if (points.length === 0) return null;

  const values = points.flatMap((point) => (point.prev !== undefined ? [point.y, point.prev] : [point.y]));
  const max = Math.max(...values, 1);
  const min = Math.min(0, ...values);
  const innerW = W - PAD.left - PAD.right;
  const innerH = height - PAD.top - PAD.bottom;
  const x = (index: number) => PAD.left + (points.length === 1 ? innerW / 2 : (index / (points.length - 1)) * innerW);
  const y = (value: number) => PAD.top + innerH - ((value - min) / (max - min || 1)) * innerH;
  const path = (pick: (point: SeriesPoint) => number | undefined) =>
    points
      .map((point, index) => ({ index, value: pick(point) }))
      .filter((item): item is { index: number; value: number } => item.value !== undefined)
      .map((item, i) => `${i === 0 ? 'M' : 'L'}${x(item.index).toFixed(1)},${y(item.value).toFixed(1)}`)
      .join(' ');
  const hasPrev = points.some((point) => point.prev !== undefined);
  const axis = (value: number) => (kind === 'money' ? formatVNDCompact(value) : kind === 'percent' ? `${(value * (Math.abs(max) <= 1 ? 100 : 1)).toFixed(0)}%` : formatCompact(value));
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((t) => min + (max - min) * t);
  const labelEvery = Math.max(1, Math.ceil(points.length / 6));
  const active = hover !== null ? points[hover] : null;

  return (
    <figure className="grid gap-2">
      <div className="relative">
        <svg
          viewBox={`0 0 ${W} ${height}`}
          role="img"
          aria-label={`${label}: ${points.length} điểm dữ liệu`}
          className="h-auto w-full"
          onMouseLeave={() => setHover(null)}
          onMouseMove={(event) => {
            const rect = event.currentTarget.getBoundingClientRect();
            const px = ((event.clientX - rect.left) / rect.width) * W;
            const index = Math.round(((px - PAD.left) / innerW) * (points.length - 1));
            setHover(Math.max(0, Math.min(points.length - 1, index)));
          }}
        >
          {ticks.map((tick) => (
            <g key={tick}>
              <line x1={PAD.left} x2={W - PAD.right} y1={y(tick)} y2={y(tick)} stroke="rgb(var(--line))" strokeWidth={1} />
              <text x={PAD.left - 6} y={y(tick) + 4} textAnchor="end" fontSize={11} fill="rgb(var(--muted))">
                {axis(tick)}
              </text>
            </g>
          ))}
          {points.map((point, index) =>
            index % labelEvery === 0 ? (
              <text key={point.x + index} x={x(index)} y={height - 8} textAnchor="middle" fontSize={11} fill="rgb(var(--muted))">
                {formatX(point.x, granularity)}
              </text>
            ) : null
          )}
          {hasPrev ? <path d={path((point) => point.prev)} fill="none" stroke="rgb(var(--muted))" strokeWidth={1.5} strokeDasharray="5 4" opacity={0.8} /> : null}
          <path d={path((point) => point.y)} fill="none" stroke="rgb(var(--brand))" strokeWidth={2.5} strokeLinejoin="round" strokeLinecap="round" />
          {points.length <= 31 ? points.map((point, index) => <circle key={index} cx={x(index)} cy={y(point.y)} r={hover === index ? 5 : 2.5} fill="rgb(var(--brand))" />) : null}
          {hover !== null ? <line x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + innerH} stroke="rgb(var(--muted))" strokeDasharray="3 3" /> : null}
        </svg>
        {active ? (
          <div className="pointer-events-none absolute right-2 top-2 rounded-lg border border-line bg-surface px-3 py-2 text-xs shadow-lg" role="status">
            <p className="font-semibold text-text">{formatX(active.x, granularity)}</p>
            <p className="text-text">{formatByKind(active.y, kind)}</p>
            {active.prev !== undefined ? <p className="text-muted">Kỳ trước: {formatByKind(active.prev, kind)}</p> : null}
          </div>
        ) : null}
      </div>
      <figcaption className="flex flex-wrap items-center gap-4 text-xs text-muted">
        <span className="inline-flex items-center gap-1">
          <span aria-hidden="true" className="inline-block h-0.5 w-4 bg-brand" /> Kỳ này
        </span>
        {hasPrev ? (
          <span className="inline-flex items-center gap-1">
            <span aria-hidden="true" className="inline-block w-4 border-t-2 border-dashed border-muted" /> Kỳ trước
          </span>
        ) : null}
      </figcaption>
      <details className="text-xs">
        <summary className="cursor-pointer text-muted">Xem dạng bảng</summary>
        <div className="mt-2 max-h-56 overflow-auto">
          <table className="w-full text-left">
            <thead className="text-muted">
              <tr>
                <th className="py-1 pr-3">Thời điểm</th>
                <th className="py-1 pr-3 text-right">Giá trị</th>
                {hasPrev ? <th className="py-1 text-right">Kỳ trước</th> : null}
              </tr>
            </thead>
            <tbody>
              {points.map((point, index) => (
                <tr key={index} className="border-t border-line">
                  <td className="py-1 pr-3 text-text">{formatX(point.x, granularity)}</td>
                  <td className="py-1 pr-3 text-right text-text">{formatByKind(point.y, kind)}</td>
                  {hasPrev ? <td className="py-1 text-right text-muted">{point.prev !== undefined ? formatByKind(point.prev, kind) : '–'}</td> : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </figure>
  );
}
