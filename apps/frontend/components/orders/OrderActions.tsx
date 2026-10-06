'use client';

import Link from 'next/link';
import { useState } from 'react';
import { orderActionRequest, type OrderAction } from '../../lib/api';
import { useAuthedAction } from '../../lib/hooks';
import { buyerCanCancel, buyerCanConfirmReceived, buyerCanReturn, canDecideReturn, sellerCanCancel, sellerCanDeliver, sellerCanShip } from '../../lib/status';
import type { Order, OrdersScope, Payment } from '../../lib/types';
import { ShipDialog } from '../seller/ShipDialog';
import { Notice } from '../ui/StateBlock';

interface Props {
  scope: OrdersScope;
  order: Order;
  payment?: Payment | null;
  onChanged: (order: Order) => void;
}

type Pending = null | 'cancel' | 'return' | 'ship' | 'reject';

/** Buttons for what the signed-in role may do with the order now (cancel / receive / return-refund / ship / ...). */
export function OrderActions({ scope, order, payment, onChanged }: Props) {
  const run = useAuthedAction();
  const [pending, setPending] = useState<Pending>(null);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const status = order.status;

  const act = async (action: OrderAction, body: { reason?: string; carrier?: string; tracking_code?: string } = {}) => {
    setBusy(true);
    setError(null);
    try {
      const updated = await run((token) => orderActionRequest(scope, order.id, action, body, token));
      setPending(null);
      setReason('');
      onChanged(updated);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const buttons: React.ReactNode[] = [];

  if (scope === 'buyer') {
    if (payment && payment.status === 'REQUIRES_ACTION' && status === 'AWAITING_PAYMENT') {
      buttons.push(
        <Link key="pay" href={payment.pay_url || `/pay/${payment.id}`} className="btn-primary">
          {payment.failure_code ? 'Thanh toán lại' : 'Thanh toán'}
        </Link>
      );
    }
    if (buyerCanConfirmReceived(status)) {
      buttons.push(
        <button key="received" type="button" className="btn-primary" disabled={busy} onClick={() => void act('confirm-received')}>
          Đã nhận hàng
        </button>
      );
    }
    if (buyerCanReturn(status)) {
      buttons.push(
        <button key="return" type="button" className="btn" disabled={busy} onClick={() => setPending('return')}>
          Yêu cầu đổi trả / hoàn tiền
        </button>
      );
    }
    if (buyerCanCancel(status)) {
      buttons.push(
        <button key="cancel" type="button" className="btn-danger" disabled={busy} onClick={() => setPending('cancel')}>
          Huỷ đơn
        </button>
      );
    }
  }

  if (scope === 'seller') {
    if (sellerCanShip(status)) {
      buttons.push(
        <button key="ship" type="button" className="btn-primary" disabled={busy} onClick={() => setPending('ship')}>
          Giao hàng
        </button>
      );
    }
    if (sellerCanDeliver(status)) {
      buttons.push(
        <button key="deliver" type="button" className="btn-primary" disabled={busy} onClick={() => void act('deliver')}>
          Xác nhận đã giao
        </button>
      );
    }
    if (canDecideReturn(status)) {
      buttons.push(
        <button key="approve" type="button" className="btn-primary" disabled={busy} onClick={() => void act('return/approve')}>
          Đồng ý hoàn trả
        </button>,
        <button key="reject" type="button" className="btn" disabled={busy} onClick={() => setPending('reject')}>
          Từ chối hoàn trả
        </button>
      );
    }
    if (sellerCanCancel(status)) {
      buttons.push(
        <button key="cancel" type="button" className="btn-danger" disabled={busy} onClick={() => setPending('cancel')}>
          Huỷ đơn
        </button>
      );
    }
  }

  if (scope === 'admin') {
    if (sellerCanDeliver(status)) {
      buttons.push(
        <button key="deliver" type="button" className="btn" disabled={busy} onClick={() => void act('deliver')}>
          Đánh dấu đã giao
        </button>
      );
    }
    if (canDecideReturn(status)) {
      buttons.push(
        <button key="approve" type="button" className="btn-primary" disabled={busy} onClick={() => void act('return/approve')}>
          Duyệt hoàn trả (hoàn tiền)
        </button>,
        <button key="reject" type="button" className="btn" disabled={busy} onClick={() => setPending('reject')}>
          Từ chối hoàn trả
        </button>
      );
    }
    if (sellerCanCancel(status)) {
      buttons.push(
        <button key="cancel" type="button" className="btn-danger" disabled={busy} onClick={() => setPending('cancel')}>
          Huỷ đơn
        </button>
      );
    }
  }

  if (buttons.length === 0 && !error) return null;

  const needsReason = pending === 'cancel' || pending === 'return' || pending === 'reject';
  const reasonRequired = pending !== 'reject';
  const submitReason = () => {
    const text = reason.trim();
    if (pending === 'cancel') void act('cancel', { reason: text });
    else if (pending === 'return') void act('return', { reason: text });
    else if (pending === 'reject') void act('return/reject', { reason: text });
  };
  const reasonTitle = { cancel: 'Lý do huỷ đơn', return: 'Lý do đổi trả / hoàn tiền', reject: 'Lý do từ chối (tuỳ chọn)', ship: '' };

  return (
    <section className="card grid gap-3" aria-label="Thao tác với đơn hàng">
      <h2 className="text-lg font-semibold text-text">Thao tác</h2>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {buttons.length > 0 ? <div className="flex flex-wrap gap-2">{buttons}</div> : null}
      {pending === 'ship' ? <ShipDialog busy={busy} onCancel={() => setPending(null)} onSubmit={(carrier, code) => void act('ship', { carrier, tracking_code: code })} /> : null}
      {needsReason && pending ? (
        <form
          className="grid gap-2 rounded-xl border border-line bg-surface2 p-4"
          onSubmit={(event) => {
            event.preventDefault();
            submitReason();
          }}
        >
          <label>
            {reasonTitle[pending]}
            <textarea rows={2} maxLength={300} required={reasonRequired} value={reason} onChange={(event) => setReason(event.target.value)} />
          </label>
          <div className="flex gap-2">
            <button type="submit" className={pending === 'cancel' ? 'btn-danger' : 'btn-primary'} disabled={busy}>
              {busy ? 'Đang gửi…' : 'Xác nhận'}
            </button>
            <button type="button" className="btn" onClick={() => setPending(null)}>
              Đóng
            </button>
          </div>
        </form>
      ) : null}
      {scope === 'buyer' && order.payment_status === 'PAID' && buyerCanCancel(status) ? <p className="text-xs text-muted">Đơn đã thanh toán sẽ được hoàn tiền tự động khi huỷ.</p> : null}
    </section>
  );
}
