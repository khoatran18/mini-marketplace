import Link from 'next/link';
import type { Category } from '../../lib/types';

/** Flat category list (parent_id) rendered as a two level tree. */
export function CategoryNav({ categories, activeId }: { categories: Category[]; activeId?: number }) {
  const roots = categories.filter((category) => !category.parent_id).sort((a, b) => a.sort - b.sort || a.name.localeCompare(b.name));
  const children = (id: number) => categories.filter((category) => category.parent_id === id).sort((a, b) => a.sort - b.sort || a.name.localeCompare(b.name));
  const item = (category: Category, nested: boolean) => (
    <li key={category.id}>
      <Link
        href={`/categories/${category.slug}`}
        aria-current={activeId === category.id ? 'page' : undefined}
        className={`flex items-center justify-between rounded-lg px-3 py-1.5 text-sm transition ${nested ? 'ml-4' : 'font-semibold'} ${
          activeId === category.id ? 'bg-brand-soft text-brand' : 'text-text hover:bg-surface2'
        }`}
      >
        <span>{category.name}</span>
        <span className="text-xs text-muted">{category.product_count}</span>
      </Link>
    </li>
  );
  return (
    <nav aria-label="Danh mục">
      <ul className="grid gap-0.5">
        {roots.map((root) => (
          <li key={root.id}>
            <ul className="grid gap-0.5">
              {item(root, false)}
              {children(root.id).map((child) => item(child, true))}
            </ul>
          </li>
        ))}
      </ul>
    </nav>
  );
}
