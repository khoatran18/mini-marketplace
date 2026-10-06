'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useMemo, useState } from 'react';
import { createProductRequest, getCategoriesRequest, updateProductRequest } from '../../lib/api';
import { formatVND } from '../../lib/format';
import { useApiData, useAuthedAction } from '../../lib/hooks';
import type { Category, Product } from '../../lib/types';
import { Notice } from '../ui/StateBlock';

interface AttributeRow {
  key: string;
  value: string;
}

interface FormState {
  name: string;
  sku: string;
  category_id: string;
  brand: string;
  description: string;
  price: string;
  inventory: string;
  low_stock_threshold: string;
  weight_g: string;
  images: string;
  tags: string;
  status: string;
}

function toState(product?: Product): FormState {
  return {
    name: product?.name ?? '',
    sku: product?.sku ?? '',
    category_id: product?.category_id ? String(product.category_id) : '',
    brand: product?.brand ?? '',
    description: product?.description ?? '',
    price: product ? String(product.price) : '',
    inventory: product ? String(product.inventory) : '0',
    low_stock_threshold: product ? String(product.low_stock_threshold ?? 0) : '0',
    weight_g: product ? String(product.weight_g ?? 0) : '0',
    images: (product?.image_urls ?? []).join('\n'),
    tags: (product?.tags ?? []).join(', '),
    status: product?.status ?? 'draft'
  };
}

function toAttributeRows(product?: Product): AttributeRow[] {
  const entries = Object.entries(product?.attributes ?? {}).map(([key, value]) => ({ key, value: typeof value === 'string' ? value : JSON.stringify(value) }));
  return entries.length > 0 ? entries : [{ key: '', value: '' }];
}

function categoryLabel(category: Category, all: Category[]) {
  const parent = all.find((item) => item.id === category.parent_id);
  return parent ? `${parent.name} › ${category.name}` : category.name;
}

