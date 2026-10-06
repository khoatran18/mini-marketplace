'use client';

import type { ReactNode } from 'react';
import { getAnalyticsRequest } from '../../lib/api';
import { isEmptyData, unwrap } from '../../lib/analytics';
import { useApiData } from '../../lib/hooks';
import { AsOfBadge } from './AsOfBadge';
import { DefinitionTooltip } from './DefinitionTooltip';

interface Props {
  title: string;
  scope: 'seller' | 'admin';
  report: string;
  params?: Record<string, string | number | undefined>;
  /** plain-language definition shown next to the title */
  definition?: string;
  /** controls rendered in the header (selectors) */
  controls?: ReactNode;
  render: (data: unknown, meta: { asOf?: string; source?: string }) => ReactNode;
  emptyText?: string;
}

/**
 * One analytics region with the four states of the UI guide: loading, error, empty and stale (AsOfBadge).
 * The analytics endpoints are being built in parallel: 404/5xx are shown as a friendly "not available yet", never as a crash.
 */
export function AnalyticsPanel({ title, scope, report, params = {}, definition, controls, render, emptyText = 'Chưa có dữ liệu cho khoảng thời gian này.' }: Props) {
  const state = useApiData((token) => getAnalyticsRequest(scope, report, params, token as string), [scope, report, JSON.stringify(params)]);
  const { data, asOf, source } = unwrap(state.data);

  return (
    <section className="card grid content-start gap-4" aria-busy={state.loading}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="grid gap-1">
          <h2 className="flex items-center gap-1 text-lg font-semibold text-text">
            {title}
            {definition ? <DefinitionTooltip>{definition}</DefinitionTooltip> : null}
          </h2>
          {state.data ? <AsOfBadge asOf={asOf} source={source} /> : null}
        </div>
        {controls}
      </header>

      {state.loading && !state.data ? <p role="status" className="text-sm text-muted">Đang tải…</p> : null}
      {state.error ? (
        <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-danger-soft px-4 py-3 text-sm text-danger">
          <span>
            {state.status === 404 || state.status === 501
              ? 'Chức năng phân tích này chưa sẵn sàng trên máy chủ.'
              : `Không tải được dữ liệu: ${state.error}`}
          </span>
          <button type="button" className="btn" onClick={state.reload}>
            Thử lại
          </button>
        </div>
      ) : null}
      {!state.loading && !state.error && state.data && isEmptyData(data) ? <p className="text-sm text-muted">{emptyText}</p> : null}
      {!state.error && state.data && !isEmptyData(data) ? render(data, { asOf, source }) : null}
    </section>
  );
}
