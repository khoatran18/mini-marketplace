'use client';

import Link from 'next/link';
import { ProductCard } from '../components/ProductCard';
import { EmptyBlock, ErrorBlock, LoadingBlock } from '../components/ui/StateBlock';
import { getCategoriesRequest, searchProductsRequest } from '../lib/api';
import { useApiData } from '../lib/hooks';

function Shelf({ title, sort, surface, href }: { title: string; sort: string; surface: 'home_trending' | 'home_new'; href: string }) {
  const list = useApiData((token) => searchProductsRequest({ sort, page: 1, page_size: 4 }, token), [sort], { auth: 'optional' });
  const products = list.data?.products ?? [];
  return (
    <section className="grid gap-4">
      <div className="flex items-end justify-between gap-3">
        <h2 className="text-2xl font-bold text-text">{title}</h2>
        <Link href={href} className="text-sm font-semibold text-brand">
          Xem tất cả →
        </Link>
      </div>
      {list.loading ? <LoadingBlock label="Đang tải sản phẩm…" /> : null}
      {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
      {!list.loading && !list.error && products.length === 0 ? <EmptyBlock>Chưa có sản phẩm.</EmptyBlock> : null}
      <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
        {products.map((product, index) => (
          <ProductCard key={product.id} product={product} surface={surface} position={index + 1} />
        ))}
      </div>
    </section>
  );
}

export function HomeSections() {
  const categories = useApiData(() => getCategoriesRequest(), [], { auth: 'none' });
  const roots = (categories.data?.categories ?? []).filter((category) => !category.parent_id && category.active !== false);
  return (
    <>
      {roots.length > 0 ? (
        <section className="grid gap-3" aria-label="Danh mục">
          <h2 className="text-2xl font-bold text-text">Danh mục</h2>
          <ul className="flex flex-wrap gap-2">
            {roots.map((category) => (
              <li key={category.id}>
                <Link href={`/categories/${category.slug}`} className="inline-block rounded-full border border-line bg-surface px-4 py-2 text-sm font-semibold text-text transition hover:border-brand hover:text-brand">
                  {category.name} <span className="text-xs text-muted">({category.product_count})</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      <Shelf title="Bán chạy" sort="best_selling" surface="home_trending" href="/products?sort=best_selling" />
      <Shelf title="Mới lên kệ" sort="newest" surface="home_new" href="/products?sort=newest" />
    </>
  );
}
