'use client';

import { useEffect, useState } from 'react';
import { getProductsRequest } from '../../lib/api';
import type { Product } from '../../lib/types';
import { useAuth } from '../../components/auth/AuthProvider';
import { ProductCard } from '../../components/ProductCard';

export function ProductsPageClient() {
  const { accessToken, userId, getValidAccessToken } = useAuth();
  const [products, setProducts] = useState<Product[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);

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
        const response = await getProductsRequest(page, 12, token, userId);
        if (!cancelled) {
          setProducts(response.products ?? []);
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
  }, [page, accessToken, getValidAccessToken, userId]);

  return (
    <div className="grid" style={{ gap: '1.5rem' }}>
      <header className="card" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ margin: 0 }}>Danh sách sản phẩm</h1>
          <p style={{ margin: 0, color: '#6b7280' }}>
            Dữ liệu được lấy từ API Gateway. Bạn cần đăng nhập để truy cập.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
          <button
            type="button"
            onClick={() => setPage((prev) => Math.max(prev - 1, 1))}
            style={{
              border: '1px solid #d1d5db',
              background: 'transparent',
              borderRadius: '0.75rem',
              padding: '0.5rem 1rem'
            }}
            disabled={page === 1}
          >
            Trang trước
          </button>
          <span>Trang {page}</span>
          <button
            type="button"
            onClick={() => setPage((prev) => prev + 1)}
            style={{
              border: '1px solid #d1d5db',
              background: 'transparent',
              borderRadius: '0.75rem',
              padding: '0.5rem 1rem'
            }}
          >
            Trang sau
          </button>
        </div>
      </header>
      {loading ? <p>Đang tải sản phẩm...</p> : null}
      {error ? (
        <div className="card" style={{ color: '#b91c1c' }}>
          <strong>Lỗi:</strong> {error}
        </div>
      ) : null}
      <div className="grid grid-cols-3" style={{ gap: '1.5rem' }}>
        {products.map((product) => (
          <ProductCard key={product.id ?? product.name} product={product} />
        ))}
        {!loading && products.length === 0 && !error ? (
          <div className="card">
            <p style={{ margin: 0 }}>Không có sản phẩm để hiển thị.</p>
          </div>
        ) : null}
      </div>
    </div>
  );
}
