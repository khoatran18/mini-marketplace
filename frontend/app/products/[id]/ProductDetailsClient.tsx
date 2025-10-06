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
  const { accessToken } = useAuth();
  const [product, setProduct] = useState<Product | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await getProductByIdRequest(productId, accessToken);
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
  }, [productId, accessToken]);

  if (loading) {
    return <p>Đang tải thông tin sản phẩm...</p>;
  }

  if (error) {
    return (
      <div className="card" style={{ color: '#b91c1c' }}>
        <strong>Lỗi:</strong> {error}
      </div>
    );
  }

  if (!product) {
    return (
      <div className="card">
        <p style={{ margin: 0 }}>Không tìm thấy sản phẩm.</p>
      </div>
    );
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
    <article className="card" style={{ display: 'grid', gap: '1.5rem' }}>
      <header style={{ display: 'grid', gap: '0.35rem' }}>
        <h1 style={{ margin: 0 }}>{product.name}</h1>
        <p style={{ margin: 0, color: '#6b7280' }}>Mã sản phẩm: {product.id}</p>
      </header>
      <div style={{ display: 'flex', gap: '1.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
        <span style={{ fontWeight: 700, color: '#4f46e5', fontSize: '1.35rem' }}>
          {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(
            product.price
          )}
        </span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      <AddToCartControl product={product} />
      {attributeEntries.length > 0 ? (
        <section style={{ display: 'grid', gap: '0.75rem' }}>
          <h2 style={{ margin: 0 }}>Thuộc tính sản phẩm</h2>
          <dl
            style={{
              display: 'grid',
              gap: '0.5rem',
              margin: 0,
              gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))'
            }}
          >
            {attributeEntries.map(([key, value]) => (
              <div
                key={key}
                style={{
                  backgroundColor: '#f9fafb',
                  borderRadius: '0.75rem',
                  padding: '0.75rem 1rem',
                  display: 'grid',
                  gap: '0.25rem'
                }}
              >
                <dt style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#6b7280' }}>
                  {key}
                </dt>
                <dd style={{ margin: 0, fontWeight: 600 }}>{formatAttributeValue(value)}</dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}
    </article>
  );
}
