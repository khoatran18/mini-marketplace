'use client';

import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useMemo, useState } from 'react';
import { getCategoriesRequest, searchProductsRequest } from '../../lib/api';
import { useApiData } from '../../lib/hooks';
import { track } from '../../lib/tracking';
import type { Category, SearchParams } from '../../lib/types';
import { ProductCard } from '../ProductCard';
import { EmptyBlock, ErrorBlock, LoadingBlock } from '../ui/StateBlock';
import { Pagination } from '../ui/Pagination';
import { CategoryNav } from './CategoryNav';
import { SearchBar } from './SearchBar';

const PAGE_SIZE = 12;

const sortOptions = [
  { value: 'relevance', label: 'Phù hợp nhất' },
  { value: 'newest', label: 'Mới nhất' },
  { value: 'best_selling', label: 'Bán chạy' },
  { value: 'price_asc', label: 'Giá tăng dần' },
  { value: 'price_desc', label: 'Giá giảm dần' }
];

interface Props {
  /** set on /categories/[slug]: the category is fixed by the route */
  categorySlug?: string;
}

/** Catalog: keyword search, category tree, filters and sorting. The state lives in the URL so results can be shared. */
export function ProductsBrowser({ categorySlug }: Props) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();

  const q = params.get('q') ?? '';
  const sort = params.get('sort') ?? 'relevance';
  const brand = params.get('brand') ?? '';
  const priceMin = params.get('price_min') ?? '';
  const priceMax = params.get('price_max') ?? '';
  const inStock = params.get('in_stock') === 'true';
  const page = Math.max(1, Number(params.get('page')) || 1);
  const categoryParam = Number(params.get('category_id')) || 0;

  const categories = useApiData(() => getCategoriesRequest(), [], { auth: 'none' });
  const list: Category[] = useMemo(() => categories.data?.categories ?? [], [categories.data]);
  const fixed = categorySlug ? list.find((category) => category.slug === categorySlug) : undefined;
  const categoryId = fixed?.id ?? categoryParam;
  const waitingForCategory = Boolean(categorySlug) && categories.loading;

  const search: SearchParams = {
    q: q || undefined,
    category_id: categoryId || undefined,
    price_min: Number(priceMin) || undefined,
    price_max: Number(priceMax) || undefined,
    brand: brand || undefined,
    in_stock: inStock || undefined,
    sort: sort === 'relevance' && !q ? undefined : sort,
    page,
    page_size: PAGE_SIZE
  };

  const result = useApiData((token) => searchProductsRequest(search, token), [JSON.stringify(search)], {
    auth: 'optional',
    enabled: !categorySlug || Boolean(fixed)
  });

  useEffect(() => {
    if (result.data && q) {
      track('search', { q, filters: { category_id: categoryId, brand, priceMin, priceMax, inStock }, result_count: result.data.total ?? 0 }, { surface: 'search_results' });
    }
    // only when a new result set for a query arrives
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [result.data]);

  const update = (changes: Record<string, string | null>, event?: { type: 'filter_apply' | 'sort_change'; filter: string; value: string }) => {
    const next = new URLSearchParams(params.toString());
    for (const [key, value] of Object.entries(changes)) {
      if (value === null || value === '') next.delete(key);
      else next.set(key, value);
    }
    if (!('page' in changes)) next.delete('page');
    const text = next.toString();
    router.replace(text ? `${pathname}?${text}` : pathname);
    if (event) track(event.type, { filter: event.filter, value: event.value }, { surface: categorySlug ? 'category_page' : 'search_results' });
  };

  const [draft, setDraft] = useState({ brand, priceMin, priceMax });
  useEffect(() => setDraft({ brand, priceMin, priceMax }), [brand, priceMin, priceMax]);

  const products = result.data?.products ?? [];
  const total = result.data?.total ?? 0;
  const surface = categorySlug ? 'category_page' : 'search_results';
  const categoryMissing = Boolean(categorySlug) && !categories.loading && !categories.error && !fixed;

  return (
    <div className="grid gap-6 lg:grid-cols-[260px_1fr]">
      <aside className="grid content-start gap-5">
        <div className="card grid gap-3 p-4">
          <h2 className="text-sm font-bold uppercase tracking-wide text-muted">Danh mục</h2>
          <Link href="/products" className={`rounded-lg px-3 py-1.5 text-sm font-semibold ${!categoryId ? 'bg-brand-soft text-brand' : 'text-text hover:bg-surface2'}`}>
            Tất cả sản phẩm
          </Link>
          {categories.loading ? <p className="text-sm text-muted">Đang tải…</p> : null}
          {categories.error ? <p className="text-sm text-danger">Không tải được danh mục.</p> : null}
          <CategoryNav categories={list} activeId={categoryId || undefined} />
        </div>
        <form
          className="card grid gap-3 p-4"
          onSubmit={(event) => {
            event.preventDefault();
            update(
              { brand: draft.brand.trim(), price_min: draft.priceMin, price_max: draft.priceMax },
              { type: 'filter_apply', filter: 'brand_price', value: `${draft.brand}|${draft.priceMin}-${draft.priceMax}` }
            );
          }}
        >
          <h2 className="text-sm font-bold uppercase tracking-wide text-muted">Bộ lọc</h2>
          <label>
            Thương hiệu
            <input value={draft.brand} onChange={(event) => setDraft((prev) => ({ ...prev, brand: event.target.value }))} />
          </label>
          <div className="grid grid-cols-2 gap-2">
            <label>
              Giá từ
              <input type="number" min={0} inputMode="numeric" value={draft.priceMin} onChange={(event) => setDraft((prev) => ({ ...prev, priceMin: event.target.value }))} />
            </label>
            <label>
              Đến
              <input type="number" min={0} inputMode="numeric" value={draft.priceMax} onChange={(event) => setDraft((prev) => ({ ...prev, priceMax: event.target.value }))} />
            </label>
          </div>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={inStock}
              onChange={(event) => update({ in_stock: event.target.checked ? 'true' : null }, { type: 'filter_apply', filter: 'in_stock', value: String(event.target.checked) })}
            />
            Chỉ hiện hàng còn
          </label>
          <div className="flex gap-2">
            <button type="submit" className="btn-primary flex-1">
              Áp dụng
            </button>
            <button type="button" className="btn" onClick={() => update({ brand: null, price_min: null, price_max: null, in_stock: null, q: null, sort: null })}>
              Xoá lọc
            </button>
          </div>
        </form>
      </aside>

      <section className="grid content-start gap-5">
        <header className="card grid gap-4">
          <div className="grid gap-1">
            <h1 className="text-2xl font-bold text-text">{fixed ? fixed.name : q ? `Kết quả cho “${q}”` : 'Danh sách sản phẩm'}</h1>
            <p className="text-sm text-muted" aria-live="polite">
              {result.loading ? 'Đang tìm…' : `${total} sản phẩm`}
            </p>
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <SearchBar key={q} initial={q} className="sm:w-96" onSearch={(text) => update({ q: text || null })} />
            <label className="flex items-center gap-2 sm:w-64">
              <span className="whitespace-nowrap">Sắp xếp</span>
              <select value={sort} onChange={(event) => update({ sort: event.target.value }, { type: 'sort_change', filter: 'sort', value: event.target.value })}>
                {sortOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </header>

        {categoryMissing ? <EmptyBlock>Không tìm thấy danh mục này.</EmptyBlock> : null}
        {result.loading || waitingForCategory ? <LoadingBlock label="Đang tải sản phẩm…" /> : null}
        {result.error ? <ErrorBlock message={result.error} onRetry={result.reload} /> : null}
        {!result.loading && !result.error && !categoryMissing && products.length === 0 ? <EmptyBlock>Không có sản phẩm phù hợp.</EmptyBlock> : null}

        <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
          {products.map((product, index) => (
            <ProductCard key={product.id ?? product.name} product={product} surface={surface} position={(page - 1) * PAGE_SIZE + index + 1} />
          ))}
        </div>
        <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={(next) => update({ page: String(next) })} />
      </section>
    </div>
  );
}
