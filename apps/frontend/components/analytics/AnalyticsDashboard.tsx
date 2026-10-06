'use client';

import { useState } from 'react';
import { kindFor, rangeParams, toFunnel, toKpis, toRows, toSeries, type RangeValue } from '../../lib/analytics';
import { AnalyticsPanel } from './AnalyticsPanel';
import { AutoData } from './AutoData';
import { DataTable } from './DataTable';
import { DateRangePicker } from './DateRangePicker';
import { FunnelChart } from './FunnelChart';
import { KpiRow } from './KpiRow';
import { TimeSeriesChart } from './TimeSeriesChart';

const definitions: Record<string, string> = {
  revenue: 'Doanh thu ghi nhận = tổng đơn đã thanh toán, cộng đơn COD đã giao; chưa trừ hoàn tiền. Không gồm đơn chờ thanh toán, đã huỷ hoặc hết hạn.',
  gmv: 'GMV = tổng giá trị đơn đã đặt (kể cả chưa thanh toán), trước khi huỷ/hoàn.',
  orders: 'Số đơn đã đặt trong kỳ (không tính đơn thất bại).',
  aov: 'AOV = doanh thu / số đơn đã tính doanh thu.',
  cancel_rate: 'Tỉ lệ huỷ = đơn bị huỷ hoặc hết hạn / đơn đã đặt.',
  cancellation_rate: 'Tỉ lệ huỷ = đơn bị huỷ hoặc hết hạn / đơn đã đặt.'
};

const select = 'w-auto py-1 text-sm';

const metricOptions = [
  { value: 'revenue', label: 'Doanh thu' },
  { value: 'orders', label: 'Đơn hàng' },
  { value: 'units', label: 'Số lượng bán' },
  { value: 'aov', label: 'AOV' }
];

const topByOptions = [
  { value: 'revenue', label: 'Doanh thu' },
  { value: 'units', label: 'Số lượng' },
  { value: 'views', label: 'Lượt xem' },
  { value: 'conversion', label: 'Chuyển đổi' }
];

function Select({ label, value, options, onChange }: { label: string; value: string; options: { value: string; label: string }[]; onChange: (value: string) => void }) {
  return (
    <label className="flex items-center gap-2 text-xs">
      <span className="whitespace-nowrap">{label}</span>
      <select className={select} value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

/** Seller (/seller/analytics/*) and admin (/admin/analytics/*) dashboards share one layout; the gateway scopes the data by role. */
export function AnalyticsDashboard({ scope }: { scope: 'seller' | 'admin' }) {
  const [range, setRange] = useState<RangeValue>({ period: '7d' });
  const [metric, setMetric] = useState('revenue');
  const [granularity, setGranularity] = useState('day');
  const [topBy, setTopBy] = useState('revenue');
  const params = rangeParams(range);
  const rangeOnly = rangeParams(range, false);

  return (
    <div className="grid gap-6">
      <header className="card grid gap-3">
        <h1 className="text-2xl font-bold text-text">{scope === 'seller' ? 'Phân tích cửa hàng' : 'Phân tích nền tảng'}</h1>
        <DateRangePicker value={range} onChange={setRange} />
      </header>

      <AnalyticsPanel
        title="Tổng quan"
        scope={scope}
        report="summary"
        params={params}
        render={(data) => <KpiRow kpis={toKpis(data)} definitions={definitions} />}
      />

      <AnalyticsPanel
        title="Xu hướng"
        scope={scope}
        report="timeseries"
        params={{ ...params, metric, granularity }}
        definition={definitions[metric]}
        controls={
          <div className="flex flex-wrap gap-3">
            <Select label="Chỉ số" value={metric} options={metricOptions} onChange={setMetric} />
            <Select
              label="Theo"
              value={granularity}
              options={[
                { value: 'hour', label: 'Giờ' },
                { value: 'day', label: 'Ngày' },
                { value: 'week', label: 'Tuần' }
              ]}
              onChange={setGranularity}
            />
          </div>
        }
        render={(data) => {
          const points = toSeries(data, metric);
          return points.length > 0 ? <TimeSeriesChart points={points} kind={kindFor(metric, 1)} granularity={granularity} label={metricOptions.find((option) => option.value === metric)?.label ?? metric} /> : <AutoData data={data} />;
        }}
      />

      <div className="grid gap-6 xl:grid-cols-2">
        <AnalyticsPanel
          title="Sản phẩm hàng đầu"
          scope={scope}
          report="top-products"
          params={{ ...rangeOnly, by: topBy, k: 10 }}
          controls={<Select label="Xếp theo" value={topBy} options={topByOptions} onChange={setTopBy} />}
          render={(data) => <DataTable rows={toRows(data)} caption="Sản phẩm hàng đầu" />}
        />
        <AnalyticsPanel
          title="Phễu xem → giỏ → thanh toán"
          scope={scope}
          report="funnel"
          params={rangeOnly}
          definition="Số phiên đi qua từng bước: xem sản phẩm, thêm vào giỏ, bắt đầu thanh toán, đã thanh toán. Dựa trên sự kiện tracking."
          render={(data) => {
            const steps = toFunnel(data);
            return steps.length > 0 ? <FunnelChart steps={steps} /> : <AutoData data={data} />;
          }}
        />
      </div>

      {scope === 'seller' ? (
        <AnalyticsPanel
          title="Sắp hết hàng"
          scope="seller"
          report="low-stock"
          definition="Sản phẩm có tồn khả dụng ≤ ngưỡng cảnh báo; “ngày còn bán” = tồn / tốc độ bán 14 ngày."
          render={(data) => <DataTable rows={toRows(data)} caption="Sản phẩm sắp hết hàng" />}
          emptyText="Không có sản phẩm nào sắp hết hàng."
        />
      ) : (
        <>
          <div className="grid gap-6 xl:grid-cols-2">
            <AnalyticsPanel title="Traffic" scope="admin" report="traffic" params={rangeOnly} definition="PV = lượt xem trang; Phiên = nhóm hoạt động cách nhau < 30 phút; UV = người dùng duy nhất theo anonymous_id/user_id." render={(data) => <AutoData data={data} />} />
            <AnalyticsPanel title="Thanh toán" scope="admin" report="payments" params={rangeOnly} definition="Tỉ lệ thành công = thanh toán SUCCEEDED / (SUCCEEDED + FAILED). Thanh toán là MÔ PHỎNG." render={(data) => <AutoData data={data} />} />
          </div>
          <AnalyticsPanel title="Chất lượng dữ liệu" scope="admin" report="data-health" definition="Độ trễ pipeline từ event đến kho phân tích, số event/phút và tỉ lệ event bị loại." render={(data) => <AutoData data={data} />} />
        </>
      )}
    </div>
  );
}
