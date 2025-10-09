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
  const { accessToken, getValidAccessToken } = useAuth();
  const { addItem } = useCart();
  const [isPromptOpen, setPromptOpen] = useState(false);
  const [quantity, setQuantity] = useState(1);
  const [status, setStatus] = useState<{ message: string; tone: 'success' | 'error' } | null>(null);

  const ensureProductId = () => {
    if (typeof product.id !== 'number') {
      setStatus({ message: 'Sản phẩm chưa có mã hợp lệ, không thể thêm vào giỏ.', tone: 'error' });
      return false;
    }
    return true;
  };

  const openPrompt = async () => {
    if (!accessToken) {
      setStatus({ message: 'Vui lòng đăng nhập để thêm sản phẩm vào giỏ hàng.', tone: 'error' });
      router.push('/login');
      return;
    }

    const token = await getValidAccessToken();
    if (!token) {
      setStatus({ message: 'Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.', tone: 'error' });
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

  const buttonClass =
    buttonVariant === 'ghost'
      ? 'w-full rounded-xl border border-slate-300 px-5 py-2.5 font-semibold text-indigo-600 transition hover:bg-indigo-50 sm:w-auto'
      : 'w-full rounded-xl bg-indigo-600 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 sm:w-auto';

  return (
    <div className="grid gap-3">
      <button
        type="button"
        onClick={() => {
          void openPrompt();
        }}
        className={buttonClass}
      >
        Thêm vào giỏ hàng
      </button>
      {isPromptOpen ? (
        <div className="card grid gap-3 border border-indigo-200 bg-indigo-50 shadow-none">
          <label>
            Số lượng
            <input
              type="number"
              min={1}
              value={quantity}
              onChange={(event) => setQuantity(Math.max(1, Number(event.target.value) || 1))}
            />
          </label>
          <div className="flex items-center gap-3">
            <button
              type="button"
              className="w-full rounded-xl bg-indigo-600 px-4 py-2 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600"
              onClick={handleAddToCart}
            >
              Xác nhận
            </button>
            <button
              type="button"
              onClick={() => setPromptOpen(false)}
              className="w-full rounded-xl border border-slate-300 bg-white px-4 py-2 font-semibold text-slate-700 transition hover:bg-slate-100"
            >
              Hủy
            </button>
          </div>
        </div>
      ) : null}
      {status ? (
        <span
          className={`text-sm ${status.tone === 'success' ? 'text-emerald-600' : 'text-rose-600'}`}
        >
          {status.message}
        </span>
      ) : null}
    </div>
  );
}
