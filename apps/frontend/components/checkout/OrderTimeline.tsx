import type { Order } from '../../lib/types';
import { formatDateTime } from '../../lib/format';
import { orderStatusMeta, orderStepsCod, orderStepsOnline, toneClass } from '../../lib/status';

const actors: Record<string, string> = { system: 'Hệ thống', buyer: 'Người mua', seller: 'Người bán', admin: 'Quản trị', payment: 'Thanh toán' };

function stepTime(order: Order, step: string): string | undefined {
  switch (step) {
    case 'PAID':
      return order.paid_at;
    case 'SHIPPED':
      return order.shipped_at;
    case 'DELIVERED':
      return order.delivered_at;
    case 'AWAITING_PAYMENT':
    case 'CONFIRMED':
      return order.created_at;
    default:
      return undefined;
  }
}

/** Progress of the happy path plus the full status history (who changed what, and why). */
export function OrderTimeline({ order }: { order: Order }) {
  const cod = order.payment_method === 'COD';
  const steps: readonly string[] = cod ? orderStepsCod : orderStepsOnline;
  const status = order.status === 'SUCCESS' ? 'AWAITING_PAYMENT' : order.status;
  const exited = ['CANCELED', 'EXPIRED', 'FAILED', 'REFUND_REQUESTED', 'REFUNDED'].includes(status);
  const refunded = status === 'REFUND_REQUESTED' || status === 'REFUNDED';
  const currentIndex = exited ? -1 : steps.indexOf(status);

  return (
    <div className="grid gap-4">
      <ol className="grid gap-3 sm:grid-flow-col sm:auto-cols-fr" aria-label="Tiến trình đơn hàng">
        <li className="flex items-center gap-2 text-sm text-text sm:flex-col sm:items-start">
          <span className="grid gap-0.5">
            <span className="flex items-center gap-2 font-semibold">
              <span aria-hidden="true" className="text-success">●</span> Đã đặt
            </span>
            <span className="text-xs text-muted">{formatDateTime(order.created_at)}</span>
          </span>
        </li>
        {steps.map((step, index) => {
          const done = exited ? refunded || Boolean(stepTime(order, step)) : index <= currentIndex;
          const active = !exited && index === currentIndex;
          const time = stepTime(order, step);
          return (
            <li key={step} className="flex items-center gap-2 text-sm sm:flex-col sm:items-start" aria-current={active ? 'step' : undefined}>
              <span className="grid gap-0.5">
                <span className={`flex items-center gap-2 font-semibold ${done ? 'text-text' : 'text-muted'}`}>
                  <span aria-hidden="true" className={done ? 'text-success' : 'text-muted'}>
                    {done ? '●' : '○'}
                  </span>
                  {orderStatusMeta(step).label}
                  {active ? <span className="sr-only"> (hiện tại)</span> : null}
                </span>
                <span className="text-xs text-muted">{done && time ? formatDateTime(time) : ''}</span>
              </span>
            </li>
          );
        })}
      </ol>

      {exited ? (
        <p className={`w-fit rounded-xl px-3 py-2 text-sm font-semibold ${toneClass[orderStatusMeta(status).tone]}`}>
          {orderStatusMeta(status).label}
          {order.cancel_reason ? ` – ${order.cancel_reason}` : ''}
          {status === 'REFUND_REQUESTED' && order.return_reason ? ` – ${order.return_reason}` : ''}
        </p>
      ) : null}

      {order.history && order.history.length > 0 ? (
        <details className="rounded-xl border border-line p-3 text-sm" open={order.history.length <= 6}>
          <summary className="cursor-pointer font-semibold text-text">Lịch sử trạng thái ({order.history.length})</summary>
          <ul className="mt-3 grid gap-2">
            {order.history.map((entry, index) => (
              <li key={`${entry.at}-${index}`} className="grid gap-0.5 border-l-2 border-line pl-3">
                <span className="font-medium text-text">
                  {entry.from_status ? `${orderStatusMeta(entry.from_status).label} → ` : ''}
                  {orderStatusMeta(entry.to_status).label}
                </span>
                <span className="text-xs text-muted">
                  {formatDateTime(entry.at)} · {actors[entry.actor_type] ?? entry.actor_type}
                  {entry.reason ? ` · ${entry.reason}` : ''}
                </span>
              </li>
            ))}
          </ul>
        </details>
      ) : null}
    </div>
  );
}
