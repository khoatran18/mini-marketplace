import type { Product } from '../lib/types';

interface Props {
  product: Product;
}

export function ProductCard({ product }: Props) {
  return (
    <article className="card" style={{ display: 'grid', gap: '0.75rem' }}>
      <div>
        <h3 style={{ margin: 0, fontSize: '1.25rem', fontWeight: 700 }}>{product.name}</h3>
        <p style={{ margin: 0, color: '#6b7280' }}>Mã sản phẩm: {product.id ?? 'Chưa xác định'}</p>
      </div>
      <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
        <span style={{ fontWeight: 700, color: '#4f46e5' }}>
          {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(
            product.price ?? 0
          )}
        </span>
        <span>Tồn kho: {product.inventory}</span>
        <span>Người bán: {product.seller_id}</span>
      </div>
      {product.attributes && Object.keys(product.attributes).length > 0 ? (
        <div>
          <h4 style={{ margin: '0 0 0.5rem', fontSize: '1rem' }}>Thuộc tính</h4>
          <pre
            style={{
              margin: 0,
              backgroundColor: '#f9fafb',
              padding: '0.75rem',
              borderRadius: '0.75rem',
              fontSize: '0.85rem',
              overflow: 'auto'
            }}
          >
            {JSON.stringify(product.attributes, null, 2)}
          </pre>
        </div>
      ) : null}
      <a
        href={`/products/${product.id}`}
        style={{ color: '#4338ca', fontWeight: 600, justifySelf: 'flex-start' }}
      >
        Xem chi tiết →
      </a>
    </article>
  );
}
