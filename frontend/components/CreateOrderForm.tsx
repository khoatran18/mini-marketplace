'use client';

import { useEffect, useState } from 'react';
import { createOrderRequest } from '../lib/api';
import type { OrderItem } from '../lib/types';
import { useAuth } from './auth/AuthProvider';

export function CreateOrderForm() {
  const { userId, getValidAccessToken } = useAuth();
  const [buyerId, setBuyerId] = useState(0);
  const [status, setStatus] = useState('pending');
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
        token,
        userId
      );
      setMessage(result.message ?? 'Tạo đơn hàng thành công');
    } catch (error) {
      setMessage((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="card" onSubmit={handleSubmit}>
      <h2 style={{ margin: 0 }}>Tạo đơn hàng</h2>
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
        <select value={status} onChange={(event) => setStatus(event.target.value)}>
          <option value="pending">pending</option>
          <option value="processing">processing</option>
          <option value="completed">completed</option>
          <option value="cancelled">cancelled</option>
        </select>
      </label>
      <div className="grid" style={{ gap: '1rem' }}>
        <h3 style={{ margin: 0 }}>Sản phẩm trong đơn</h3>
        {items.map((item, index) => (
          <div
            key={index}
            className="card"
            style={{
              boxShadow: 'none',
              border: '1px solid #e5e7eb',
              padding: '1rem'
            }}
          >
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
                style={{
                  background: 'transparent',
                  border: '1px solid #ef4444',
                  color: '#ef4444',
                  borderRadius: '0.75rem',
                  padding: '0.5rem 1rem'
                }}
              >
                Xóa
              </button>
            ) : null}
          </div>
        ))}
        <button
          type="button"
          onClick={addItem}
          style={{
            border: '1px dashed #6366f1',
            background: 'transparent',
            color: '#4338ca',
            padding: '0.75rem 1rem',
            borderRadius: '0.75rem'
          }}
        >
          Thêm sản phẩm
        </button>
      </div>
      <button className="primary" type="submit" disabled={loading}>
        {loading ? 'Đang xử lý...' : 'Tạo đơn hàng'}
      </button>
      {message ? <p style={{ margin: 0 }}>{message}</p> : null}
    </form>
  );
}
