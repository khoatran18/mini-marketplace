import Link from 'next/link';
import type { Product } from '../lib/types';
import { formatVND } from '../lib/format';
import { AddToCartControl } from './AddToCartControl';
import { StockBadge } from './shop/StockBadge';
import { TrackClick } from './tracking/TrackClick';
import { TrackImpression } from './tracking/TrackImpression';
import { StatusBadge } from './ui/StatusBadge';

interface Props {
  product: Product;
  /** where the card is shown (tracking `surface`) */
  surface?: string;
  /** 1-based position in the list (tracking) */
  position?: number;
  /** show the product status (seller views) */
  showStatus?: boolean;
  /** hide the add-to-cart control (seller/admin views) */
  hideCart?: boolean;
}

export function ProductCard({ product, surface = 'search_results', position, showStatus = false, hideCart = false }: Props) {
  const image = product.image_urls?.[0];
  const item = { product_id: product.id, position };
  return (
    <TrackImpression surface={surface} item={item}>
      <article className="card grid h-full gap-4 overflow-hidden">
        <TrackClick surface={surface} item={item}>
          <Link href={`/products/${product.id}`} className="grid gap-3">
            <div className="flex aspect-[4/3] items-center justify-center overflow-hidden rounded-xl bg-surface2">
              {image ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={image} alt={product.name} loading="lazy" className="h-full w-full object-contain" />
              ) : (
                <span className="text-sm text-muted">Chưa có ảnh</span>
              )}
            </div>
            <div className="grid gap-1">
              <h3 className="break-words text-lg font-bold text-text">{product.name}</h3>
              <p className="text-sm text-muted">
                {[product.brand, product.category_name].filter(Boolean).join(' · ') || `Mã sản phẩm: ${product.id ?? '–'}`}
              </p>
            </div>
          </Link>
        </TrackClick>
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <span className="text-lg font-semibold text-brand">{formatVND(product.price)}</span>
          <StockBadge level={product.stock_level} inventory={product.inventory} />
          {showStatus ? <StatusBadge kind="product" value={product.status} /> : null}
          {product.sold ? <span className="text-muted">Đã bán {product.sold}</span> : null}
        </div>
        {hideCart ? null : (
          <div className="mt-auto">
            <AddToCartControl product={product} buttonVariant="ghost" />
          </div>
        )}
      </article>
    </TrackImpression>
  );
}
