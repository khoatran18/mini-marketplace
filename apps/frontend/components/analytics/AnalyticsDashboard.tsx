'use client';

import { useState } from 'react';
import {
  change,
  funnelLabels,
  parseDataHealth,
  parseFunnel,
  parseLowStock,
  parsePayments,
  parseSearchTerms,
  parseSummary,
  parseTimeseries,
  parseTopProducts,
  parseTraffic,
  rangeParams,
  type KeyCount,
  type LowStockRow,
  type RangeValue,
  type RevenueMetric,
  type Summary,
  type TopSort
} from '../../lib/analytics';
import { formatDateTime, formatNumber, formatPercent, formatVND, formatVNDCompact, formatCompact } from '../../lib/format';
import { paymentMethodLabel } from '../../lib/status';
import { AnalyticsPanel } from './AnalyticsPanel';
import { DataTable, type Column } from './DataTable';
import { DateRangePicker } from './DateRangePicker';
import { FunnelChart } from './FunnelChart';
import { KpiRow, type Kpi } from './KpiRow';
import { TimeSeriesChart } from './TimeSeriesChart';
import { StatusBadge } from '../ui/StatusBadge';

const defs = {
  revenue: 'Doanh thu ghi nhận = đơn đã thanh toán, cộng đơn COD đã giao; chưa trừ hoàn tiền. Không gồm đơn chờ thanh toán, đã huỷ hoặc hết hạn.',
  net: 'Doanh thu thuần = doanh thu − hoàn tiền.',
  gmv: 'GMV = tổng giá trị đơn đã đặt (kể cả chưa thanh toán), trước khi huỷ/hoàn.',
  aov: 'AOV = doanh thu / số đơn đã tính doanh thu.',
  cancel: 'Tỉ lệ huỷ = (đơn bị huỷ + hết hạn) / đơn đã đặt.',
  funnel: 'Số lượt đi qua từng bước: xem sản phẩm → thêm vào giỏ → bắt đầu thanh toán → đã thanh toán. Dựa trên sự kiện tracking.',
  lowStock: 'Sản phẩm có tồn khả dụng thấp; “ngày còn bán” = tồn / tốc độ bán gần đây (– nếu chưa có doanh số).'
};

export function summaryKpis(s: Summary): Kpi[] {
  const c = s.current;
  const p = s.previous;
  return [
    { key: 'revenue', label: 'Doanh thu', value: formatVND(c.revenue), delta: change(c.revenue, p?.revenue), definition: defs.revenue },
    { key: 'net_revenue', label: 'Doanh thu thuần', value: formatVND(c.net_revenue), delta: change(c.net_revenue, p?.net_revenue), definition: defs.net },
    { key: 'gmv', label: 'GMV', value: formatVND(c.gmv), delta: change(c.gmv, p?.gmv), definition: defs.gmv },
    { key: 'orders_recognized', label: 'Đơn tính doanh thu', value: formatNumber(c.orders_recognized), delta: change(c.orders_recognized, p?.orders_recognized) },
    { key: 'orders_placed', label: 'Đơn đã đặt', value: formatNumber(c.orders_placed), delta: change(c.orders_placed, p?.orders_placed) },
    { key: 'aov', label: 'AOV', value: formatVND(c.aov), delta: change(c.aov, p?.aov), definition: defs.aov },
    { key: 'cancel_rate', label: 'Tỉ lệ huỷ', value: formatPercent(c.cancel_rate), delta: change(c.cancel_rate, p?.cancel_rate), goodWhenDown: true, definition: defs.cancel },
    { key: 'refunds', label: 'Hoàn tiền', value: formatVND(c.refunds), delta: change(c.refunds, p?.refunds), goodWhenDown: true }
  ];
}

const selectClass = 'w-auto py-1 text-sm';

