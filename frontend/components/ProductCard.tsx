import Link from 'next/link';
import type { Product } from '../lib/types';
import { AddToCartControl } from './AddToCartControl';

interface Props {
  product: Product;
}

export function ProductCard({ product }: Props) {
  
  return (
    <article className="card grid gap-4">
      <div className="grid gap-2">
        <div className="grid gap-1">
          <h3 className="text-xl font-bold text-slate-900 break-words">{product.name}</h3>
          <p className="text-sm text-slate-500">Mã sản phẩm: {product.id ?? 'Chưa xác định'}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3 text-sm text-slate-700">
          <span className="text-lg font-semibold text-indigo-600">
            {new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(product.price ?? 0)}
          </span>
          <span>Tồn kho: {product.inventory}</span>
          <span>Người bán: {product.seller_id}</span>
        </div>
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Link
          href={`/products/${product.id}`}
          className="font-semibold text-indigo-600 transition hover:text-indigo-500"
        >
          Xem chi tiết →
        </Link>
        <AddToCartControl product={product} buttonVariant="ghost" />
      </div>
    </article>
  );
}
