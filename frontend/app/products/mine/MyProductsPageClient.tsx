'use client';

import { useEffect, useState } from 'react';
import { ProductCard } from '../../../components/ProductCard';
import { useAuth } from '../../../components/auth/AuthProvider';
import { getProductsBySellerRequest } from '../../../lib/api';
import type { Product } from '../../../lib/types';

export function MyProductsPageClient() {
  const { userId, getValidAccessToken } = useAuth();
  const [products, setProducts] = useState<Product[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    if (userId === null) {
      return () => {
        cancelled = true;
      };
    }

    if (!userId || userId <= 0) {
      setProducts([]);
      setLoading(false);
      setError('Không tìm thấy mã người bán hợp lệ. Vui lòng đăng nhập lại.');
      return () => {
        cancelled = true;
      };
    }

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const token = await getValidAccessToken();
        if (!token) {
          throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
        }
        const response = await getProductsBySellerRequest(userId, token);
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
  }, [userId, getValidAccessToken]);

  return (
    <div className="grid gap-6">
      <header className="card grid gap-2">
        <h1 className="text-2xl font-bold text-slate-900">Sản phẩm của tôi</h1>
        <p className="text-sm text-slate-600">
          Danh sách sản phẩm được lấy theo mã người bán hiện tại thông qua API Gateway.
        </p>
      </header>
      {loading ? <p className="text-sm text-slate-600">Đang tải sản phẩm của bạn...</p> : null}
      {error ? (
        <div className="card border border-rose-200 bg-rose-50 text-sm font-medium text-rose-700">
          <strong className="font-semibold">Lỗi:</strong> {error}
        </div>
      ) : null}
      <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
        {products.map((product) => (
          <ProductCard key={product.id ?? product.name} product={product} />
        ))}
        {!loading && products.length === 0 && !error ? (
          <div className="card text-sm text-slate-600">Bạn chưa có sản phẩm nào.</div>
        ) : null}
      </div>
    </div>
  );
}
