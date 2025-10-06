import Link from 'next/link';
import type { Product } from '../lib/types';
import { AddToCartControl } from './AddToCartControl';

interface Props {
  product: Product;
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

export function ProductCard({ product }: Props) {
  const attributes = product.attributes ?? {};
  const attributeEntries = Object.entries(attributes);

  return (
    <article className="card grid gap-4">
      <div>
        <h3 className="text-xl font-bold text-slate-900">{product.name}</h3>
        <p className="text-sm text-slate-500">Mã sản phẩm: {product.id ?? 'Chưa xác định'}</p>
      </div>
      <div className="flex flex-wrap items-center gap-3 text-sm text-slate-700">
        <span className="text-lg font-semibold text-indigo-600">
          {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(product.price ?? 0)}
        </span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      {attributeEntries.length > 0 ? (
        <div className="grid gap-2">
          <h4 className="text-base font-semibold text-slate-900">Thuộc tính</h4>
          <dl className="grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-2">
            {attributeEntries.map(([key, value]) => (
              <div key={key} className="rounded-xl bg-slate-50 px-3 py-2">
                <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{key}</dt>
                <dd className="text-sm font-medium text-slate-800">{formatAttributeValue(value)}</dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}
      <div className="flex items-center justify-between">
        <Link href={`/products/${product.id}`} className="font-semibold text-indigo-600 hover:text-indigo-500">
          Xem chi tiết →
        </Link>
        <AddToCartControl product={product} buttonVariant="ghost" />
      </div>
    </article>
  );
}
