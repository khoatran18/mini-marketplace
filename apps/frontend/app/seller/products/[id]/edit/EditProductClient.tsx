'use client';

import { ProductForm } from '../../../../../components/seller/ProductForm';
import { ErrorBlock, LoadingBlock } from '../../../../../components/ui/StateBlock';
import { getProductByIdRequest } from '../../../../../lib/api';
import { useApiData } from '../../../../../lib/hooks';

export function EditProductClient({ productId }: { productId: number }) {
  // with the seller's token the gateway returns the owner view (exact stock, threshold, any status)
  const loaded = useApiData((token) => getProductByIdRequest(productId, token), [productId]);
  if (loaded.loading) return <LoadingBlock label="Đang tải sản phẩm…" />;
  if (loaded.error) return <ErrorBlock message={loaded.error} onRetry={loaded.reload} />;
  const product = loaded.data?.product;
  if (!product) return <ErrorBlock message="Không tìm thấy sản phẩm." />;
  return <ProductForm product={product} />;
}
