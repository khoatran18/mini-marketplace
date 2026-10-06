'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

/** Keyword search box: sends the visitor to /products?q=... (suggestions are not available on the gateway yet). */
export function SearchBar({ initial = '', onSearch, className = '' }: { initial?: string; onSearch?: (q: string) => void; className?: string }) {
  const router = useRouter();
  const [q, setQ] = useState(initial);
  return (
    <form
      role="search"
      className={`flex gap-2 ${className}`}
      onSubmit={(event) => {
        event.preventDefault();
        const text = q.trim();
        if (onSearch) onSearch(text);
        else router.push(text ? `/products?q=${encodeURIComponent(text)}` : '/products');
      }}
    >
      <input type="search" value={q} onChange={(event) => setQ(event.target.value)} placeholder="Tìm sản phẩm, thương hiệu…" aria-label="Tìm kiếm sản phẩm" />
      <button type="submit" className="btn-primary">
        Tìm
      </button>
    </form>
  );
}