/** Create / edit form with every catalog field (sku, category, brand, description, stock threshold, images, tags, status ...). */
export function ProductForm({ product }: { product?: Product }) {
  const router = useRouter();
  const run = useAuthedAction();
  const categories = useApiData(() => getCategoriesRequest(), [], { auth: 'none' });
  const [form, setForm] = useState<FormState>(() => toState(product));
  const [attributes, setAttributes] = useState<AttributeRow[]>(() => toAttributeRows(product));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const list = useMemo(() => categories.data?.categories ?? [], [categories.data]);
  const set = (key: keyof FormState) => (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    setSaved(false);
    setForm((prev) => ({ ...prev, [key]: event.target.value }));
  };

  const imageUrls = form.images.split('\n').map((line) => line.trim()).filter(Boolean);
  const tags = form.tags.split(',').map((tag) => tag.trim()).filter(Boolean);
  const attributeMap = attributes.reduce<Record<string, string>>((result, row) => {
    if (row.key.trim()) result[row.key.trim()] = row.value;
    return result;
  }, {});

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    setSaved(false);
    const price = Number(form.price);
    const inventory = Number(form.inventory);
    if (!form.name.trim()) return setError('Vui lòng nhập tên sản phẩm.');
    if (!Number.isFinite(price) || price <= 0) return setError('Giá phải lớn hơn 0.');
    if (!Number.isInteger(inventory) || inventory < 0) return setError('Tồn kho phải là số nguyên không âm.');
    if (imageUrls.some((url) => !/^https?:\/\//i.test(url))) return setError('Mỗi ảnh phải là một đường dẫn http(s).');

    const payload = {
      name: form.name.trim(),
      sku: form.sku.trim(),
      category_id: Number(form.category_id) || 0,
      brand: form.brand.trim(),
      description: form.description,
      price,
      inventory,
      low_stock_threshold: Number(form.low_stock_threshold) || 0,
      weight_g: Number(form.weight_g) || 0,
      image_urls: imageUrls,
      tags,
      attributes: attributeMap,
      status: form.status
    };

    setSaving(true);
    try {
      if (product?.id) {
        // PUT replaces the product: send the whole record. The seller/store comes from the token, never from the body.
        await run((token) => updateProductRequest(product.id as number, { ...product, ...payload, seller_id: product.seller_id }, token));
        setSaved(true);
      } else {
        const created = await run((token) => createProductRequest(payload, token));
        router.push(created.id ? `/seller/products/${created.id}/edit` : '/seller/products');
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
      <form className="card grid gap-5" onSubmit={(event) => void submit(event)}>
        <h2 className="text-xl font-semibold text-text">{product ? `Sửa sản phẩm #${product.id}` : 'Thêm sản phẩm mới'}</h2>

        <div className="grid gap-4 sm:grid-cols-2">
          <label className="sm:col-span-2">
            Tên sản phẩm
            <input value={form.name} onChange={set('name')} required maxLength={200} />
          </label>
          <label>
            SKU
            <input value={form.sku} onChange={set('sku')} maxLength={64} />
          </label>
          <label>
            Thương hiệu
            <input value={form.brand} onChange={set('brand')} maxLength={100} />
          </label>
          <label>
            Danh mục
            <select value={form.category_id} onChange={set('category_id')}>
              <option value="">— Chưa chọn —</option>
              {list.map((category) => (
                <option key={category.id} value={category.id}>
                  {categoryLabel(category, list)}
                </option>
              ))}
            </select>
          </label>
          <label>
            Trạng thái
            <select value={form.status} onChange={set('status')}>
              <option value="draft">Nháp (chưa hiển thị)</option>
              <option value="active">Đang bán</option>
              <option value="hidden">Ẩn</option>
            </select>
          </label>
          <label>
            Giá (VND)
            <input type="number" min={1} step={1} inputMode="numeric" value={form.price} onChange={set('price')} required />
          </label>
          <label>
            Tồn kho
            <input type="number" min={0} step={1} inputMode="numeric" value={form.inventory} onChange={set('inventory')} required />
          </label>
          <label>
            Ngưỡng cảnh báo tồn thấp
            <input type="number" min={0} step={1} value={form.low_stock_threshold} onChange={set('low_stock_threshold')} />
            <span className="text-xs font-normal">0 = dùng ngưỡng mặc định</span>
          </label>
          <label>
            Khối lượng (gram)
            <input type="number" min={0} step={1} value={form.weight_g} onChange={set('weight_g')} />
          </label>
          <label className="sm:col-span-2">
            Mô tả (văn bản thuần)
            <textarea rows={5} value={form.description} onChange={set('description')} />
          </label>
          <label className="sm:col-span-2">
            Ảnh (mỗi dòng một đường dẫn https)
            <textarea rows={3} value={form.images} onChange={set('images')} placeholder="https://…/anh-1.jpg" />
            <span className="text-xs font-normal">Tải ảnh lên chưa được hỗ trợ ở gateway: dùng đường dẫn ảnh có sẵn. Ảnh đầu tiên là ảnh chính.</span>
          </label>
          <label className="sm:col-span-2">
            Thẻ (phân tách bằng dấu phẩy)
            <input value={form.tags} onChange={set('tags')} placeholder="mới, giảm giá" />
          </label>
        </div>

        <fieldset className="grid gap-3">
          <legend className="text-sm font-semibold text-muted">Thuộc tính (tên / giá trị)</legend>
          {attributes.map((row, index) => (
            <div key={index} className="grid grid-cols-[1fr_1fr_auto] gap-2">
              <input aria-label="Tên thuộc tính" placeholder="VD: Màu sắc" value={row.key} onChange={(event) => setAttributes((prev) => prev.map((item, i) => (i === index ? { ...item, key: event.target.value } : item)))} />
              <input aria-label="Giá trị" placeholder="VD: Đen" value={row.value} onChange={(event) => setAttributes((prev) => prev.map((item, i) => (i === index ? { ...item, value: event.target.value } : item)))} />
              <button type="button" className="btn" aria-label="Xoá thuộc tính" onClick={() => setAttributes((prev) => (prev.length > 1 ? prev.filter((_, i) => i !== index) : [{ key: '', value: '' }]))}>
                ✕
              </button>
            </div>
          ))}
          <div>
            <button type="button" className="btn" onClick={() => setAttributes((prev) => [...prev, { key: '', value: '' }])}>
              + Thêm thuộc tính
            </button>
          </div>
        </fieldset>

        {error ? <Notice tone="error">{error}</Notice> : null}
        {saved ? <Notice tone="success">Đã lưu thay đổi.</Notice> : null}

        <div className="flex flex-wrap gap-3">
          <button type="submit" className="btn-primary" disabled={saving}>
            {saving ? 'Đang lưu…' : product ? 'Lưu thay đổi' : 'Tạo sản phẩm'}
          </button>
          <Link href="/seller/products" className="btn">
            Quay lại danh sách
          </Link>
        </div>
      </form>

      <aside className="card grid h-fit content-start gap-3" aria-label="Xem trước">
        <h3 className="text-sm font-bold uppercase tracking-wide text-muted">Xem trước trang sản phẩm</h3>
        <div className="flex aspect-[4/3] items-center justify-center overflow-hidden rounded-xl bg-surface2">
          {imageUrls[0] ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={imageUrls[0]} alt="" className="h-full w-full object-contain" />
          ) : (
            <span className="text-sm text-muted">Chưa có ảnh</span>
          )}
        </div>
        <p className="break-words text-lg font-bold text-text">{form.name || 'Tên sản phẩm'}</p>
        <p className="text-lg font-semibold text-brand">{formatVND(Number(form.price) || 0)}</p>
        <p className="text-sm text-muted">{[form.brand, list.find((category) => String(category.id) === form.category_id)?.name].filter(Boolean).join(' · ')}</p>
        {form.description ? <p className="line-clamp-4 whitespace-pre-wrap text-sm text-text">{form.description}</p> : null}
        {Object.keys(attributeMap).length > 0 ? (
          <dl className="grid gap-1 text-xs">
            {Object.entries(attributeMap).map(([key, value]) => (
              <div key={key} className="flex justify-between gap-2">
                <dt className="text-muted">{key}</dt>
                <dd className="text-text">{value}</dd>
              </div>
            ))}
          </dl>
        ) : null}
      </aside>
    </div>
  );
}
