'use client';

import Link from 'next/link';
import { useState } from 'react';
import { countOrdersRequest, listOrdersRequest, orderActionRequest } from '../../lib/api';
import { formatDateTime, formatVND } from '../../lib/format';
import { useApiData, useAuthedAction } from '../../lib/hooks';
import { paymentMethodLabel, sellerCanShip } from '../../lib/status';
import type { OrdersScope } from '../../lib/types';
import { ShipDialog } from '../seller/ShipDialog';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../ui/StateBlock';
import { Pagination } from '../ui/Pagination';
import { StatusBadge } from '../ui/StatusBadge';
import { Tabs, type TabItem } from '../ui/Tabs';

const PAGE_SIZE = 20;

const tabsByScope: Record<OrdersScope, { value: string; label: string }[]> = {
  buyer: [
    { value: '', label: 'Tất cả' },
    { value: 'AWAITING_PAYMENT', label: 'Chờ thanh toán' },
    { value: 'PAID', label: 'Đã thanh toán' },
    { value: 'CONFIRMED', label: 'Đã xác nhận' },
    { value: 'SHIPPED', label: 'Đang giao' },
    { value: 'DELIVERED', label: 'Đã giao' },
    { value: 'REFUND_REQUESTED', label: 'Đổi trả' },
    { value: 'CANCELED', label: 'Đã huỷ' },
    { value: 'EXPIRED', label: 'Hết hạn' }
  ],
  seller: [
    { value: '', label: 'Tất cả' },
    { value: 'PAID', label: 'Cần giao (đã thanh toán)' },
    { value: 'CONFIRMED', label: 'Cần giao (COD)' },
    { value: 'AWAITING_PAYMENT', label: 'Chờ thanh toán' },
    { value: 'SHIPPED', label: 'Đang giao' },
    { value: 'DELIVERED', label: 'Hoàn tất' },
    { value: 'REFUND_REQUESTED', label: 'Đổi trả' },
    { value: 'CANCELED', label: 'Đã huỷ' }
  ],
  admin: [
    { value: '', label: 'Tất cả' },
    { value: 'AWAITING_PAYMENT', label: 'Chờ thanh toán' },
    { value: 'PAID', label: 'Đã thanh toán' },
    { value: 'CONFIRMED', label: 'COD đã xác nhận' },
    { value: 'SHIPPED', label: 'Đang giao' },
    { value: 'DELIVERED', label: 'Đã giao' },
    { value: 'REFUND_REQUESTED', label: 'Đổi trả' },
    { value: 'REFUNDED', label: 'Đã hoàn tiền' },
    { value: 'CANCELED', label: 'Đã huỷ' },
    { value: 'EXPIRED', label: 'Hết hạn' },
    { value: 'FAILED', label: 'Thất bại' }
  ]
};

const detailBase: Record<OrdersScope, string> = { buyer: '/orders', seller: '/seller/orders', admin: '/admin/orders' };

export function OrdersList({ scope }: { scope: OrdersScope }) {
  const run = useAuthedAction();
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [shipping, setShipping] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);

  const list = useApiData((token) => listOrdersRequest(scope, { status: status || undefined, page, page_size: PAGE_SIZE }, token as string), [scope, status, page]);
  // counts per status (the admin route has no summary)
  const counts = useApiData((token) => countOrdersRequest(scope as 'buyer' | 'seller', token as string), [scope, list.data], { enabled: scope !== 'admin' });

  const tabs: TabItem[] = tabsByScope[scope].map((tab) => ({
    ...tab,
    count: scope === 'admin' ? undefined : tab.value ? counts.data?.by_status?.[tab.value] ?? 0 : undefined
  }));

  const orders = list.data?.orders ?? [];

  const ship = async (orderId: number, carrier: string, trackingCode: string) => {
    setBusy(true);
    setMessage(null);
    try {
      await run((token) => orderActionRequest('seller', orderId, 'ship', { carrier, tracking_code: trackingCode }, token));
      setShipping(null);
      setMessage({ tone: 'success', text: `Đã chuyển đơn #${orderId} sang trạng thái đang giao.` });
      list.reload();
    } catch (err) {
      setMessage({ tone: 'error', text: (err as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="grid gap-5">
      <Tabs
        label="Lọc theo trạng thái"
        items={tabs}
        value={status}
        onChange={(value) => {
          setStatus(value);
          setPage(1);
        }}
      />
      {message ? <Notice tone={message.tone}>{message.text}</Notice> : null}
      {list.loading ? <LoadingBlock label="Đang tải đơn hàng…" /> : null}
      {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
      {!list.loading && !list.error && orders.length === 0 ? <EmptyBlock>Chưa có đơn hàng nào.</EmptyBlock> : null}

      {orders.length > 0 ? (
        <div className="card overflow-x-auto p-0">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Danh sách đơn hàng</caption>
            <thead className="text-muted">
              <tr>
                <th scope="col" className="px-4 py-3">Mã đơn</th>
                <th scope="col" className="px-4 py-3">Ngày đặt</th>
                <th scope="col" className="px-4 py-3">Sản phẩm</th>
                <th scope="col" className="px-4 py-3 text-right">Tổng tiền</th>
                <th scope="col" className="px-4 py-3">Thanh toán</th>
                <th scope="col" className="px-4 py-3">Trạng thái</th>
                {scope === 'seller' ? <th scope="col" className="px-4 py-3">Thao tác</th> : null}
              </tr>
            </thead>
            <tbody>
              {orders.map((order) => (
                <tr key={order.id} className="border-t border-line align-top">
                  <td className="px-4 py-3 font-semibold">
                    <Link href={`${detailBase[scope]}/${order.id}`} className="text-brand hover:underline">
                      #{order.id}
                    </Link>
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-muted">{formatDateTime(order.created_at)}</td>
                  <td className="px-4 py-3 text-text">
                    {order.items?.slice(0, 2).map((item) => `${item.name} × ${item.quantity}`).join(', ')}
                    {order.items && order.items.length > 2 ? ` (+${order.items.length - 2})` : ''}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-right font-semibold text-text">{formatVND(order.total_price)}</td>
                  <td className="px-4 py-3">
                    <span className="grid gap-1">
                      <span className="text-xs text-muted">{paymentMethodLabel(order.payment_method)}</span>
                      <StatusBadge kind="payment" value={order.payment_status} className="w-fit" />
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge kind="order" value={order.status} />
                  </td>
                  {scope === 'seller' ? (
                    <td className="min-w-48 px-4 py-3">
                      {sellerCanShip(order.status) ? (
                        shipping === order.id ? (
                          <ShipDialog busy={busy} onCancel={() => setShipping(null)} onSubmit={(carrier, code) => void ship(order.id, carrier, code)} />
                        ) : (
                          <button type="button" className="btn-primary" onClick={() => setShipping(order.id)}>
                            Giao hàng
                          </button>
                        )
                      ) : (
                        <Link href={`${detailBase[scope]}/${order.id}`} className="text-sm font-semibold text-brand">
                          Chi tiết
                        </Link>
                      )}
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <Pagination page={page} pageSize={PAGE_SIZE} total={list.data?.total ?? 0} onChange={setPage} />
    </div>
  );
}
