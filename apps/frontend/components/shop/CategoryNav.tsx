import Link from 'next/link';
import type { Category } from '../../lib/types';

const bySort = (a: Category, b: Category) => a.sort - b.sort || a.name.localeCompare(b.name);

/** Flat category list (parent_id) rendered as groups: a bold parent header with an accent bar and an indented list of children (dot bullets) under a guide line. */
export function CategoryNav({ categories, activeId }: { categories: Category[]; activeId?: number }) {
  const roots = categories.filter((category) => !category.parent_id).sort(bySort);
  const children = (id: number) => categories.filter((category) => category.parent_id === id).sort(bySort);
  const link = (category: Category, nested: boolean) => {
    const active = activeId === category.id;
    return (
      <Link
        href={`/categories/${category.slug}`}
        aria-current={active ? 'page' : undefined}
        className={`flex items-center justify-between gap-2 rounded-lg transition ${
          nested ? 'px-3 py-1 text-[13px] font-normal' : 'px-2 py-1.5 text-sm font-bold uppercase tracking-wide'
        } ${active ? 'bg-brand-soft text-brand' : nested ? 'text-muted hover:bg-surface2 hover:text-text' : 'text-text hover:bg-surface2'}`}
      >
        <span className="flex items-center gap-2">
          <span aria-hidden="true" className={nested ? 'h-1 w-1 rounded-full bg-muted/60' : 'h-4 w-1 rounded-full bg-brand'} />
          {category.name}
        </span>
        <span className="text-xs font-normal normal-case tracking-normal text-muted">{category.product_count}</span>
      </Link>
    );
  };
  return (
    <nav aria-label="Danh mục">
      <ul className="grid gap-3">
        {roots.map((root) => {
          const kids = children(root.id);
          return (
            <li key={root.id} className="grid gap-0.5">
              {link(root, false)}
              {kids.length > 0 ? (
                <ul className="ml-4 grid gap-0.5 border-l-2 border-line pl-2">
                  {kids.map((child) => (
                    <li key={child.id}>{link(child, true)}</li>
                  ))}
                </ul>
              ) : null}
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
