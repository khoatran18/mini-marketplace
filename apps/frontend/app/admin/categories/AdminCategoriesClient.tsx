'use client';

import { useState } from 'react';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../../../components/ui/StateBlock';
import { getAdminCategoriesRequest, saveCategoryRequest } from '../../../lib/api';
import { useApiData, useAuthedAction } from '../../../lib/hooks';
import type { Category } from '../../../lib/types';

interface Draft {
  id?: number;
  parent_id: string;
  name: string;
  slug: string;
  sort: string;
  active: boolean;
}

const blank: Draft = { parent_id: '', name: '', slug: '', sort: '0', active: true };

function slugify(text: string) {
  return text
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/đ/gi, 'd')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

export function AdminCategoriesClient() {
  const run = useAuthedAction();
  const list = useApiData((token) => getAdminCategoriesRequest(token as string), []);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);
  const categories: Category[] = list.data?.categories ?? [];
  const roots = categories.filter((category) => !category.parent_id);

  const save = async () => {
    if (!draft) return;
    setSaving(true);
    setMessage(null);
    try {
      await run((token) =>
        saveCategoryRequest({ parent_id: Number(draft.parent_id) || 0, name: draft.name.trim(), slug: draft.slug.trim(), sort: Number(draft.sort) || 0, active: draft.active }, token, draft.id)
      );
      setMessage({ tone: 'success', text: 'Đã lưu danh mục.' });
      setDraft(null);
      list.reload();
    } catch (err) {
      setMessage({ tone: 'error', text: (err as Error).message });
    } finally {
      setSaving(false);
    }
  };

  const row = (category: Category, nested: boolean) => (
    <tr key={category.id} className="border-t border-line">
      <td className={`px-3 py-2 text-text ${nested ? 'pl-8' : 'font-semibold'}`}>{category.name}</td>
      <td className="px-3 py-2 font-mono text-xs text-muted">{category.slug}</td>
      <td className="px-3 py-2 text-right text-text">{category.sort}</td>
      <td className="px-3 py-2 text-right text-text">{category.product_count}</td>
      <td className="px-3 py-2">
        <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${category.active ? 'bg-success-soft text-success' : 'bg-surface2 text-muted'}`}>{category.active ? 'Hiển thị' : 'Ẩn'}</span>
      </td>
      <td className="px-3 py-2">
        <button type="button" className="btn py-1 text-xs" onClick={() => setDraft({ id: category.id, parent_id: category.parent_id ? String(category.parent_id) : '', name: category.name, slug: category.slug, sort: String(category.sort), active: category.active })}>
          Sửa
        </button>
      </td>
    </tr>
  );

  return (
    <div className="grid gap-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="grid gap-1">
          <h1 className="text-2xl font-bold text-text">Danh mục</h1>
          <p className="text-sm text-muted">Danh mục cha/con; danh mục ẩn không xuất hiện với khách.</p>
        </div>
        <button type="button" className="btn-primary" onClick={() => setDraft({ ...blank })}>
          + Thêm danh mục
        </button>
      </header>

      {message ? <Notice tone={message.tone}>{message.text}</Notice> : null}

      {draft ? (
        <form
          className="card grid gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}
        >
          <h2 className="text-lg font-semibold text-text">{draft.id ? `Sửa danh mục #${draft.id}` : 'Danh mục mới'}</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <label>
              Tên
              <input
                required
                value={draft.name}
                onChange={(event) => setDraft((prev) => (prev ? { ...prev, name: event.target.value, slug: prev.id || prev.slug !== slugify(prev.name) ? prev.slug : slugify(event.target.value) } : prev))}
              />
            </label>
            <label>
              Slug
              <input required pattern="[a-z0-9]+(-[a-z0-9]+)*" value={draft.slug} onChange={(event) => setDraft((prev) => (prev ? { ...prev, slug: event.target.value } : prev))} />
            </label>
            <label>
              Danh mục cha
              <select value={draft.parent_id} onChange={(event) => setDraft((prev) => (prev ? { ...prev, parent_id: event.target.value } : prev))}>
                <option value="">— Gốc —</option>
                {roots.filter((category) => category.id !== draft.id).map((category) => (
                  <option key={category.id} value={category.id}>{category.name}</option>
                ))}
              </select>
            </label>
            <label>
              Thứ tự
              <input type="number" value={draft.sort} onChange={(event) => setDraft((prev) => (prev ? { ...prev, sort: event.target.value } : prev))} />
            </label>
          </div>
          <label className="flex items-center gap-2 font-normal">
            <input type="checkbox" checked={draft.active} onChange={(event) => setDraft((prev) => (prev ? { ...prev, active: event.target.checked } : prev))} />
            Hiển thị với khách
          </label>
          <div className="flex gap-2">
            <button type="submit" className="btn-primary" disabled={saving}>{saving ? 'Đang lưu…' : 'Lưu'}</button>
            <button type="button" className="btn" onClick={() => setDraft(null)}>Huỷ</button>
          </div>
        </form>
      ) : null}

      {list.loading ? <LoadingBlock /> : null}
      {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
      {!list.loading && !list.error && categories.length === 0 ? <EmptyBlock>Chưa có danh mục.</EmptyBlock> : null}
      {categories.length > 0 ? (
        <div className="card overflow-x-auto p-0">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Danh mục</caption>
            <thead className="text-muted">
              <tr>
                <th className="px-3 py-3">Tên</th>
                <th className="px-3 py-3">Slug</th>
                <th className="px-3 py-3 text-right">Thứ tự</th>
                <th className="px-3 py-3 text-right">Sản phẩm</th>
                <th className="px-3 py-3">Trạng thái</th>
                <th className="px-3 py-3" />
              </tr>
            </thead>
            <tbody>
              {roots.flatMap((root) => [row(root, false), ...categories.filter((category) => category.parent_id === root.id).map((child) => row(child, true))])}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