function Select<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: { value: T; label: string }[]; onChange: (value: T) => void }) {
  return (
    <label className="flex items-center gap-2 text-xs">
      <span className="whitespace-nowrap">{label}</span>
      <select className={selectClass} value={value} onChange={(event) => onChange(event.target.value as T)}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

const granularityOptions = [
  { value: 'hour', label: 'Giờ' },
  { value: 'day', label: 'Ngày' },
  { value: 'week', label: 'Tuần' },
  { value: 'month', label: 'Tháng' }
];

const metricOptions: { value: RevenueMetric; label: string }[] = [
  { value: 'revenue', label: 'Doanh thu' },
  { value: 'refunds', label: 'Hoàn tiền' },
  { value: 'orders', label: 'Đơn tính doanh thu' },
  { value: 'placed', label: 'Đơn đã đặt' }
];

const topOptions: { value: TopSort; label: string }[] = [
  { value: 'revenue', label: 'Doanh thu' },
  { value: 'units', label: 'Số lượng bán' },
  { value: 'views', label: 'Lượt xem' },
  { value: 'conversion', label: 'Chuyển đổi' },
  { value: 'viewed_unsold', label: 'Xem nhiều chưa bán' }
];

const keyCountColumns = (label: string): Column<KeyCount>[] => [
  { key: 'key', label, render: (row) => row.key || '(trống)' },
  { key: 'count', label: 'Số lượng', numeric: true, render: (row) => formatNumber(row.count) }
];

const lowStockColumns = (withStore: boolean): Column<LowStockRow>[] => [
  { key: 'name', label: 'Sản phẩm', render: (row) => <span>{row.name || `#${row.product_id}`}</span> },
  ...(withStore ? [{ key: 'store', label: 'Cửa hàng', render: (row: LowStockRow) => (row.store_id ? `#${row.store_id}` : '–') }] : []),
  { key: 'available', label: 'Khả dụng', numeric: true, render: (row) => formatNumber(row.available) },
  { key: 'reserved', label: 'Giữ chỗ', numeric: true, render: (row) => formatNumber(row.reserved) },
  { key: 'level', label: 'Mức', render: (row) => <StatusBadge kind="stock" value={row.level} /> },
  { key: 'rate', label: 'Bán/ngày', numeric: true, render: (row) => formatNumber(row.units_per_day) },
  { key: 'days', label: 'Ngày còn bán', numeric: true, render: (row) => (row.days_left === null ? '–' : formatNumber(row.days_left)) }
];
/** Seller (/seller/analytics/*) and admin (/admin/analytics/*) dashboards; the gateway scopes the data by role. */
export function AnalyticsDashboard({ scope }: { scope: 'seller' | 'admin' }) {
  const [range, setRange] = useState<RangeValue>({ period: '7d' });
  const [metric, setMetric] = useState<RevenueMetric>('revenue');
  const [granularity, setGranularity] = useState('day');
  const [topSort, setTopSort] = useState<TopSort>('revenue');
  const [trafficMetric, setTrafficMetric] = useState<'page_views' | 'sessions' | 'visitors'>('page_views');
  const params = rangeParams(range);
  const money = metric === 'revenue' || metric === 'refunds';

  return (
    <div className="grid gap-6">
      <header className="card grid gap-3">
        <h1 className="text-2xl font-bold text-text">{scope === 'seller' ? 'Phân tích cửa hàng' : 'Phân tích nền tảng'}</h1>
        <DateRangePicker value={range} onChange={setRange} />
      </header>

      <AnalyticsPanel title="Tổng quan" scope={scope} report="summary" params={params} parse={parseSummary} render={(summary) => <KpiRow kpis={summaryKpis(summary)} />} />

      <AnalyticsPanel
        title="Xu hướng"
        scope={scope}
        report="timeseries"
        params={{ ...params, granularity }}
        definition={money ? defs.revenue : undefined}
        parse={parseTimeseries}
        isEmpty={(points) => points.length === 0}
        controls={
          <div className="flex flex-wrap gap-3">
            <Select label="Chỉ số" value={metric} options={metricOptions} onChange={setMetric} />
            <Select label="Theo" value={granularity} options={granularityOptions} onChange={setGranularity} />
          </div>
        }
        render={(points) => (
          <TimeSeriesChart
            points={points.map((point) => ({ bucket: point.bucket, value: point[metric] }))}
            format={(value) => (money ? formatVND(value) : formatNumber(value))}
            formatAxis={(value) => (money ? formatVNDCompact(value) : formatCompact(value))}
            granularity={granularity}
            label={metricOptions.find((option) => option.value === metric)?.label ?? metric}
          />
        )}
      />

      <div className="grid gap-6 xl:grid-cols-2">
        <AnalyticsPanel
          title="Sản phẩm hàng đầu"
          scope={scope}
          report="top-products"
          params={{ ...params, sort: topSort, limit: 10 }}
          parse={parseTopProducts}
          isEmpty={(items) => items.length === 0}
          controls={<Select label="Xếp theo" value={topSort} options={topOptions} onChange={setTopSort} />}
          render={(items) => (
            <DataTable
              caption="Sản phẩm hàng đầu"
              rows={items}
              columns={[
                { key: 'name', label: 'Sản phẩm', render: (row) => row.name || `#${row.product_id}` },
                { key: 'units', label: 'Bán', numeric: true, render: (row) => formatNumber(row.units) },
                { key: 'revenue', label: 'Doanh thu', numeric: true, render: (row) => formatVND(row.revenue) },
                { key: 'views', label: 'Xem', numeric: true, render: (row) => formatNumber(row.views) },
                { key: 'carts', label: 'Giỏ', numeric: true, render: (row) => formatNumber(row.carts) },
                { key: 'conversion', label: 'Chuyển đổi', numeric: true, render: (row) => formatPercent(row.conversion) }
              ]}
            />
          )}
        />
        <AnalyticsPanel
          title="Phễu xem → giỏ → thanh toán"
          scope={scope}
          report="funnel"
          params={params}
          definition={defs.funnel}
          parse={parseFunnel}
          isEmpty={(funnel) => funnel.steps.length === 0}
          render={(funnel) => (
            <div className="grid gap-4">
              <FunnelChart steps={funnel.steps} />
              {funnel.unit ? <p className="text-xs text-muted">Đơn vị: {funnel.unit}</p> : null}
              {funnel.by_device.length > 0 ? (
                <DataTable
                  caption="Phễu theo thiết bị"
                  rows={funnel.by_device}
                  columns={[
                    { key: 'device', label: 'Thiết bị', render: (row) => row.device_type || '(không rõ)' },
                    { key: 'views', label: funnelLabels.view, numeric: true, render: (row) => formatNumber(row.views) },
                    { key: 'carts', label: funnelLabels.cart, numeric: true, render: (row) => formatNumber(row.carts) },
                    { key: 'checkouts', label: funnelLabels.checkout, numeric: true, render: (row) => formatNumber(row.checkouts) }
                  ]}
                />
              ) : null}
            </div>
          )}
        />
      </div>

      <AnalyticsPanel
        title="Sắp hết hàng"
        scope={scope}
        report="low-stock"
        definition={defs.lowStock}
        parse={parseLowStock}
        isEmpty={(items) => items.length === 0}
        emptyText="Không có sản phẩm nào sắp hết hàng."
        render={(items) => <DataTable caption="Sản phẩm sắp hết hàng" rows={items} columns={lowStockColumns(scope === 'admin')} />}
      />

      {scope === 'admin' ? (
        <>
          <AnalyticsPanel
            title="Traffic"
            scope="admin"
            report="traffic"
            params={{ ...params, granularity }}
            definition="PV = lượt xem trang; Phiên = nhóm hoạt động cách nhau < 30 phút; Khách = người truy cập duy nhất theo anonymous_id."
            parse={parseTraffic}
            controls={
              <Select
                label="Chỉ số"
                value={trafficMetric}
                options={[
                  { value: 'page_views', label: 'Lượt xem trang' },
                  { value: 'sessions', label: 'Phiên' },
                  { value: 'visitors', label: 'Khách' }
                ]}
                onChange={setTrafficMetric}
              />
            }
            render={(t) => (
              <div className="grid gap-5">
                <KpiRow
                  kpis={[
                    { key: 'pv', label: 'Lượt xem trang', value: formatNumber(t.page_views) },
                    { key: 'sessions', label: 'Phiên', value: formatNumber(t.sessions) },
                    { key: 'visitors', label: 'Khách', value: formatNumber(t.visitors) },
                    { key: 'new', label: 'Khách mới', value: formatNumber(t.new_visitors) },
                    { key: 'returning', label: 'Khách quay lại', value: formatNumber(t.returning_visitors) },
                    { key: 'errors', label: 'Lỗi phía client', value: formatNumber(t.client_errors), goodWhenDown: true }
                  ]}
                />
                <TimeSeriesChart
                  points={t.series.map((point) => ({ bucket: point.bucket, value: point[trafficMetric] }))}
                  format={formatNumber}
                  formatAxis={formatCompact}
                  granularity={granularity}
                  label="Traffic"
                />
                <div className="grid gap-5 lg:grid-cols-2">
                  <DataTable caption="Trang xem nhiều" rows={t.top_paths} columns={keyCountColumns('Trang')} />
                  <DataTable caption="Nguồn truy cập" rows={t.top_referrers} columns={keyCountColumns('Nguồn')} />
                  <DataTable caption="Thiết bị" rows={t.devices} columns={keyCountColumns('Thiết bị')} />
                  <DataTable caption="Quốc gia" rows={t.countries} columns={keyCountColumns('Quốc gia')} />
                </div>
              </div>
            )}
          />

          <AnalyticsPanel
            title="Thanh toán (mô phỏng)"
            scope="admin"
            report="payments"
            params={params}
            definition="Tỉ lệ thành công = thanh toán thành công / (thành công + thất bại). Mọi thanh toán đều là MÔ PHỎNG."
            parse={parsePayments}
            render={(p) => (
              <div className="grid gap-5">
                <KpiRow
                  kpis={[
                    { key: 'succeeded', label: 'Thành công', value: formatNumber(p.succeeded) },
                    { key: 'failed', label: 'Thất bại', value: formatNumber(p.failed), goodWhenDown: true },
                    { key: 'rate', label: 'Tỉ lệ thành công', value: formatPercent(p.success_rate) },
                    { key: 'amount', label: 'Số tiền thu', value: formatVND(p.amount_succeeded) },
                    { key: 'refunds', label: 'Hoàn tiền', value: `${formatNumber(p.refunds.count)} · ${formatVND(p.refunds.amount)}` },
                    { key: 'awaiting', label: 'Đơn chờ thanh toán', value: formatNumber(p.orders_awaiting_payment) }
                  ]}
                />
                <div className="grid gap-5 lg:grid-cols-2">
                  <DataTable
                    caption="Theo phương thức"
                    rows={p.by_method}
                    columns={[
                      { key: 'method', label: 'Phương thức', render: (row) => paymentMethodLabel(row.method) },
                      { key: 'ok', label: 'Thành công', numeric: true, render: (row) => formatNumber(row.succeeded) },
                      { key: 'fail', label: 'Thất bại', numeric: true, render: (row) => formatNumber(row.failed) },
                      { key: 'rate', label: 'Tỉ lệ', numeric: true, render: (row) => formatPercent(row.success_rate) },
                      { key: 'amount', label: 'Số tiền', numeric: true, render: (row) => formatVND(row.amount) }
                    ]}
                  />
                  <DataTable caption="Mã lỗi hàng đầu" rows={p.failure_codes} columns={keyCountColumns('Mã lỗi')} />
                </div>
              </div>
            )}
          />

          <AnalyticsPanel
            title="Từ khoá tìm kiếm"
            scope="admin"
            report="search-terms"
            params={{ ...params, limit: 20 }}
            parse={parseSearchTerms}
            isEmpty={(s) => s.top.length === 0 && s.zero_results.length === 0}
            render={(s) => (
              <div className="grid gap-5 lg:grid-cols-2">
                <div className="grid gap-2">
                  <h3 className="text-sm font-semibold text-text">Tìm nhiều nhất</h3>
                  <DataTable
                    caption="Từ khoá tìm nhiều"
                    rows={s.top}
                    columns={[
                      { key: 'term', label: 'Từ khoá', render: (row) => row.term },
                      { key: 'count', label: 'Lượt', numeric: true, render: (row) => formatNumber(row.count) },
                      { key: 'avg', label: 'KQ trung bình', numeric: true, render: (row) => formatNumber(row.avg_results) }
                    ]}
                  />
                </div>
                <div className="grid gap-2">
                  <h3 className="text-sm font-semibold text-text">Không có kết quả</h3>
                  <DataTable
                    caption="Từ khoá không có kết quả"
                    rows={s.zero_results}
                    columns={[
                      { key: 'term', label: 'Từ khoá', render: (row) => row.term },
                      { key: 'count', label: 'Lượt', numeric: true, render: (row) => formatNumber(row.count) }
                    ]}
                  />
                </div>
              </div>
            )}
          />

          <AnalyticsPanel
            title="Chất lượng dữ liệu"
            scope="admin"
            report="data-health"
            definition="Số dòng, thời điểm event gần nhất và độ trễ trung bình 15 phút của từng bảng phân tích."
            parse={parseDataHealth}
            render={(h) => (
              <div className="grid gap-5">
                <DataTable
                  caption="Bảng dữ liệu"
                  rows={h.tables}
                  columns={[
                    { key: 'table', label: 'Bảng', render: (row) => row.table },
                    { key: 'rows', label: 'Số dòng', numeric: true, render: (row) => formatNumber(row.rows) },
                    { key: 'last', label: 'Event gần nhất', render: (row) => formatDateTime(row.last_event_at) },
                    { key: 'lag', label: 'Trễ TB 15 phút (giây)', numeric: true, render: (row) => (row.avg_lag_seconds_15m === null ? '–' : formatNumber(row.avg_lag_seconds_15m)) }
                  ]}
                />
                <div className="grid gap-2">
                  <h3 className="text-sm font-semibold text-text">Event trong 1 giờ qua theo loại</h3>
                  <DataTable caption="Event theo loại" rows={h.events_last_hour_by_type} columns={keyCountColumns('Loại event')} />
                </div>
                <p className="text-xs text-muted">Đối soát với nguồn: {h.reconciliation === 'not_implemented' ? 'chưa được triển khai' : h.reconciliation || '–'}</p>
              </div>
            )}
          />
        </>
      ) : null}
    </div>
  );
}
