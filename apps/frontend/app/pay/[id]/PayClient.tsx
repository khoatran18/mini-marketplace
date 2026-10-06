'use client';

import Link from 'next/link';
import { useCallback, useEffect, useRef, useState } from 'react';
import { CountdownTimer } from '../../../components/checkout/CountdownTimer';
import { MockPayForm } from '../../../components/checkout/MockPayForm';
import { ErrorBlock, LoadingBlock, Notice, SimulatedBadge } from '../../../components/ui/StateBlock';
import { StatusBadge } from '../../../components/ui/StatusBadge';
import { cancelPaymentRequest, confirmPaymentRequest, getPaymentMethodsRequest, getPaymentRequest, type ConfirmPaymentInput } from '../../../lib/api';
import { formatVND } from '../../../lib/format';
import { useApiData, useAuthedAction } from '../../../lib/hooks';
import { failureCodeLabel, paymentMethodLabel } from '../../../lib/status';
import { track } from '../../../lib/tracking';
import type { Payment } from '../../../lib/types';

const POLL_MS = 2000;

export function PayClient({ paymentId }: { paymentId: number }) {
  const run = useAuthedAction();
  const loaded = useApiData((token) => getPaymentRequest(paymentId, token as string), [paymentId]);
  const methods = useApiData(() => getPaymentMethodsRequest(), [], { auth: 'none' });
  const [payment, setPayment] = useState<Payment | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const tracked = useRef(false);

  useEffect(() => {
    if (loaded.data) setPayment(loaded.data);
  }, [loaded.data]);

  useEffect(() => {
    if (payment && !tracked.current) {
      tracked.current = true;
      track('payment_page_view', { method: payment.method }, { surface: 'checkout' });
    }
  }, [payment]);

  // After submitting, the (simulated) provider answers through a webhook: poll until the payment settles.
  const status = payment?.status;
  useEffect(() => {
    if (status !== 'PROCESSING') return;
    const id = setInterval(() => {
      void run((token) => getPaymentRequest(paymentId, token))
        .then(setPayment)
        .catch(() => undefined);
    }, POLL_MS);
    return () => clearInterval(id);
  }, [status, paymentId, run]);

  const submit = useCallback(
    async (input: ConfirmPaymentInput) => {
      setBusy(true);
      setError(null);
      try {
        setPayment(await run((token) => confirmPaymentRequest(paymentId, input, token)));
      } catch (err) {
        setError((err as Error).message);
      } finally {
        setBusy(false);
      }
    },
    [paymentId, run]
  );

  const cancel = async () => {
    setBusy(true);
    setError(null);
    try {
      setPayment(await run((token) => cancelPaymentRequest(paymentId, token)));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (loaded.loading && !payment) return <LoadingBlock label="Đang tải thông tin thanh toán…" />;
  if (loaded.error && !payment) return <ErrorBlock message={loaded.status === 404 ? 'Không tìm thấy khoản thanh toán.' : loaded.error} onRetry={loaded.reload} />;
  if (!payment) return null;

  const orderLink = payment.order_ids && payment.order_ids.length === 1 ? `/orders/${payment.order_ids[0]}` : '/orders';
  const open = payment.status === 'REQUIRES_ACTION';
  const failureText = failureCodeLabel(payment.failure_code);

  return (
    <div className="mx-auto grid max-w-xl gap-4">
      <div role="note" className="rounded-xl border-2 border-dashed border-warning bg-warning-soft px-4 py-3 text-center text-sm font-bold uppercase tracking-wide text-warning">
        Trang thanh toán giả lập – không trừ tiền thật
      </div>

      <section className="card grid gap-4">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div className="grid gap-1">
            <h1 className="text-2xl font-bold text-text">Thanh toán #{payment.id}</h1>
            <p className="flex flex-wrap items-center gap-2 text-sm text-muted">
              {paymentMethodLabel(payment.method).replace(/\s*\(MÔ PHỎNG\)/, '')} <SimulatedBadge />
            </p>
          </div>
          <div className="grid justify-items-end gap-2">
            <StatusBadge kind="payment" value={payment.status} />
            {open ? <CountdownTimer target={payment.expires_at} label="Hạn thanh toán" onExpire={() => loaded.reload()} /> : null}
          </div>
        </header>

        <p className="text-3xl font-bold text-text">{formatVND(payment.amount)}</p>

        {error ? <Notice tone="error">{error}</Notice> : null}

        {open ? (
          <>
            <MockPayForm payment={payment} methods={methods.data?.methods ?? []} busy={busy} onSubmit={(input) => void submit(input)} />
            <button type="button" className="btn-danger w-fit" disabled={busy} onClick={() => void cancel()}>
              Huỷ thanh toán
            </button>
          </>
        ) : null}

        {payment.status === 'PROCESSING' ? (
          <div role="status" aria-live="polite" className="grid gap-2">
            <Notice tone="info">Đang xử lý… kết quả sẽ tự cập nhật (có thể mất vài giây, kịch bản “timeout” mất khoảng 30 giây).</Notice>
          </div>
        ) : null}

        {['SUCCEEDED', 'PARTIALLY_REFUNDED', 'REFUNDED'].includes(payment.status) ? <Notice tone="success">Thanh toán thành công. Cảm ơn bạn!</Notice> : null}
        {payment.status === 'FAILED' ? <Notice tone="error">Thanh toán thất bại{failureText ? `: ${failureText}` : ''}. Đơn hàng sẽ hết hạn nếu không được thanh toán.</Notice> : null}
        {payment.status === 'EXPIRED' ? <Notice tone="error">Khoản thanh toán đã hết hạn, hàng giữ chỗ đã được trả lại.</Notice> : null}
        {payment.status === 'CANCELED' ? <Notice tone="warning">Bạn đã huỷ thanh toán này.</Notice> : null}

        <footer className="flex flex-wrap gap-3 text-sm font-semibold text-brand">
          <Link href={orderLink}>Xem đơn hàng →</Link>
          <Link href="/products">Tiếp tục mua sắm</Link>
        </footer>
      </section>
    </div>
  );
}
