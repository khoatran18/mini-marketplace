'use client';

import { useState } from 'react';
import { formatBucket } from '../../lib/analytics';

export interface ChartPoint {
  bucket: string;
  value: number;
}

interface Props {
  points: ChartPoint[];
  /** full value for tooltip/table */
  format: (value: number) => string;
  /** short value for the y axis */
  formatAxis: (value: number) => string;
  granularity: string;
  label: string;
  height?: number;
}

const W = 640;
const PAD = { top: 12, right: 12, bottom: 28, left: 56 };

/** Dependency-free SVG line chart: one accent colour, axes, hover tooltip and a table alternative. */
export function TimeSeriesChart({ points, format, formatAxis, granularity, label, height = 240 }: Props) {
  const [hover, setHover] = useState<number | null>(null);
  if (points.length === 0) return null;

  const max = Math.max(...points.map((p) => p.value), 1);
  const innerW = W - PAD.left - PAD.right;
  const innerH = height - PAD.top - PAD.bottom;
  const x = (index: number) => PAD.left + (points.length === 1 ? innerW / 2 : (index / (points.length - 1)) * innerW);
  const y = (value: number) => PAD.top + innerH - (value / max) * innerH;
  const path = points.map((point, index) => `${index === 0 ? 'M' : 'L'}${x(index).toFixed(1)},${y(point.value).toFixed(1)}`).join(' ');
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((t) => max * t);
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
                {formatAxis(tick)}
              </text>
            </g>
          ))}
          {points.map((point, index) =>
            index % labelEvery === 0 ? (
              <text key={point.bucket} x={x(index)} y={height - 8} textAnchor="middle" fontSize={11} fill="rgb(var(--muted))">
                {formatBucket(point.bucket, granularity)}
              </text>
            ) : null
          )}
          <path d={path} fill="none" stroke="rgb(var(--brand))" strokeWidth={2.5} strokeLinejoin="round" strokeLinecap="round" />
          {points.length <= 31 ? points.map((point, index) => <circle key={point.bucket} cx={x(index)} cy={y(point.value)} r={hover === index ? 5 : 2.5} fill="rgb(var(--brand))" />) : null}
          {hover !== null ? <line x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + innerH} stroke="rgb(var(--muted))" strokeDasharray="3 3" /> : null}
        </svg>
        {active ? (
          <div className="pointer-events-none absolute right-2 top-2 rounded-lg border border-line bg-surface px-3 py-2 text-xs shadow-lg" role="status">
            <p className="font-semibold text-text">{formatBucket(active.bucket, granularity)}</p>
            <p className="text-text">{format(active.value)}</p>
          </div>
        ) : null}
      </div>
      <details className="text-xs">
        <summary className="cursor-pointer text-muted">Xem dạng bảng</summary>
        <div className="mt-2 max-h-56 overflow-auto">
          <table className="w-full text-left">
            <thead className="text-muted">
              <tr>
                <th className="py-1 pr-3">Thời điểm</th>
                <th className="py-1 text-right">Giá trị</th>
              </tr>
            </thead>
            <tbody>
              {points.map((point) => (
                <tr key={point.bucket} className="border-t border-line">
                  <td className="py-1 pr-3 text-text">{formatBucket(point.bucket, granularity)}</td>
                  <td className="py-1 text-right text-text">{format(point.value)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </figure>
  );
}
