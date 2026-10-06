'use client';

import Link from 'next/link';
import { useState } from 'react';
import { AnalyticsPanel } from '../../components/analytics/AnalyticsPanel';
import { DateRangePicker } from '../../components/analytics/DateRangePicker';
import { summaryKpis } from '../../components/analytics/AnalyticsDashboard';
import { KpiRow } from '../../components/analytics/KpiRow';
import { StockBadge } from '../../components/shop/StockBadge';
import { ErrorBlock, LoadingBlock } from '../../components/ui/StateBlock';
import { countOrdersRequest, getLowStockRequest } from '../../lib/api';
import { parseSummary, rangeParams, type RangeValue } from '../../lib/analytics';
import { useApiData } from '../../lib/hooks';

export function SellerOverviewClient() {
  const [range, setRange] = useState<RangeValue>({ period: '7d' });
  const counts = useApiData((token) => countOrdersRequest('seller', token as string), []);
  const low = useApiData((token) => getLowStockRequest(token as string), []);
  const by = counts.data?.by_status ?? {};
  const toShip = (by.PAID ?? 0) + (by.CONFIRMED ?? 0);
  const lowProducts = low.data?.products ?? [];

  const tiles = [
    { label: 'Cần giao', value: toShip, href: '/seller/orders', hint: 'Đã thanh toán hoặc COD đã xác nhận' },
    { label: 'Chờ thanh toán', value: by.AWAITING_PAYMENT ?? 0, href: '/seller/orders', hint: 'Khách chưa thanh toán' },
    { label: 'Đang giao', value: by.SHIPPED ?? 0, href: '/seller/orders', hint: 'Chờ xác nhận đã giao' },
    { label: 'Yêu cầu đổi trả', value: by.REFUND_REQUESTED ?? 0, href: '/seller/orders', hint: 'Cần bạn duyệt hoặc từ chối' }
  ];

  return (
    <div className="grid gap-6">
      <header className="card grid gap-3">
        <h1 className="text-2xl font-bold text-text">Tổng quan cửa hàng</h1>
        <DateRangePicker value={range} onChange={setRange} />
      </header>

      <AnalyticsPanel title="Kết quả kinh doanh" scope="seller" report="summary" params={rangeParams(range)} parse={parseSummary} render={(summary) => <KpiRow kpis={summaryKpis(summary)} />} />

      <div className="grid gap-6 lg:grid-cols-[1fr_340px]">
        <section className="card grid content-start gap-4">
          <h2 className="text-lg font-semibold text-text">Đơn hàng cần xử lý</h2>
          {counts.loading ? <LoadingBlock /> : null}
          {counts.error ? <ErrorBlock message={counts.error} onRetry={counts.reload} /> : null}
          {counts.data ? (
            <ul className="grid gap-3 sm:grid-cols-2">
              {tiles.map((tile) => (
                <li key={tile.label}>
                  <Link href={tile.href} className="block rounded-xl border border-line bg-surface2 p-4 transition hover:border-brand">
                    <span className="text-xs font-semibold uppercase tracking-wide text-muted">{tile.label}</span>
                    <span className="block text-3xl font-bold text-text">{tile.value}</span>
                    <span className="text-xs text-muted">{tile.hint}</span>
                  </Link>
                </li>
              ))}
            </ul>
          ) : null}
        </section>

        <aside className="card grid content-start gap-3">
          <h2 className="text-lg font-semibold text-text">Cần chú ý</h2>
          {low.loading ? <LoadingBlock /> : null}
          {low.error ? <p className="text-sm text-danger">{low.error}</p> : null}
          {low.data && lowProducts.length === 0 ? <p className="text-sm text-muted">Không có sản phẩm nào sắp hết hàng.</p> : null}
          <ul className="grid gap-2">
            {lowProducts.slice(0, 6).map((product) => (
              <li key={product.id} className="flex items-center justify-between gap-2 text-sm">
                <Link href={`/seller/products/${product.id}/edit`} className="min-w-0 truncate font-medium text-text hover:underline">
                  {product.name}
                </Link>
                <span className="flex items-center gap-2 whitespace-nowrap">
                  <span className="text-muted">{product.inventory}</span>
                  <StockBadge level={product.stock_level} />
                </span>
              </li>
            ))}
          </ul>
          {lowProducts.length > 0 ? (
            <Link href="/seller/inventory" className="text-sm font-semibold text-brand">
              Quản lý tồn kho →
            </Link>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
