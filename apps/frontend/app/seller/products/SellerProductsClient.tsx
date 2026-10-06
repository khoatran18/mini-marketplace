'use client';

import Link from 'next/link';
import { useState } from 'react';
import { SearchBar } from '../../../components/shop/SearchBar';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../../../components/ui/StateBlock';
import { Pagination } from '../../../components/ui/Pagination';
import { StatusBadge } from '../../../components/ui/StatusBadge';
import { getSellerProductsRequest, setProductStatusRequest } from '../../../lib/api';
import { formatVND } from '../../../lib/format';
import { useApiData, useAuthedAction } from '../../../lib/hooks';

const PAGE_SIZE = 20;

export function SellerProductsClient() {
  const run = useAuthedAction();
  const [q, setQ] = useState('');
  const [page, setPage] = useState(1);
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);
  const list = useApiData((token) => getSellerProductsRequest({ q: q || undefined, page, page_size: PAGE_SIZE, sort: q ? undefined : 'newest' }, token as string), [q, page]);
  const products = list.data?.products ?? [];

  const changeStatus = async (id: number, status: string) => {
    setMessage(null);
    try {
      await run((token) => setProductStatusRequest(id, status, token));
      setMessage({ tone: 'success', text: `Đã đổi trạng thái sản phẩm #${id}.` });
      list.reload();
    } catch (err) {
      setMessage({ tone: 'error', text: (err as Error).message });
    }
  };

  return (
    <div className="grid gap-5">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="grid gap-1">
          <h1 className="text-2xl font-bold text-text">Sản phẩm của cửa hàng</h1>
          <p className="text-sm text-muted">Gồm cả nháp và sản phẩm đang ẩn.</p>
        </div>
        <Link href="/seller/products/new" className="btn-primary">
          + Thêm sản phẩm
        </Link>
      </header>
      <SearchBar className="max-w-md" onSearch={(text) => { setQ(text); setPage(1); }} />
      {message ? <Notice tone={message.tone}>{message.text}</Notice> : null}
      {list.loading ? <LoadingBlock label="Đang tải sản phẩm…" /> : null}
      {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
      {!list.loading && !list.error && products.length === 0 ? <EmptyBlock>Chưa có sản phẩm nào.</EmptyBlock> : null}
      {products.length > 0 ? (
        <div className="card overflow-x-auto p-0">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Sản phẩm của cửa hàng</caption>
            <thead className="text-muted">
              <tr>
                <th scope="col" className="px-4 py-3">Sản phẩm</th>
                <th scope="col" className="px-4 py-3">SKU</th>
                <th scope="col" className="px-4 py-3 text-right">Giá</th>
                <th scope="col" className="px-4 py-3 text-right">Tồn / giữ chỗ</th>
                <th scope="col" className="px-4 py-3">Trạng thái</th>
                <th scope="col" className="px-4 py-3">Thao tác</th>
              </tr>
            </thead>
            <tbody>
              {products.map((product) => (
                <tr key={product.id} className="border-t border-line align-middle">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-3">
                      <div className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-surface2">
                        {product.image_urls?.[0] ? (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img src={product.image_urls[0]} alt="" className="h-full w-full object-contain" />
                        ) : null}
                      </div>
                      <div className="min-w-0">
                        <Link href={`/products/${product.id}`} className="font-semibold text-text hover:underline">
                          {product.name}
                        </Link>
                        <p className="text-xs text-muted">#{product.id} · {product.category_name || 'Chưa có danh mục'}</p>
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-muted">{product.sku || '–'}</td>
                  <td className="whitespace-nowrap px-4 py-3 text-right font-semibold text-text">{formatVND(product.price)}</td>
                  <td className="whitespace-nowrap px-4 py-3 text-right text-text">
                    {product.inventory} / {product.reserved ?? 0}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge kind="product" value={product.status} />
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <Link href={`/seller/products/${product.id}/edit`} className="btn">
                        Sửa
                      </Link>
                      {product.status !== 'banned' ? (
                        <select
                          aria-label={`Trạng thái sản phẩm ${product.name}`}
                          className="w-auto py-1.5 text-sm"
                          value={product.status}
                          onChange={(event) => void changeStatus(product.id as number, event.target.value)}
                        >
                          <option value="draft">Nháp</option>
                          <option value="active">Đang bán</option>
                          <option value="hidden">Ẩn</option>
                        </select>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <Pagination page={page} pageSize={PAGE_SIZE} total={list.data?.total ?? 0} onChange={setPage} />
    </div>
  );
}
