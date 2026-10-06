'use client';

import { useEffect, useRef, useState } from 'react';
import { getProductsRequest } from '../../lib/api';
import type { Product } from '../../lib/types';
import { useAuth } from '../../components/auth/AuthProvider';
import { ProductCard } from '../../components/ProductCard';

export function ProductsPageClient() {
  const { getValidAccessToken } = useAuth();
  const [products, setProducts] = useState<Product[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const lastNonEmptyPageRef = useRef(1);
  const [finalPage, setFinalPage] = useState<number | null>(null);

  const pageSize = 12;

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
        const response = await getProductsRequest(page, pageSize, token);
        if (!cancelled) {
          const fetchedProducts = response.products ?? [];
          if (fetchedProducts.length === 0) {
            const fallbackLastPage = Math.max(lastNonEmptyPageRef.current, 1);
            setProducts([]);
            setFinalPage((previous) =>
              previous === fallbackLastPage ? previous : fallbackLastPage
            );
            if (page > fallbackLastPage) {
              setPage(fallbackLastPage);
            }
            return;
          }

          setProducts(fetchedProducts);
          lastNonEmptyPageRef.current = page;
          setFinalPage((previous) => {
            if (fetchedProducts.length < pageSize) {
              return previous === page ? previous : page;
            }
            if (previous !== null && page > previous) {
              return null;
            }
            return previous;
          });
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
  }, [page, getValidAccessToken]);

  const isLastPage = finalPage !== null && page >= finalPage;

  return (
    <div className="grid gap-6">
      <header className="card flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div className="grid gap-1">
          <h1 className="text-2xl font-bold text-slate-900">Danh sách sản phẩm</h1>
          <p className="text-sm text-slate-600">Bạn cần đăng nhập để xem và mua hàng.</p>
        </div>
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <button
            type="button"
            onClick={() => setPage((prev) => Math.max(prev - 1, 1))}
            className="rounded-xl border border-slate-300 px-4 py-2 font-semibold text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={page === 1}
          >
            Trang trước
          </button>
          <span className="font-semibold text-slate-700">Trang {page}</span>
          <button
            type="button"
            onClick={() => setPage((prev) => prev + 1)}
            className="rounded-xl border border-slate-300 px-4 py-2 font-semibold text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isLastPage}
          >
            Trang sau
          </button>
        </div>
      </header>
      {loading ? <p className="text-sm text-slate-600">Đang tải sản phẩm...</p> : null}
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
          <div className="card text-sm text-slate-600">
            Không có sản phẩm để hiển thị.
          </div>
        ) : null}
      </div>
    </div>
  );
}
