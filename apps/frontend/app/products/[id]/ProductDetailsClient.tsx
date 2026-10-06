'use client';

import { useEffect, useState } from 'react';
import { getProductByIdRequest } from '../../../lib/api';
import type { Product } from '../../../lib/types';
import { useAuth } from '../../../components/auth/AuthProvider';
import { AddToCartControl } from '../../../components/AddToCartControl';

interface Props {
  productId: number;
}

export function ProductDetailsClient({ productId }: Props) {
  const { accessToken, getValidAccessToken } = useAuth();
  const [product, setProduct] = useState<Product | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const token = await getValidAccessToken();
        if (!token) {
          throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
        }
        const response = await getProductByIdRequest(productId, token);
        if (!cancelled) {
          setProduct(response.product ?? null);
        }
      } catch (err) {
        if (!cancelled) {
          setError((err as Error).message);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [productId, accessToken, getValidAccessToken]);

  if (loading) {
    return <p className="text-sm text-slate-600">Đang tải thông tin sản phẩm...</p>;
  }

  if (error) {
    return (
      <div className="card border border-rose-200 bg-rose-50 text-sm font-medium text-rose-700">
        <strong className="font-semibold">Lỗi:</strong> {error}
      </div>
    );
  }

  if (!product) {
    return <div className="card text-sm text-slate-600">Không tìm thấy sản phẩm.</div>;
  }

  const attributes = product.attributes ?? {};
  const attributeEntries = Object.entries(attributes);

  const formatAttributeValue = (value: unknown): string => {
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
  };

  return (
    <article className="card grid gap-6">
      <header className="grid gap-1">
        <h1 className="text-3xl font-bold text-slate-900">{product.name}</h1>
        <p className="text-sm text-slate-500">Mã sản phẩm: {product.id}</p>
      </header>
      <div className="flex flex-wrap items-center gap-4 text-sm text-slate-700">
        <span className="text-2xl font-semibold text-indigo-600">
          {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(product.price)}
        </span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      <AddToCartControl product={product} />
      {attributeEntries.length > 0 ? (
        <section className="grid gap-3">
          <h2 className="text-xl font-semibold text-slate-900">Thuộc tính sản phẩm</h2>
          <dl className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
            {attributeEntries.map(([key, value]) => (
              <div key={key} className="grid gap-2 rounded-xl bg-slate-50 px-4 py-3">
                <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{key}</dt>
                <dd className="text-sm font-medium text-slate-800">{formatAttributeValue(value)}</dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}
    </article>
  );
}
