'use client';

import { useEffect, useState } from 'react';
import { createOrderRequest } from '../lib/api';
import type { OrderItem } from '../lib/types';
import { useAuth } from './auth/AuthProvider';

export function CreateOrderForm() {
  const { userId, getValidAccessToken } = useAuth();
  const [buyerId, setBuyerId] = useState(0);
  const [status, setStatus] = useState("pending");
  const [items, setItems] = useState<OrderItem[]>([
    { product_id: 0, quantity: 1, price: 0 }
  ]);
  const [message, setMessage] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (userId && userId > 0) {
      setBuyerId(userId);
    }
  }, [userId]);

  const updateItem = (index: number, patch: Partial<OrderItem>) => {
    setItems((prev) => prev.map((item, idx) => (idx === index ? { ...item, ...patch } : item)));
  };

  const addItem = () => {
    setItems((prev) => [...prev, { product_id: 0, quantity: 1, price: 0 }]);
  };

  const removeItem = (index: number) => {
    setItems((prev) => prev.filter((_, idx) => idx !== index));
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setMessage(null);

    try {
      const token = await getValidAccessToken();
      if (!token) {
        throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      }
      const totalPrice = items.reduce((total, item) => total + item.price * item.quantity, 0);
      const result = await createOrderRequest(
        {
          order: {
            buyer_id: buyerId,
            status,
            total_price: totalPrice,
            order_items: items
          }
        },
        token
      );
      setMessage(result.message ?? 'Tạo đơn hàng thành công');
    } catch (error) {
      setMessage((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="card grid gap-4" onSubmit={handleSubmit}>
      <h2 className="text-2xl font-bold text-slate-900">Tạo đơn hàng</h2>
      <label>
        Mã người mua
        <input
          type="number"
          value={buyerId}
          min={1}
          onChange={(event) => setBuyerId(Number(event.target.value) || 0)}
          required
        />
      </label>
      <label>
        Trạng thái
        <select
          value={status}
          onChange={(event) => setStatus(event.target.value)}
          className="w-full rounded-xl border border-slate-300 px-3 py-2 text-base shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
        >
          <option value="pending">pending</option>
          <option value="processing">processing</option>
          <option value="completed">completed</option>
          <option value="cancelled">cancelled</option>
        </select>
      </label>
      <div className="grid gap-4">
        <h3 className="text-lg font-semibold text-slate-900">Sản phẩm trong đơn</h3>
        {items.map((item, index) => (
          <div key={index} className="card grid gap-3 border border-slate-200 bg-slate-50 p-4 shadow-none">
            <label>
              Mã sản phẩm
              <input
                type="number"
                min={1}
                value={item.product_id}
                onChange={(event) => updateItem(index, { product_id: Number(event.target.value) || 0 })}
                required
              />
            </label>
            <label>
              Số lượng
              <input
                type="number"
                min={1}
                value={item.quantity}
                onChange={(event) => updateItem(index, { quantity: Number(event.target.value) || 1 })}
                required
              />
            </label>
            <label>
              Đơn giá
              <input
                type="number"
                min={0}
                step={1000}
                value={item.price}
                onChange={(event) => updateItem(index, { price: Number(event.target.value) || 0 })}
                required
              />
            </label>
            {items.length > 1 ? (
              <button
                type="button"
                onClick={() => removeItem(index)}
                className="w-full rounded-xl border border-rose-200 bg-white px-4 py-2 font-semibold text-rose-600 transition hover:bg-rose-50"
              >
                Xóa
              </button>
            ) : null}
          </div>
        ))}
        <button
          type="button"
          onClick={addItem}
          className="rounded-xl border border-dashed border-indigo-400 px-4 py-2 font-semibold text-indigo-600 transition hover:bg-indigo-50"
        >
          Thêm sản phẩm
        </button>
      </div>
      <button
        type="submit"
        disabled={loading}
        className="rounded-xl bg-indigo-600 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-70"
      >
        {loading ? 'Đang xử lý...' : 'Tạo đơn hàng'}
      </button>
      {message ? <p className="text-sm text-slate-700">{message}</p> : null}
    </form>
  );
}
