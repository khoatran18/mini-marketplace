'use client';

import { useEffect, useMemo, useState } from 'react';
import { createProductRequest } from '../lib/api';
import type { CreateProductInput } from '../lib/types';
import { useAuth } from './auth/AuthProvider';

type Step = 'form' | 'confirm';

interface AttributeRow {
  key: string;
  value: string;
}

interface StatusState {
  type: 'success' | 'error';
  message: string;
}

function createEmptyAttribute(): AttributeRow {
  return { key: '', value: '' };
}

export function CreateProductForm() {
  const { userId, getValidAccessToken } = useAuth();
  const [formState, setFormState] = useState<Pick<CreateProductInput, 'name' | 'price' | 'inventory' | 'seller_id'>>({
    name: '',
    price: 0,
    inventory: 0,
    seller_id: 0
  });
  const [attributes, setAttributes] = useState<AttributeRow[]>([createEmptyAttribute()]);
  const [step, setStep] = useState<Step>('form');
  const [status, setStatus] = useState<StatusState | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (userId && userId > 0) {
      setFormState((prev) => ({ ...prev, seller_id: userId }));
    }
  }, [userId]);

  const sanitizedAttributes = useMemo(() => {
    const entries = attributes
      .map((attribute) => ({
        key: attribute.key.trim(),
        value: attribute.value
      }))
      .filter((attribute) => attribute.key.length > 0);

    if (entries.length === 0) {
      return undefined;
    }

    return entries.reduce<Record<string, string>>((result, attribute) => {
      result[attribute.key] = attribute.value;
      return result;
    }, {});
  }, [attributes]);

  const resetForm = () => {
    setFormState((previous) => ({
      name: '',
      price: 0,
      inventory: 0,
      seller_id: previous.seller_id
    }));
    setAttributes([createEmptyAttribute()]);
  };

  const validateForm = () => {
    if (!formState.name.trim()) {
      return 'Vui lòng nhập tên sản phẩm.';
    }
    if (!Number.isFinite(formState.price) || formState.price <= 0) {
      return 'Giá sản phẩm phải lớn hơn 0.';
    }
    if (!Number.isFinite(formState.inventory) || formState.inventory < 0) {
      return 'Số lượng tồn kho phải lớn hơn hoặc bằng 0.';
    }
    if (!Number.isFinite(formState.seller_id) || formState.seller_id <= 0) {
      return 'Mã người bán không hợp lệ.';
    }
    return null;
  };

  const handleReview = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const validationMessage = validateForm();
    if (validationMessage) {
      setStatus({ type: 'error', message: validationMessage });
      return;
    }

    setStatus(null);
    setStep('confirm');
  };

  const handleConfirm = async () => {
    setLoading(true);
    setStatus(null);

    try {
      const token = await getValidAccessToken();
      if (!token) {
        throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      }

      const payload: CreateProductInput = {
        ...formState,
        attributes: sanitizedAttributes
      };

      const result = await createProductRequest(payload, token);
      setStatus({ type: 'success', message: result.message ?? 'Tạo sản phẩm thành công.' });
      resetForm();
      setStep('form');
    } catch (error) {
      setStatus({ type: 'error', message: (error as Error).message });
    } finally {
      setLoading(false);
    }
  };

  const handleEdit = () => {
    setStep('form');
    setStatus(null);
  };

  const updateAttribute = (index: number, field: keyof AttributeRow, value: string) => {
    setAttributes((previous) =>
      previous.map((attribute, attributeIndex) =>
        attributeIndex === index ? { ...attribute, [field]: value } : attribute
      )
    );
  };

  const addAttribute = () => {
    setAttributes((previous) => [...previous, createEmptyAttribute()]);
  };

  const removeAttribute = (index: number) => {
    setAttributes((previous) => previous.filter((_, attributeIndex) => attributeIndex !== index));
  };

  return (
    <div className="card grid gap-4">
      {step === 'form' ? (
        <form className="grid gap-4" onSubmit={handleReview}>
          <h2 className="text-2xl font-bold text-slate-900">Tạo sản phẩm mới</h2>
          <label className="grid gap-2 text-sm font-medium text-slate-700">
            Tên sản phẩm
            <input
              type="text"
              value={formState.name}
              onChange={(event) => setFormState((prev) => ({ ...prev, name: event.target.value }))}
              required
            />
          </label>
          <label className="grid gap-2 text-sm font-medium text-slate-700">
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
          <label className="grid gap-2 text-sm font-medium text-slate-700">
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
          <label className="grid gap-1 text-sm font-medium text-slate-700">
            <span>Mã người bán</span>
            <input
              type="number"
              value={formState.seller_id}
              readOnly
              min={1}
              required
              className="cursor-not-allowed bg-slate-100"
            />
            <span className="text-xs font-normal text-slate-500">
              Trường này được liên kết với tài khoản hiện tại và không thể thay đổi.
            </span>
          </label>
          <div className="grid gap-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-semibold text-slate-700">Thuộc tính sản phẩm</span>
              <button
                type="button"
                onClick={addAttribute}
                className="rounded-xl border border-dashed border-indigo-400 px-4 py-2 text-sm font-semibold text-indigo-600 transition hover:bg-indigo-50"
              >
                Thêm thuộc tính
              </button>
            </div>
            {attributes.map((attribute, index) => (
              <div key={`attribute-${index}`} className="grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4">
                <div className="grid gap-3 sm:grid-cols-2 sm:gap-4">
                  <label className="grid gap-1 text-sm font-medium text-slate-700">
                    Tên thuộc tính
                    <input
                      type="text"
                      value={attribute.key}
                      onChange={(event) => updateAttribute(index, 'key', event.target.value)}
                      placeholder="Ví dụ: Màu sắc"
                    />
                  </label>
                  <label className="grid gap-1 text-sm font-medium text-slate-700">
                    Giá trị
                    <input
                      type="text"
                      value={attribute.value}
                      onChange={(event) => updateAttribute(index, 'value', event.target.value)}
                      placeholder="Ví dụ: Đỏ"
                    />
                  </label>
                </div>
                {attributes.length > 1 ? (
                  <button
                    type="button"
                    onClick={() => removeAttribute(index)}
                    className="w-full rounded-xl border border-rose-200 bg-white px-4 py-2 text-sm font-semibold text-rose-600 transition hover:bg-rose-50"
                  >
                    Xóa thuộc tính
                  </button>
                ) : null}
              </div>
            ))}
          </div>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <button
              type="submit"
              className="rounded-xl bg-indigo-600 px-5 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600"
            >
              Xem lại thông tin
            </button>
          </div>
        </form>
      ) : (
        <div className="grid gap-4">
          <h2 className="text-2xl font-bold text-slate-900">Xác nhận tạo sản phẩm</h2>
          <div className="grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 text-sm text-slate-700">
            <div className="flex items-center justify-between">
              <span className="text-slate-600">Tên sản phẩm</span>
              <span className="font-semibold text-slate-900">{formState.name}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-600">Giá bán</span>
              <span className="font-semibold text-slate-900">
                {formState.price.toLocaleString('vi-VN')} VND
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-600">Tồn kho</span>
              <span className="font-semibold text-slate-900">{formState.inventory}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-600">Mã người bán</span>
              <span className="font-semibold text-slate-900">{formState.seller_id}</span>
            </div>
            <div className="grid gap-2">
              <span className="text-slate-600">Thuộc tính</span>
              {sanitizedAttributes ? (
                <ul className="grid gap-2">
                  {Object.entries(sanitizedAttributes).map(([key, value]) => (
                    <li
                      key={key}
                      className="flex items-start justify-between gap-3 rounded-xl border border-slate-200 bg-white px-3 py-2"
                    >
                      <span className="font-medium text-slate-700">{key}</span>
                      <span className="text-right text-slate-600">{String(value)}</span>
                    </li>
                  ))}
                </ul>
              ) : (
                <span className="text-slate-600">Không có thuộc tính bổ sung.</span>
              )}
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <button
              type="button"
              onClick={handleEdit}
              disabled={loading}
              className="rounded-xl border border-slate-300 px-5 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
            >
              Chỉnh sửa
            </button>
            <button
              type="button"
              onClick={handleConfirm}
              disabled={loading}
              className="rounded-xl bg-indigo-600 px-5 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-70"
            >
              {loading ? 'Đang tạo...' : 'Xác nhận tạo sản phẩm'}
            </button>
          </div>
        </div>
      )}
      {status ? (
        <p
          className={`text-sm ${
            status.type === 'error' ? 'text-rose-600' : 'text-emerald-600'
          }`}
        >
          {status.message}
        </p>
      ) : null}
    </div>
  );
}
