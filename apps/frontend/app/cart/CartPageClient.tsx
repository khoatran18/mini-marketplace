'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { createOrderRequest } from '../../lib/api';
import type { CartEntry } from '../../lib/types';
import { useAuth } from '../../components/auth/AuthProvider';
import { useCart } from '../../components/cart/CartProvider';

function formatCurrency(value: number) {
  return new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(value);
}

function formatAttributeValue(value: unknown): string {
  if (Array.isArray(value)) {
    return value.map((item) => formatAttributeValue(item)).join(', ');
  }
  if (value && typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>)
      .map(([key, val]) => `${key}: ${formatAttributeValue(val)}`)
      .join('; ');
  }
  if (typeof value === 'boolean') {
    return value ? 'Có' : 'Không';
  }
  if (value === null || value === undefined) {
    return '—';
  }
  return String(value);
}

function CartItemCard({ entry, onQuantityChange, onRemove, canModify }: {
  entry: CartEntry;
  onQuantityChange: (quantity: number) => void;
  onRemove: () => void;
  canModify: boolean;
}) {
  const { product, quantity } = entry;
  const attributes = product.attributes ?? {};
  const attributeEntries = Object.entries(attributes);

  return (
    <article className="card grid gap-3 border border-slate-200 bg-white shadow-none">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-lg font-semibold text-slate-900">{product.name}</h3>
          <p className="text-sm text-slate-500">Mã sản phẩm: {product.id}</p>
        </div>
        <button
          type="button"
          onClick={onRemove}
          disabled={!canModify}
          className={`rounded-xl border px-3 py-1.5 text-sm font-semibold transition ${
            canModify
              ? 'border-rose-300 text-rose-600 hover:bg-rose-50'
              : 'border-rose-100 text-rose-300 cursor-not-allowed'
          }`}
        >
          Xóa
        </button>
      </header>
      <div className="flex flex-wrap items-center gap-4 text-sm text-slate-700">
        <span className="text-base font-semibold text-indigo-600">{formatCurrency(product.price ?? 0)}</span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      <label className="inline-flex max-w-xs flex-col gap-1 text-sm font-semibold text-slate-700">
        Số lượng
        <input
          type="number"
          min={1}
          value={quantity}
          onChange={(event) => onQuantityChange(Math.max(1, Number(event.target.value) || 1))}
          className="max-w-[120px] rounded-xl border border-slate-300 px-3 py-2 text-base shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={!canModify}
        />
      </label>
      {attributeEntries.length > 0 ? (
        <dl className="grid gap-2 text-sm text-slate-700 sm:grid-cols-2 lg:grid-cols-3">
          {attributeEntries.map(([key, value]) => (
            <div key={key} className="rounded-xl bg-slate-50 px-3 py-2">
              <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{key}</dt>
              <dd className="font-medium text-slate-800">{formatAttributeValue(value)}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {!canModify ? (
        <p className="text-sm font-medium text-rose-600">
          Không thể thao tác vì sản phẩm thiếu mã định danh. Vui lòng làm mới dữ liệu.
        </p>
      ) : null}
    </article>
  );
}

export function CartPageClient() {
  const router = useRouter();
  const { accessToken, role, userId, getValidAccessToken } = useAuth();
  const { items, totalPrice, totalQuantity, updateItemQuantity, removeItem, clearCart } = useCart();
  const [status, setStatus] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const missingProductIds = items.some((entry) => typeof entry.product.id !== 'number');

  const handleCheckout = async () => {
    if (items.length === 0) {
      setStatus('Giỏ hàng đang trống, hãy thêm sản phẩm trước khi thanh toán.');
      return;
    }

    if (!accessToken) {
      router.push('/login');
      return;
    }

    if (!userId || userId <= 0) {
      setStatus('Không thể xác định tài khoản hiện tại. Vui lòng đăng nhập lại.');
      router.push('/login');
      return;
    }

    if (missingProductIds) {
      setStatus('Một số sản phẩm thiếu mã hợp lệ, vui lòng tải lại danh sách.');
      return;
    }

    setLoading(true);
    setStatus(null);

    try {
      const token = await getValidAccessToken();
      if (!token) {
        setStatus('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
        router.push('/login');
        return;
      }
      const orderItems = items.map((entry) => ({
        product_id: entry.product.id as number,
        quantity: entry.quantity,
        price: entry.product.price ?? 0,
        name: entry.product.name
      }));

      const response = await createOrderRequest(
        {
          order: {
            buyer_id: userId,
            status: 'pending',
            total_price: orderItems.reduce((total, item) => total + item.price * item.quantity, 0),
            order_items: orderItems
          }
        },
        token
      );

      setStatus(response.message ?? 'Đặt hàng thành công!');
      clearCart();
    } catch (error) {
      setStatus((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="grid gap-6">
      <header className="card grid gap-2">
        <h1 className="text-2xl font-bold text-slate-900">Giỏ hàng của bạn</h1>
        <p className="text-sm text-slate-600">
          Tổng số lượng: <strong>{totalQuantity}</strong> sản phẩm | Tổng giá trị: <strong>{formatCurrency(totalPrice)}</strong>
        </p>
        <p className="text-sm text-slate-600">
          Vai trò hiện tại: <strong>{role}</strong>
        </p>
        <p className="text-sm text-slate-600">
          ID người dùng: <strong>{userId ?? 'Không xác định'}</strong>
        </p>
        <div className="flex flex-wrap gap-3">
          <Link href="/products" className="font-semibold text-indigo-600 hover:text-indigo-500">
            ← Tiếp tục mua sắm
          </Link>
        </div>
      </header>

      {missingProductIds ? (
        <div className="card border border-rose-200 bg-rose-50 text-sm font-medium text-rose-700">
          Một số sản phẩm trong giỏ không có mã định danh hợp lệ. Vui lòng tải lại danh sách sản phẩm hoặc thêm lại sản
          phẩm để có thể thanh toán.
        </div>
      ) : null}

      {items.length === 0 ? (
        <div className="card text-sm text-slate-600">
          Giỏ hàng trống. Hãy duyệt danh sách sản phẩm và thêm món bạn thích.
        </div>
      ) : (
        <div className="grid gap-4">
          {items.map((entry) => {
            const hasValidId = typeof entry.product.id === 'number';
            return (
              <CartItemCard
                key={`${entry.product.id}-${entry.product.name}`}
                entry={entry}
                canModify={hasValidId}
                onQuantityChange={(quantity) => {
                  if (hasValidId) {
                    updateItemQuantity(entry.product.id as number, Math.max(1, quantity));
                  }
                }}
                onRemove={() => {
                  if (hasValidId) {
                    removeItem(entry.product.id as number);
                  }
                }}
              />
            );
          })}
        </div>
      )}

      <section className="card grid gap-3">
        <h2 className="text-xl font-semibold text-slate-900">Thanh toán</h2>
        <p className="text-sm text-slate-600">
          Đơn hàng sẽ được tạo cho tài khoản có ID <strong>{userId && userId > 0 ? userId : 'Không xác định'}</strong>.
        </p>
        <button
          type="button"
          onClick={handleCheckout}
          disabled={loading || items.length === 0 || !userId}
          className="rounded-xl bg-indigo-600 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-70"
        >
          {loading ? 'Đang xử lý...' : 'Checkout (tạo đơn hàng)'}
        </button>
        {status ? <p className="text-sm text-slate-700">{status}</p> : null}
      </section>
    </div>
  );
}
