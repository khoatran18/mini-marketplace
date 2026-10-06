'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { getCheckoutRequest, getOrderRequest } from '../../lib/api';
import { formatDateTime, formatVND } from '../../lib/format';
import { useApiData } from '../../lib/hooks';
import { paymentMethodLabel } from '../../lib/status';
import type { Order, OrdersScope } from '../../lib/types';
import { OrderTimeline } from '../checkout/OrderTimeline';
import { CountdownTimer } from '../checkout/CountdownTimer';
import { ErrorBlock, LoadingBlock, SimulatedBadge } from '../ui/StateBlock';
import { StatusBadge } from '../ui/StatusBadge';
import { OrderActions } from './OrderActions';

const backLinks: Record<OrdersScope, { href: string; label: string }> = {
  buyer: { href: '/orders', label: '← Đơn hàng của tôi' },
  seller: { href: '/seller/orders', label: '← Hộp thư đơn hàng' },
  admin: { href: '/admin/orders', label: '← Tất cả đơn hàng' }
};

export function OrderDetail({ scope, orderId }: { scope: OrdersScope; orderId: number }) {
  const loaded = useApiData((token) => getOrderRequest(scope, orderId, token as string), [scope, orderId]);
  const [order, setOrder] = useState<Order | null>(null);

  useEffect(() => {
    if (loaded.data) setOrder(loaded.data);
  }, [loaded.data]);

  // Buyers also need the payment of the checkout (to pay or retry): GET /checkouts/:id returns it next to the orders.
  const checkoutId = order?.checkout_id;
  const checkout = useApiData((token) => getCheckoutRequest(checkoutId as string, token as string), [checkoutId, order?.status], {
    enabled: scope === 'buyer' && Boolean(checkoutId)
  });
  const payment = checkout.data?.payment ?? null;

  if (loaded.loading && !order) return <LoadingBlock label="Đang tải đơn hàng…" />;
  if (loaded.error && !order) return <ErrorBlock message={loaded.status === 404 ? 'Không tìm thấy đơn hàng.' : loaded.error} onRetry={loaded.reload} />;
  if (!order) return null;

  const address = order.shipping_address ?? {};
  const awaiting = order.status === 'AWAITING_PAYMENT' || order.status === 'SUCCESS';

  return (
    <div className="grid gap-6">
      <Link href={backLinks[scope].href} className="w-fit text-sm font-semibold text-brand">
        {backLinks[scope].label}
      </Link>

      <header className="card flex flex-wrap items-start justify-between gap-4">
        <div className="grid gap-1">
          <h1 className="text-2xl font-bold text-text">Đơn hàng #{order.id}</h1>
          <p className="text-sm text-muted">Đặt lúc {formatDateTime(order.created_at)}</p>
          {scope !== 'buyer' ? <p className="text-sm text-muted">Người mua #{order.buyer_id} · Cửa hàng #{order.store_id}</p> : null}
        </div>
        <div className="grid justify-items-end gap-2">
          <StatusBadge kind="order" value={order.status} />
          {awaiting && scope === 'buyer' ? <CountdownTimer target={order.expires_at} label="Hạn thanh toán" onExpire={() => loaded.reload()} /> : null}
        </div>
      </header>

      <section className="card grid gap-3">
        <h2 className="text-lg font-semibold text-text">Tiến trình</h2>
        <OrderTimeline order={order} />
      </section>

      <OrderActions scope={scope} order={order} payment={payment} onChanged={(updated) => { setOrder({ ...order, ...updated, items: updated.items?.length ? updated.items : order.items }); loaded.reload(); checkout.reload(); }} />

      <div className="grid gap-6 md:grid-cols-2">
        <section className="card grid content-start gap-2 text-sm">
          <h2 className="text-lg font-semibold text-text">Giao hàng</h2>
          <p className="font-semibold text-text">
            {address.receiver_name} {address.phone ? `· ${address.phone}` : ''}
          </p>
          <p className="text-muted">{[address.line1, address.ward, address.district, address.city].filter(Boolean).join(', ') || '–'}</p>
          {order.carrier || order.tracking_code ? (
            <p className="flex flex-wrap items-center gap-2 text-text">
              Vận chuyển: {order.carrier || '–'} · Mã vận đơn: <span className="font-mono">{order.tracking_code || '–'}</span> <SimulatedBadge />
            </p>
          ) : null}
          {order.note ? <p className="text-muted">Ghi chú: {order.note}</p> : null}
        </section>

        <section className="card grid content-start gap-2 text-sm">
          <h2 className="text-lg font-semibold text-text">Thanh toán</h2>
          <p className="flex flex-wrap items-center gap-2 text-text">
            {paymentMethodLabel(order.payment_method)} <StatusBadge kind="payment" value={order.payment_status} />
          </p>
          {payment ? (
            <p className="text-muted">
              Mã thanh toán #{payment.id} · <StatusBadge kind="payment" value={payment.status} />
              {payment.card_last4 ? ` · thẻ •••• ${payment.card_last4}` : ''}
            </p>
          ) : null}
          {order.paid_at ? <p className="text-muted">Thanh toán lúc {formatDateTime(order.paid_at)}</p> : null}
        </section>
      </div>

      <section className="card grid gap-3">
        <h2 className="text-lg font-semibold text-text">Sản phẩm</h2>
        <ul className="grid gap-3">
          {order.items.map((item) => (
            <li key={item.id ?? item.product_id} className="flex items-center gap-3">
              <div className="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-surface2">
                {item.image_url ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={item.image_url} alt="" className="h-full w-full object-contain" />
                ) : null}
              </div>
              <div className="min-w-0 flex-1 text-sm">
                <Link href={`/products/${item.product_id}`} className="font-semibold text-text hover:underline">
                  {item.name}
                </Link>
                <p className="text-muted">
                  {formatVND(item.unit_price)} × {item.quantity}
                  {item.sku ? ` · SKU ${item.sku}` : ''}
                </p>
              </div>
              <span className="text-sm font-semibold text-text">{formatVND(item.line_total)}</span>
            </li>
          ))}
        </ul>
        <dl className="grid gap-1 border-t border-line pt-3 text-sm">
          <div className="flex justify-between">
            <dt className="text-muted">Tạm tính</dt>
            <dd>{formatVND(order.subtotal)}</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-muted">Phí giao hàng</dt>
            <dd>{formatVND(order.shipping_fee)}</dd>
          </div>
          <div className="flex justify-between text-base font-bold text-text">
            <dt>Tổng cộng</dt>
            <dd>{formatVND(order.total_price)}</dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
