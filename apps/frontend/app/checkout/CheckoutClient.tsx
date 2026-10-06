'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import { AddressPicker } from '../../components/checkout/AddressPicker';
import { OrderSummary } from '../../components/checkout/OrderSummary';
import { PaymentMethodPicker } from '../../components/checkout/PaymentMethodPicker';
import { useCart } from '../../components/cart/CartProvider';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../../components/ui/StateBlock';
import { checkoutRequest, getPaymentMethodsRequest, previewCheckoutRequest } from '../../lib/api';
import { useApiData, useAuthedAction } from '../../lib/hooks';
import { track } from '../../lib/tracking';
import type { PaymentMethodInfo } from '../../lib/types';

// Fallback when GET /payments/methods is unavailable.
const fallbackMethods: PaymentMethodInfo[] = [
  { code: 'MOCK_CARD', label: 'Thẻ (MÔ PHỎNG)', online: true, simulated: true },
  { code: 'MOCK_WALLET', label: 'Ví điện tử (MÔ PHỎNG)', online: true, simulated: true },
  { code: 'MOCK_BANK_TRANSFER', label: 'Chuyển khoản (MÔ PHỎNG)', online: true, simulated: true },
  { code: 'COD', label: 'Thanh toán khi nhận hàng (COD)', online: false, simulated: false }
];

function newKey(): string {
  return typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function CheckoutClient() {
  const router = useRouter();
  const run = useAuthedAction();
  const { refresh } = useCart();
  const preview = useApiData((token) => previewCheckoutRequest(token as string), []);
  const methodsState = useApiData(() => getPaymentMethodsRequest(), [], { auth: 'none' });
  const [addressId, setAddressId] = useState<number | null>(null);
  const [method, setMethod] = useState('MOCK_CARD');
  const [note, setNote] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // One key per checkout attempt: a retry after a network error replays the same order instead of creating another.
  const keyRef = useRef<string>(newKey());

  const methods = methodsState.data?.methods?.length ? methodsState.data.methods : fallbackMethods;
  const data = preview.data;

  useEffect(() => {
    track('checkout_start', { cart_value: data?.grand_total ?? 0 }, { surface: 'checkout' });
    // once per visit, after the preview is known
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [Boolean(data)]);

  const placeOrder = async () => {
    if (!addressId) {
      setError('Vui lòng chọn địa chỉ giao hàng.');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      track('checkout_submit', { cart_value: data?.grand_total ?? 0, method }, { surface: 'checkout' });
      const result = await run((token) =>
        checkoutRequest({ from_cart: true, address_id: addressId, payment_method: method, note: note.trim() || undefined }, keyRef.current, token)
      );
      keyRef.current = newKey();
      await refresh();
      if (result.payment && result.payment.next_action !== 'none') {
        router.push(result.payment.pay_url || `/pay/${result.payment.id}`);
      } else if (result.orders?.length === 1) {
        router.push(`/orders/${result.orders[0].id}`);
      } else {
        router.push('/orders');
      }
    } catch (err) {
      setError((err as Error).message);
      setSubmitting(false);
    }
  };

  if (preview.loading) return <LoadingBlock label="Đang tính tổng đơn hàng…" />;
  if (preview.error) return <ErrorBlock message={preview.error} onRetry={preview.reload} />;
  if (!data || data.groups.length === 0) {
    return (
      <EmptyBlock>
        Giỏ hàng trống.{' '}
        <Link href="/products" className="font-semibold text-brand">
          Tiếp tục mua sắm
        </Link>
      </EmptyBlock>
    );
  }

  const selected = methods.find((item) => item.code === method);

  return (
    <div className="grid gap-6">
      <header className="card grid gap-1">
        <h1 className="text-2xl font-bold text-text">Thanh toán</h1>
        <p className="text-sm text-muted">Giá, phí giao hàng và tồn kho do máy chủ tính và kiểm tra lại khi đặt hàng.</p>
      </header>

      <div className="grid gap-6 lg:grid-cols-[1fr_340px]">
        <div className="grid content-start gap-6">
          <section className="card grid gap-3">
            <h2 className="text-lg font-semibold text-text">1. Địa chỉ giao hàng</h2>
            <AddressPicker selectedId={addressId} onSelect={setAddressId} />
          </section>
          <section className="card grid gap-3">
            <h2 className="text-lg font-semibold text-text">2. Phương thức thanh toán</h2>
            <PaymentMethodPicker methods={methods} value={method} onChange={setMethod} />
            {selected?.simulated ? <Notice tone="warning">Chế độ mô phỏng: không trừ tiền thật. Bạn sẽ được chuyển tới trang thanh toán giả lập.</Notice> : null}
          </section>
          <section className="card grid gap-3">
            <h2 className="text-lg font-semibold text-text">3. Ghi chú</h2>
            <label>
              Ghi chú cho người bán (tuỳ chọn)
              <textarea rows={2} maxLength={500} value={note} onChange={(event) => setNote(event.target.value)} />
            </label>
          </section>
        </div>

        <aside className="card grid h-fit content-start gap-4 lg:sticky lg:top-4">
          <h2 className="text-lg font-semibold text-text">Tóm tắt đơn hàng</h2>
          <OrderSummary preview={data} />
          {!data.can_order ? <Notice tone="error">Có sản phẩm không thể đặt. Hãy quay lại giỏ hàng để điều chỉnh.</Notice> : null}
          {error ? <Notice tone="error">{error}</Notice> : null}
          <button type="button" className="btn-primary" disabled={submitting || !data.can_order || !addressId} onClick={() => void placeOrder()}>
            {submitting ? 'Đang đặt hàng…' : 'Đặt hàng'}
          </button>
          <Link href="/cart" className="text-center text-sm font-semibold text-brand">
            ← Quay lại giỏ hàng
          </Link>
        </aside>
      </div>
    </div>
  );
}
