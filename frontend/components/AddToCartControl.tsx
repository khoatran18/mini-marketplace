'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import type { Product } from '../lib/types';
import { useAuth } from './auth/AuthProvider';
import { useCart } from './cart/CartProvider';

interface Props {
  product: Product;
  buttonVariant?: 'solid' | 'ghost';
}

export function AddToCartControl({ product, buttonVariant = 'solid' }: Props) {
  const router = useRouter();
  const { accessToken } = useAuth();
  const { addItem } = useCart();
  const [isPromptOpen, setPromptOpen] = useState(false);
  const [quantity, setQuantity] = useState(1);
  const [status, setStatus] = useState<{ message: string; tone: 'success' | 'error' } | null>(
    null
  );

  const ensureProductId = () => {
    if (typeof product.id !== 'number') {
      setStatus({ message: 'Sản phẩm chưa có mã hợp lệ, không thể thêm vào giỏ.', tone: 'error' });
      return false;
    }
    return true;
  };

  const openPrompt = () => {
    if (!accessToken) {
      setStatus({ message: 'Vui lòng đăng nhập để thêm sản phẩm vào giỏ hàng.', tone: 'error' });
      router.push('/login');
      return;
    }
    if (!ensureProductId()) {
      return;
    }
    setStatus(null);
    setQuantity(1);
    setPromptOpen(true);
  };

  const handleAddToCart = () => {
    if (!ensureProductId()) {
      return;
    }

    addItem(product, quantity);
    setPromptOpen(false);
    setStatus({ message: 'Đã thêm sản phẩm vào giỏ hàng.', tone: 'success' });
  };

  const buttonStyles =
    buttonVariant === 'ghost'
      ? {
          border: '1px solid #d1d5db',
          background: 'transparent',
          color: '#4338ca'
        }
      : {
          background: '#4338ca',
          color: 'white',
          border: 'none'
        };

  return (
    <div style={{ display: 'grid', gap: '0.75rem' }}>
      <button
        type="button"
        onClick={openPrompt}
        style={{
          padding: '0.5rem 1.25rem',
          borderRadius: '0.75rem',
          fontWeight: 600,
          cursor: 'pointer',
          ...buttonStyles
        }}
      >
        Thêm vào giỏ hàng
      </button>
      {isPromptOpen ? (
        <div
          className="card"
          style={{
            border: '1px solid #c7d2fe',
            background: '#eef2ff',
            display: 'grid',
            gap: '0.5rem',
            boxShadow: 'none'
          }}
        >
          <label style={{ fontWeight: 600 }}>
            Số lượng
            <input
              type="number"
              min={1}
              value={quantity}
              onChange={(event) => setQuantity(Math.max(1, Number(event.target.value) || 1))}
              style={{ marginTop: '0.35rem' }}
            />
          </label>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <button
              type="button"
              className="primary"
              onClick={handleAddToCart}
              style={{ flex: 1 }}
            >
              Xác nhận
            </button>
            <button
              type="button"
              onClick={() => setPromptOpen(false)}
              style={{
                flex: 1,
                border: '1px solid #d1d5db',
                borderRadius: '0.75rem',
                background: 'white',
                fontWeight: 600
              }}
            >
              Hủy
            </button>
          </div>
        </div>
      ) : null}
      {status ? (
        <span
          style={{
            color: status.tone === 'success' ? '#047857' : '#b91c1c',
            fontSize: '0.9rem'
          }}
        >
          {status.message}
        </span>
      ) : null}
    </div>
  );
}
