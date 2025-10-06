'use client';

import { useEffect, useState } from 'react';
import { getProductByIdRequest } from '../../../lib/api';
import type { Product } from '../../../lib/types';
import { useAuth } from '../../../components/auth/AuthProvider';

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

  return (
    <article className="card" style={{ display: 'grid', gap: '1.25rem' }}>
      <header>
        <h1 style={{ margin: 0 }}>{product.name}</h1>
        <p style={{ margin: 0, color: '#6b7280' }}>Mã sản phẩm: {product.id}</p>
      </header>
      <div style={{ display: 'flex', gap: '1.5rem', flexWrap: 'wrap' }}>
        <span style={{ fontWeight: 700, color: '#4f46e5', fontSize: '1.25rem' }}>
          {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(
            product.price
          )}
        </span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      {product.attributes ? (
        <section>
          <h2 style={{ marginTop: 0 }}>Thuộc tính sản phẩm</h2>
          <pre
            style={{
              margin: 0,
              backgroundColor: '#f9fafb',
              padding: '1rem',
              borderRadius: '0.75rem',
              fontSize: '0.9rem',
              overflowX: 'auto'
            }}
          >
            {JSON.stringify(product.attributes, null, 2)}
          </pre>
        </section>
      ) : null}
    </article>
  );
}
