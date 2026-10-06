'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { getProductByIdRequest, getProductStockRequest } from '../../../lib/api';
import { formatVND } from '../../../lib/format';
import { useApiData } from '../../../lib/hooks';
import { track } from '../../../lib/tracking';
import { AddToCartControl } from '../../../components/AddToCartControl';
import { StockBadge } from '../../../components/shop/StockBadge';
import { ErrorBlock, LoadingBlock } from '../../../components/ui/StateBlock';
import { StatusBadge } from '../../../components/ui/StatusBadge';

interface Props {
  productId: number;
}

function formatAttributeValue(value: unknown): string {
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
}

export function ProductDetailsClient({ productId }: Props) {
  // public endpoints: the token is only used so owners can see their own drafts
  const product = useApiData((token) => getProductByIdRequest(productId, token), [productId], { auth: 'optional' });
  const stock = useApiData(() => getProductStockRequest(productId), [productId], { auth: 'none' });
  const [imageIndex, setImageIndex] = useState(0);

  const item = product.data?.product ?? null;

  useEffect(() => {
    if (item?.id) track('product_view', {}, { surface: 'pdp', item: { product_id: item.id } });
  }, [item?.id]);

  if (product.loading) return <LoadingBlock label="Đang tải thông tin sản phẩm…" />;
  if (product.error) {
    return <ErrorBlock message={product.status === 404 ? 'Không tìm thấy sản phẩm.' : product.error} onRetry={product.reload} />;
  }
  if (!item) return <div className="card text-sm text-muted">Không tìm thấy sản phẩm.</div>;

  const level = stock.data?.level ?? item.stock_level;
  const images = item.image_urls ?? [];
  const attributeEntries = Object.entries(item.attributes ?? {});
  const available: typeof item = { ...item, stock_level: level };

  return (
    <article className="card grid gap-8">
      <div className="grid gap-8 md:grid-cols-2">
        <div className="grid content-start gap-3">
          <div className="flex aspect-square items-center justify-center overflow-hidden rounded-2xl bg-surface2">
            {images[imageIndex] ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={images[imageIndex]} alt={item.name} className="h-full w-full object-contain" />
            ) : (
              <span className="text-sm text-muted">Chưa có ảnh</span>
            )}
          </div>
          {images.length > 1 ? (
            <ul className="flex flex-wrap gap-2">
              {images.map((url, index) => (
                <li key={url}>
                  <button
                    type="button"
                    aria-label={`Ảnh ${index + 1}`}
                    aria-pressed={index === imageIndex}
                    onClick={() => setImageIndex(index)}
                    className={`h-16 w-16 overflow-hidden rounded-lg border bg-surface2 ${index === imageIndex ? 'border-brand' : 'border-line'}`}
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img src={url} alt="" className="h-full w-full object-cover" />
                  </button>
                </li>
              ))}
            </ul>
          ) : null}
        </div>

        <div className="grid content-start gap-4">
          <header className="grid gap-1">
            <p className="text-sm text-muted">
              {item.category_id ? (
                <Link href={`/products?category_id=${item.category_id}`} className="font-semibold text-brand">
                  {item.category_name || 'Danh mục'}
                </Link>
              ) : null}
              {item.brand ? <span> · {item.brand}</span> : null}
            </p>
            <h1 className="text-3xl font-bold text-text">{item.name}</h1>
            <p className="text-sm text-muted">
              Mã sản phẩm: {item.id}
              {item.sku ? ` · SKU: ${item.sku}` : ''}
            </p>
          </header>
          <div className="flex flex-wrap items-center gap-4">
            <span className="text-3xl font-semibold text-brand">{formatVND(item.price)}</span>
            <StockBadge level={level} inventory={item.inventory} />
            {item.status && item.status !== 'active' ? <StatusBadge kind="product" value={item.status} /> : null}
          </div>
          <p className="text-sm text-muted">
            Còn lại: {item.inventory > 20 ? '20+' : item.inventory} sản phẩm
            {item.sold ? ` · Đã bán ${item.sold}` : ''}
            {item.weight_g ? ` · ${item.weight_g} g` : ''}
          </p>
          <AddToCartControl product={available} buttonVariant="solid" />
          {item.tags && item.tags.length > 0 ? (
            <ul className="flex flex-wrap gap-2">
              {item.tags.map((tag) => (
                <li key={tag} className="rounded-full bg-surface2 px-3 py-1 text-xs font-semibold text-muted">
                  #{tag}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      </div>

      {item.description ? (
        <section className="grid gap-2">
          <h2 className="text-xl font-semibold text-text">Mô tả</h2>
          {/* plain text only: user content is never rendered as HTML */}
          <p className="whitespace-pre-wrap break-words text-sm leading-relaxed text-text">{item.description}</p>
        </section>
      ) : null}

      {attributeEntries.length > 0 ? (
        <section className="grid gap-3">
          <h2 className="text-xl font-semibold text-text">Thuộc tính sản phẩm</h2>
          <dl className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
            {attributeEntries.map(([key, value]) => (
              <div key={key} className="grid gap-2 rounded-xl bg-surface2 px-4 py-3">
                <dt className="text-xs font-semibold uppercase tracking-wide text-muted">{key}</dt>
                <dd className="text-sm font-medium text-text">{formatAttributeValue(value)}</dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}
    </article>
  );
}
