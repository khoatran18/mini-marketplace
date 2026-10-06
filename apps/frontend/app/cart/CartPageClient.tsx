'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useAuth } from '../../components/auth/AuthProvider';
import { useCart } from '../../components/cart/CartProvider';
import { StockBadge } from '../../components/shop/StockBadge';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../../components/ui/StateBlock';
import { formatVND } from '../../lib/format';
import type { CartLine } from '../../lib/types';

function CartLineCard({ line, busy, onQuantity, onRemove }: { line: CartLine; busy: boolean; onQuantity: (quantity: number) => void; onRemove: () => void }) {
  return (
    <article className="card grid gap-3 shadow-none sm:grid-cols-[96px_1fr_auto]">
      <div className="flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl bg-surface2">
        {line.image_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={line.image_url} alt="" className="h-full w-full object-contain" />
        ) : (
          <span className="text-xs text-muted">Chưa có ảnh</span>
        )}
      </div>
      <div className="grid content-start gap-1">
        <Link href={`/products/${line.product_id}`} className="text-lg font-semibold text-text hover:underline">
          {line.name}
        </Link>
        <p className="text-sm text-muted">
          {formatVND(line.unit_price)} × {line.quantity} = <strong className="text-text">{formatVND(line.line_total)}</strong>
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <StockBadge level={line.stock_level} />
          {line.issue ? <span className="text-sm font-medium text-danger">{line.issue}</span> : null}
        </div>
      </div>
      <div className="flex items-start gap-2 sm:flex-col sm:items-end">
        <div className="inline-flex items-center gap-1">
          <button type="button" className="btn px-3" aria-label="Giảm số lượng" disabled={busy || line.quantity <= 1} onClick={() => onQuantity(line.quantity - 1)}>
            −
          </button>
          <span className="min-w-8 text-center font-semibold" aria-live="polite">
            {line.quantity}
          </span>
          <button type="button" className="btn px-3" aria-label="Tăng số lượng" disabled={busy || line.quantity >= 99} onClick={() => onQuantity(line.quantity + 1)}>
            +
          </button>
        </div>
        <button type="button" className="btn-danger" disabled={busy} onClick={onRemove}>
          Xoá
        </button>
      </div>
    </article>
  );
}

export function CartPageClient() {
  const { role, ready } = useAuth();
  const { lines, subtotal, totalQuantity, loading, error, isGuest, setQuantity, removeItem, emptyCart, refresh } = useCart();
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setActionError(null);
    try {
      await fn();
    } catch (err) {
      setActionError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (!ready) return <LoadingBlock />;
  if (role && role !== 'buyer') {
    return <EmptyBlock>Giỏ hàng chỉ dành cho tài khoản người mua.</EmptyBlock>;
  }

  const hasIssue = lines.some((line) => line.available === false || Boolean(line.issue));

  return (
    <div className="grid gap-6">
      <header className="card grid gap-1">
        <h1 className="text-2xl font-bold text-text">Giỏ hàng</h1>
        <p className="text-sm text-muted">
          {isGuest ? 'Bạn chưa đăng nhập: giỏ hàng được lưu trên trình duyệt và sẽ được gộp vào tài khoản khi đăng nhập.' : 'Giỏ hàng được lưu trên máy chủ, giá và tồn kho luôn được kiểm tra lại khi thanh toán.'}
        </p>
      </header>

      {loading && lines.length === 0 ? <LoadingBlock label="Đang tải giỏ hàng…" /> : null}
      {error ? <ErrorBlock message={error} onRetry={() => void refresh()} /> : null}
      {actionError ? <Notice tone="error">{actionError}</Notice> : null}

      {!loading && !error && lines.length === 0 ? (
        <EmptyBlock>
          Giỏ hàng của bạn đang trống.{' '}
          <Link href="/products" className="font-semibold text-brand">
            Tiếp tục mua sắm
          </Link>
        </EmptyBlock>
      ) : null}

      {lines.length > 0 ? (
        <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
          <div className="grid content-start gap-4">
            {lines.map((line) => (
              <CartLineCard
                key={line.product_id}
                line={line}
                busy={busy}
                onQuantity={(quantity) => void run(() => setQuantity(line.product_id, quantity))}
                onRemove={() => void run(() => removeItem(line.product_id))}
              />
            ))}
          </div>
          <aside className="card grid content-start gap-3">
            <h2 className="text-lg font-semibold text-text">Tạm tính</h2>
            <dl className="grid gap-1 text-sm">
              <div className="flex justify-between">
                <dt className="text-muted">Số lượng</dt>
                <dd>{totalQuantity}</dd>
              </div>
              <div className="flex justify-between text-base font-semibold">
                <dt>Tạm tính</dt>
                <dd>{formatVND(subtotal)}</dd>
              </div>
            </dl>
            <p className="text-xs text-muted">Phí giao hàng được tính ở bước thanh toán.</p>
            {hasIssue ? <Notice tone="warning">Một số sản phẩm không còn đủ hàng. Hãy điều chỉnh trước khi thanh toán.</Notice> : null}
            <Link href={isGuest ? '/login' : '/checkout'} className="btn-primary">
              {isGuest ? 'Đăng nhập để thanh toán' : 'Tiến hành thanh toán'}
            </Link>
            <button type="button" className="btn-danger" disabled={busy} onClick={() => void run(emptyCart)}>
              Xoá toàn bộ giỏ hàng
            </button>
          </aside>
        </div>
      ) : null}
    </div>
  );
}
