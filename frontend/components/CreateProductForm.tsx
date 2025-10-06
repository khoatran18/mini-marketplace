'use client';

import { useEffect, useState } from 'react';
import { createProductRequest } from '../lib/api';
import type { CreateProductInput } from '../lib/types';
import { useAuth } from './auth/AuthProvider';

export function CreateProductForm() {
  const { userId, getValidAccessToken } = useAuth();
  const [formState, setFormState] = useState<Pick<CreateProductInput, 'name' | 'price' | 'inventory' | 'seller_id'>>({
    name: '',
    price: 0,
    inventory: 0,
    seller_id: 0
  });
  const [attributesText, setAttributesText] = useState('');
  const [status, setStatus] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (userId && userId > 0) {
      setFormState((prev) => ({ ...prev, seller_id: userId }));
    }
  }, [userId]);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setStatus(null);

    try {
      const token = await getValidAccessToken();
      if (!token) {
        throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      }
      let attributes: Record<string, unknown> | undefined;
      if (attributesText.trim().length > 0) {
        try {
          attributes = JSON.parse(attributesText) as Record<string, unknown>;
        } catch (parseError) {
          throw new Error('Thuộc tính phải là JSON hợp lệ');
        }
      }

      const payload: CreateProductInput = {
        ...formState,
        attributes
      };

      const result = await createProductRequest(payload, token);
      setStatus(result.message ?? 'Tạo sản phẩm thành công');
    } catch (error) {
      setStatus((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="card grid gap-4" onSubmit={handleSubmit}>
      <h2 className="text-2xl font-bold text-slate-900">Tạo sản phẩm mới</h2>
      <label>
        Tên sản phẩm
        <input
          type="text"
          value={formState.name}
          onChange={(event) => setFormState((prev) => ({ ...prev, name: event.target.value }))}
          required
        />
      </label>
      <label>
        Giá (VND)
        <input
          type="number"
          value={formState.price}
          onChange={(event) =>
            setFormState((prev) => ({ ...prev, price: Number(event.target.value) || 0 }))
          }
          min={0}
          step={1000}
          required
        />
      </label>
      <label>
        Số lượng tồn kho
        <input
          type="number"
          value={formState.inventory}
          onChange={(event) =>
            setFormState((prev) => ({ ...prev, inventory: Number(event.target.value) || 0 }))
          }
          min={0}
          required
        />
      </label>
      <label>
        Mã người bán
        <input
          type="number"
          value={formState.seller_id}
          onChange={(event) =>
            setFormState((prev) => ({ ...prev, seller_id: Number(event.target.value) || 0 }))
          }
          min={1}
          required
        />
      </label>
      <label>
        Thuộc tính (JSON)
        <textarea
          value={attributesText}
          onChange={(event) => setAttributesText(event.target.value)}
          rows={4}
          placeholder='{"color":"red","size":"L"}'
          className="min-h-[120px] rounded-xl border border-slate-300 px-3 py-2 text-base shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
        />
      </label>
      <button
        type="submit"
        disabled={loading}
        className="rounded-xl bg-indigo-600 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-70"
      >
        {loading ? 'Đang xử lý...' : 'Tạo sản phẩm'}
      </button>
      {status ? <p className="text-sm text-slate-700">{status}</p> : null}
    </form>
  );
}
