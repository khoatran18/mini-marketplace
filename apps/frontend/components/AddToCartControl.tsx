'use client';

import Link from 'next/link';
import { useState } from 'react';
import type { Product } from '../lib/types';
import { useAuth } from './auth/AuthProvider';
import { useCart } from './cart/CartProvider';

interface Props {
  product: Product;
  buttonVariant?: 'solid' | 'ghost';
}

export function AddToCartControl({ product, buttonVariant = 'solid' }: Props) {
  const { role } = useAuth();
  const { addItem } = useCart();
  const [isPromptOpen, setPromptOpen] = useState(false);
  const [quantity, setQuantity] = useState(1);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<{ message: string; tone: 'success' | 'error' } | null>(null);

  // sellers and admins do not shop
  if (role && role !== 'buyer') {
    return null;
  }

  const soldOut = product.stock_level === 'none' || product.inventory === 0;

  const handleAddToCart = async () => {
    setBusy(true);
    try {
      await addItem(product, quantity);
      setPromptOpen(false);
      setStatus({ message: 'Đã thêm sản phẩm vào giỏ hàng.', tone: 'success' });
    } catch (err) {
      setStatus({ message: (err as Error).message, tone: 'error' });
    } finally {
      setBusy(false);
    }
  };

  const buttonClass = buttonVariant === 'ghost' ? 'btn w-full text-brand sm:w-auto' : 'btn-primary w-full sm:w-auto';

  return (
    <div className="grid gap-3">
      <button
        type="button"
        disabled={soldOut}
        onClick={() => {
          setStatus(null);
          setQuantity(1);
          setPromptOpen(true);
        }}
        className={buttonClass}
      >
        {soldOut ? 'Hết hàng' : 'Thêm vào giỏ hàng'}
      </button>
      {isPromptOpen ? (
        <div className="grid gap-3 rounded-2xl border border-brand/30 bg-brand-soft p-4">
          <label>
            Số lượng
            <input
              type="number"
              min={1}
              max={99}
              value={quantity}
              onChange={(event) => setQuantity(Math.max(1, Math.min(99, Number(event.target.value) || 1)))}
            />
          </label>
          <div className="flex items-center gap-3">
            <button type="button" className="btn-primary w-full" disabled={busy} onClick={() => void handleAddToCart()}>
              {busy ? 'Đang thêm…' : 'Xác nhận'}
            </button>
            <button type="button" onClick={() => setPromptOpen(false)} className="btn w-full">
              Hủy
            </button>
          </div>
        </div>
      ) : null}
      {status ? (
        <span role="status" aria-live="polite" className={`text-sm ${status.tone === 'success' ? 'text-success' : 'text-danger'}`}>
          {status.message}{' '}
          {status.tone === 'success' ? (
            <Link href="/cart" className="font-semibold underline">
              Xem giỏ hàng
            </Link>
          ) : null}
        </span>
      ) : null}
    </div>
  );
}
