'use client';

import { useEffect, useMemo, useState } from 'react';
import { useAuth } from '../../components/auth/AuthProvider';
import { getOrdersByBuyerStatusRequest } from '../../lib/api';
import type { Order, OrderStatus } from '../../lib/types';

const STATUS_META: Record<OrderStatus, { label: string; description: string; badgeClass: string }> = {
  PENDING: {
    label: 'Đang chờ',
    description: 'Các đơn hàng đang được xử lý.',
    badgeClass: 'bg-amber-100 text-amber-700'
  },
  FAILED: {
    label: 'Thất bại',
    description: 'Đơn hàng không thể hoàn tất.',
    badgeClass: 'bg-rose-100 text-rose-700'
  },
  SUCCESS: {
    label: 'Thành công',
    description: 'Đơn hàng đã thanh toán thành công.',
    badgeClass: 'bg-emerald-100 text-emerald-700'
  }
};

const statusOptions: OrderStatus[] = ['PENDING', 'FAILED', 'SUCCESS'];

const currencyFormatter = new Intl.NumberFormat('vi-VN', {
  style: 'currency',
  currency: 'VND'
});

function toNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function formatCurrency(value: unknown): string {
  const numeric = toNumber(value);
  if (numeric === null) {
    return 'Không xác định';
  }
  return currencyFormatter.format(numeric);
}

export function OrdersPageClient() {
  const { userId, getValidAccessToken } = useAuth();
  const [selectedStatus, setSelectedStatus] = useState<OrderStatus>('PENDING');
  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      if (!userId) {
        if (!cancelled) {
          setOrders([]);
          setError('Không tìm thấy thông tin người dùng. Vui lòng đăng nhập lại.');
          setLoading(false);
        }
        return;
      }

      if (!cancelled) {
        setLoading(true);
        setError(null);
      }

      try {
        const token = await getValidAccessToken();
        if (!token) {
          throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
        }
        const response = await getOrdersByBuyerStatusRequest(userId, selectedStatus, token);
        if (!cancelled) {
          setOrders(response.orders ?? []);
        }
      } catch (err) {
        if (!cancelled) {
          setError((err as Error).message);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, [selectedStatus, userId, getValidAccessToken]);

  const statusDescription = useMemo(() => STATUS_META[selectedStatus]?.description ?? '', [selectedStatus]);

  return (
    <div className="grid gap-6">
      <header className="card space-y-4">
        <div className="grid gap-1">
          <h1 className="text-2xl font-bold text-slate-900">Đơn hàng của bạn</h1>
          <p className="text-sm text-slate-600">Chọn trạng thái để xem các đơn hàng tương ứng.</p>
        </div>
        <div className="flex flex-wrap gap-3">
          {statusOptions.map((status) => {
            const isActive = status === selectedStatus;
            const meta = STATUS_META[status];
            return (
              <button
                key={status}
                type="button"
                onClick={() => setSelectedStatus(status)}
                className={`rounded-full px-4 py-2 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 ${
                  isActive
                    ? 'bg-indigo-600 text-white shadow-lg shadow-indigo-200 focus-visible:outline-indigo-600'
                    : 'border border-slate-300 text-slate-700 hover:bg-slate-100 focus-visible:outline-slate-400'
                }`}
                aria-pressed={isActive}
              >
                {meta.label}
              </button>
            );
          })}
        </div>
        <p className="text-sm text-slate-500">{statusDescription}</p>
      </header>

      {loading ? <p className="text-sm text-slate-600">Đang tải đơn hàng...</p> : null}
      {error ? (
        <div className="card border border-rose-200 bg-rose-50 text-sm font-medium text-rose-700">
          <strong className="font-semibold">Lỗi:</strong> {error}
        </div>
      ) : null}

      <div className="grid gap-5">
        {orders.map((order, index) => {
          const normalizedStatus = typeof order.status === 'string'
            ? (order.status.toUpperCase() as OrderStatus)
            : undefined;
          const status = normalizedStatus && STATUS_META[normalizedStatus] ? normalizedStatus : selectedStatus;
          const meta = STATUS_META[status];
          const total = formatCurrency(order.total_price);
          const items = Array.isArray(order.order_items) ? order.order_items : [];

          return (
            <article key={order.id ?? `${status}-${index}`} className="card space-y-4">
              <header className="flex flex-wrap items-center justify-between gap-3">
                <div className="grid gap-1">
                  <h2 className="text-lg font-semibold text-slate-900">
                    Đơn hàng #{order.id ?? '—'}
                  </h2>
                  <p className="text-sm text-slate-500">Tổng tiền: {total}</p>
                </div>
                <span className={`rounded-full px-3 py-1 text-sm font-semibold ${meta.badgeClass}`}>{meta.label}</span>
              </header>

              <div className="grid gap-3">
                {items.length === 0 ? (
                  <p className="rounded-xl border border-dashed border-slate-300 p-4 text-sm text-slate-500">
                    Đơn hàng này chưa có thông tin sản phẩm.
                  </p>
                ) : (
                  items.map((item) => {
                    const unitPrice = formatCurrency(item.price);
                    const quantityNumber = toNumber(item.quantity) ?? 0;
                    const itemTotal = formatCurrency((toNumber(item.price) ?? 0) * quantityNumber);
                    return (
                      <div
                        key={`${order.id ?? 'order'}-${item.product_id}-${item.id ?? 'item'}`}
                        className="rounded-xl border border-slate-200 p-4"
                      >
                        <div className="flex flex-wrap items-start justify-between gap-2">
                          <div>
                            <p className="text-sm font-semibold text-slate-800">
                              {item.name ?? `Sản phẩm #${item.product_id}`}
                            </p>
                            <p className="text-xs text-slate-500">Mã sản phẩm: {item.product_id}</p>
                          </div>
                          <div className="text-right text-sm text-slate-600">
                            <p>Đơn giá: {unitPrice}</p>
                            <p>Số lượng: {quantityNumber}</p>
                            <p className="font-semibold text-slate-800">Thành tiền: {itemTotal}</p>
                          </div>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </article>
          );
        })}

        {!loading && !error && orders.length === 0 ? (
          <div className="card text-sm text-slate-600">
            Không có đơn hàng nào trong trạng thái này.
          </div>
        ) : null}
      </div>
    </div>
  );
}
