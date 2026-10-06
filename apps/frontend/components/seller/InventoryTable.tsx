'use client';

import { useState } from 'react';
import { adjustInventoryRequest, getInventoryLedgerRequest, getLowStockRequest, getSellerProductsRequest } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import { useApiData, useAuthedAction } from '../../lib/hooks';
import type { Product } from '../../lib/types';
import { StockBadge } from '../shop/StockBadge';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../ui/StateBlock';
import { Pagination } from '../ui/Pagination';
import { Tabs } from '../ui/Tabs';

const PAGE_SIZE = 25;

function AdjustForm({ product, onDone }: { product: Product; onDone: (message: string) => void }) {
  const run = useAuthedAction();
  const [delta, setDelta] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  return (
    <form
      className="grid gap-2 sm:grid-cols-[120px_1fr_auto]"
      onSubmit={(event) => {
        event.preventDefault();
        const value = Number(delta);
        if (!Number.isInteger(value) || value === 0) return setError('Số lượng thay đổi phải là số nguyên khác 0 (âm để trừ).');
        setBusy(true);
        setError(null);
        void run((token) => adjustInventoryRequest(product.id as number, value, reason.trim(), token))
          .then((res) => onDone(`Đã điều chỉnh ${product.name}: tồn kho hiện tại ${res.inventory ?? '?'}.`))
          .catch((err) => setError((err as Error).message))
          .finally(() => setBusy(false));
      }}
    >
      <input type="number" step={1} aria-label="Thay đổi (+/-)" placeholder="+10 / -3" value={delta} onChange={(event) => setDelta(event.target.value)} required />
      <input aria-label="Lý do (bắt buộc)" placeholder="Lý do (VD: nhập thêm hàng, kiểm kê)" value={reason} onChange={(event) => setReason(event.target.value)} required />
      <button type="submit" className="btn-primary" disabled={busy}>
        {busy ? 'Đang lưu…' : 'Điều chỉnh'}
      </button>
      {error ? (
        <div className="sm:col-span-3">
          <Notice tone="error">{error}</Notice>
        </div>
      ) : null}
    </form>
  );
}

function Ledger({ productId }: { productId: number }) {
  const ledger = useApiData((token) => getInventoryLedgerRequest(productId, token as string), [productId]);
  if (ledger.loading) return <LoadingBlock label="Đang tải lịch sử…" />;
  if (ledger.error) return <ErrorBlock message={ledger.error} onRetry={ledger.reload} />;
  const entries = ledger.data?.entries ?? [];
  if (entries.length === 0) return <p className="text-sm text-muted">Chưa có biến động tồn kho.</p>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-xs">
        <thead className="text-muted">
          <tr>
            <th className="py-1 pr-3">Thời gian</th>
            <th className="py-1 pr-3">Lý do</th>
            <th className="py-1 pr-3 text-right">Thay đổi</th>
            <th className="py-1 pr-3 text-right">Tồn sau</th>
            <th className="py-1">Tham chiếu</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => (
            <tr key={entry.id} className="border-t border-line">
              <td className="py-1 pr-3 text-muted">{formatDateTime(entry.at)}</td>
              <td className="py-1 pr-3 text-text">{entry.reason}</td>
              <td className={`py-1 pr-3 text-right font-semibold ${entry.delta < 0 ? 'text-danger' : 'text-success'}`}>{entry.delta > 0 ? `+${entry.delta}` : entry.delta}</td>
              <td className="py-1 pr-3 text-right text-text">{entry.balance_after}</td>
              <td className="py-1 text-muted">{entry.ref_type ? `${entry.ref_type} ${entry.ref_id}` : '–'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Stock per product with manual adjustment (a reason is mandatory, every change is written to the ledger). */
export function InventoryTable() {
  const [tab, setTab] = useState<'all' | 'low'>('all');
  const [page, setPage] = useState(1);
  const [open, setOpen] = useState<{ id: number; mode: 'adjust' | 'ledger' } | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const all = useApiData((token) => getSellerProductsRequest({ page, page_size: PAGE_SIZE, sort: 'newest' }, token as string), [page], { enabled: tab === 'all' });
  const low = useApiData((token) => getLowStockRequest(token as string), [], { enabled: tab === 'low' });
  const current = tab === 'all' ? all : low;
  const products = tab === 'all' ? all.data?.products ?? [] : low.data?.products ?? [];

  return (
    <div className="grid gap-4">
      <Tabs
        label="Phạm vi"
        value={tab}
        onChange={(value) => {
          setTab(value as 'all' | 'low');
          setOpen(null);
        }}
        items={[
          { value: 'all', label: 'Tất cả sản phẩm' },
          { value: 'low', label: 'Tồn thấp' }
        ]}
      />
      {message ? <Notice tone="success">{message}</Notice> : null}
      {current.loading ? <LoadingBlock label="Đang tải tồn kho…" /> : null}
      {current.error ? <ErrorBlock message={current.error} onRetry={current.reload} /> : null}
      {!current.loading && !current.error && products.length === 0 ? <EmptyBlock>{tab === 'low' ? 'Không có sản phẩm nào sắp hết hàng.' : 'Chưa có sản phẩm.'}</EmptyBlock> : null}

      <ul className="grid gap-3">
        {products.map((product) => {
          const id = product.id as number;
          const isOpen = open?.id === id;
          return (
            <li key={id} className="card grid gap-3 p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-semibold text-text">{product.name}</p>
                  <p className="text-xs text-muted">#{id}{product.sku ? ` · ${product.sku}` : ''}</p>
                </div>
                <div className="flex flex-wrap items-center gap-4 text-sm">
                  <span className="text-text">
                    Khả dụng: <strong>{product.inventory}</strong>
                  </span>
                  <span className="text-muted">Giữ chỗ: {product.reserved ?? 0}</span>
                  <span className="text-muted">Ngưỡng: {product.low_stock_threshold || 'mặc định'}</span>
                  <StockBadge level={product.stock_level} />
                  <button type="button" className="btn" aria-expanded={isOpen && open?.mode === 'adjust'} onClick={() => setOpen(isOpen && open?.mode === 'adjust' ? null : { id, mode: 'adjust' })}>
                    Điều chỉnh
                  </button>
                  <button type="button" className="btn" aria-expanded={isOpen && open?.mode === 'ledger'} onClick={() => setOpen(isOpen && open?.mode === 'ledger' ? null : { id, mode: 'ledger' })}>
                    Lịch sử
                  </button>
                </div>
              </div>
              {isOpen && open?.mode === 'adjust' ? (
                <AdjustForm
                  product={product}
                  onDone={(text) => {
                    setMessage(text);
                    setOpen(null);
                    current.reload();
                  }}
                />
              ) : null}
              {isOpen && open?.mode === 'ledger' ? <Ledger productId={id} /> : null}
            </li>
          );
        })}
      </ul>
      {tab === 'all' ? <Pagination page={page} pageSize={PAGE_SIZE} total={all.data?.total ?? 0} onChange={setPage} /> : null}
    </div>
  );
}
