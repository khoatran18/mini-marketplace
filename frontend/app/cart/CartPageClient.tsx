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
    <article
      className="card"
      style={{ display: 'grid', gap: '0.75rem', border: '1px solid #e5e7eb', boxShadow: 'none' }}
    >
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h3 style={{ margin: 0 }}>{product.name}</h3>
          <p style={{ margin: 0, color: '#6b7280' }}>Mã sản phẩm: {product.id}</p>
        </div>
        <button
          type="button"
          onClick={onRemove}
          disabled={!canModify}
          style={{
            background: 'transparent',
            border: '1px solid #ef4444',
            color: canModify ? '#ef4444' : '#fca5a5',
            borderRadius: '0.75rem',
            padding: '0.35rem 0.75rem',
            fontWeight: 600,
            cursor: canModify ? 'pointer' : 'not-allowed'
          }}
        >
          Xóa
        </button>
      </header>
      <div style={{ display: 'flex', gap: '1.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
        <span style={{ fontWeight: 700, color: '#4f46e5' }}>{formatCurrency(product.price ?? 0)}</span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      <label style={{ display: 'inline-flex', flexDirection: 'column', gap: '0.35rem' }}>
        Số lượng
        <input
          type="number"
          min={1}
          value={quantity}
          onChange={(event) => onQuantityChange(Math.max(1, Number(event.target.value) || 1))}
          style={{ maxWidth: '120px' }}
          disabled={!canModify}
        />
      </label>
      {attributeEntries.length > 0 ? (
        <dl
          style={{
            display: 'grid',
            gap: '0.35rem',
            margin: 0,
            gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))'
          }}
        >
          {attributeEntries.map(([key, value]) => (
            <div
              key={key}
              style={{ backgroundColor: '#f9fafb', padding: '0.5rem 0.75rem', borderRadius: '0.75rem' }}
            >
              <dt style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#6b7280' }}>{key}</dt>
              <dd style={{ margin: 0, fontWeight: 600 }}>{formatAttributeValue(value)}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {!canModify ? (
        <p style={{ margin: 0, color: '#b91c1c' }}>
          Không thể thao tác vì sản phẩm thiếu mã định danh. Vui lòng làm mới dữ liệu.
        </p>
      ) : null}
    </article>
  );
}

export function CartPageClient() {
  const router = useRouter();
  const { accessToken, role } = useAuth();
  const { items, totalPrice, totalQuantity, updateItemQuantity, removeItem, clearCart } = useCart();
  const [buyerId, setBuyerId] = useState('');
  const [status, setStatus] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const missingProductIds = items.some((entry) => typeof entry.product.id !== 'number');

  const handleCheckout = async () => {
    if (!accessToken) {
      router.push('/login');
      return;
    }

    if (items.length === 0) {
      setStatus('Giỏ hàng đang trống, hãy thêm sản phẩm trước khi thanh toán.');
      return;
    }

    const buyerIdNumber = Number(buyerId);
    if (!buyerIdNumber) {
      setStatus('Vui lòng nhập mã người mua hợp lệ.');
      return;
    }

    if (missingProductIds) {
      setStatus('Một số sản phẩm thiếu mã hợp lệ, vui lòng tải lại danh sách.');
      return;
    }

    setLoading(true);
    setStatus(null);

    try {
      const orderItems = items.map((entry) => ({
        product_id: entry.product.id as number,
        quantity: entry.quantity,
        price: entry.product.price ?? 0,
        name: entry.product.name
      }));

      const response = await createOrderRequest(
        {
          order: {
            buyer_id: buyerIdNumber,
            status: 'pending',
            total_price: orderItems.reduce((total, item) => total + item.price * item.quantity, 0),
            order_items: orderItems
          }
        },
        accessToken
      );

      setStatus(response.message ?? 'Đặt hàng thành công!');
      clearCart();
      setBuyerId('');
    } catch (error) {
      setStatus((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="grid" style={{ gap: '1.5rem' }}>
      <header className="card" style={{ display: 'grid', gap: '0.5rem' }}>
        <h1 style={{ margin: 0 }}>Giỏ hàng của bạn</h1>
        <p style={{ margin: 0, color: '#6b7280' }}>
          Tổng số lượng: <strong>{totalQuantity}</strong> sản phẩm | Tổng giá trị:{' '}
          <strong>{formatCurrency(totalPrice)}</strong>
        </p>
        <p style={{ margin: 0, color: '#6b7280' }}>
          Vai trò hiện tại: <strong>{role}</strong>
        </p>
        <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
          <Link href="/products" style={{ color: '#4338ca', fontWeight: 600 }}>
            ← Tiếp tục mua sắm
          </Link>
        </div>
      </header>

      {missingProductIds ? (
        <div className="card" style={{ backgroundColor: '#fef2f2', color: '#b91c1c' }}>
          Một số sản phẩm trong giỏ không có mã định danh hợp lệ. Vui lòng tải lại danh sách sản phẩm
          hoặc thêm lại sản phẩm để có thể thanh toán.
        </div>
      ) : null}

      {items.length === 0 ? (
        <div className="card">
          <p style={{ margin: 0 }}>Giỏ hàng trống. Hãy duyệt danh sách sản phẩm và thêm món bạn thích.</p>
        </div>
      ) : (
        <div className="grid" style={{ gap: '1rem' }}>
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

      <section className="card" style={{ display: 'grid', gap: '1rem' }}>
        <h2 style={{ margin: 0 }}>Thanh toán</h2>
        <label style={{ display: 'inline-flex', flexDirection: 'column', gap: '0.35rem', maxWidth: '240px' }}>
          Mã người mua
          <input
            type="number"
            min={1}
            value={buyerId}
            onChange={(event) => setBuyerId(event.target.value)}
            placeholder="Nhập ID người mua"
          />
        </label>
        <button
          className="primary"
          type="button"
          onClick={handleCheckout}
          disabled={loading || items.length === 0}
        >
          {loading ? 'Đang xử lý...' : 'Checkout (tạo đơn hàng)'}
        </button>
        {status ? <p style={{ margin: 0 }}>{status}</p> : null}
      </section>
    </div>
  );
}
