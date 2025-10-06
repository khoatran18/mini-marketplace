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
    <article className="card" style={{ display: 'grid', gap: '1rem' }}>
      <div>
        <h3 style={{ margin: 0, fontSize: '1.25rem', fontWeight: 700 }}>{product.name}</h3>
        <p style={{ margin: 0, color: '#6b7280' }}>Mã sản phẩm: {product.id ?? 'Chưa xác định'}</p>
      </div>
      <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap', alignItems: 'center' }}>
        <span style={{ fontWeight: 700, color: '#4f46e5', fontSize: '1.1rem' }}>
          {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(
            product.price ?? 0
          )}
        </span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      {attributeEntries.length > 0 ? (
        <div style={{ display: 'grid', gap: '0.5rem' }}>
          <h4 style={{ margin: '0 0 0.25rem', fontSize: '1rem' }}>Thuộc tính</h4>
          <dl
            style={{
              display: 'grid',
              gap: '0.35rem',
              margin: 0,
              gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))'
            }}
          >
            {attributeEntries.map(([key, value]) => (
              <div
                key={key}
                style={{
                  backgroundColor: '#f9fafb',
                  borderRadius: '0.75rem',
                  padding: '0.5rem 0.75rem'
                }}
              >
                <dt style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#6b7280' }}>
                  {key}
                </dt>
                <dd style={{ margin: 0, fontWeight: 600 }}>{formatAttributeValue(value)}</dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Link href={`/products/${product.id}`} style={{ color: '#4338ca', fontWeight: 600 }}>
          Xem chi tiết →
        </Link>
        <AddToCartControl product={product} buttonVariant="ghost" />
      </div>
    </article>
  );
}
