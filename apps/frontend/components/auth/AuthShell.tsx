import Link from 'next/link';
import { ShoppingBag } from 'lucide-react';
import type { ReactNode } from 'react';

/** Frame for login / register: a slim brand strip on the left (hidden on phones), the form on the right. */
export function AuthShell({ title, subtitle, children, footer }: { title: string; subtitle: string; children: ReactNode; footer: ReactNode }) {
  return (
    <div className="mx-auto grid w-full max-w-3xl overflow-hidden rounded-2xl border border-line bg-surface shadow-sm md:grid-cols-[2fr_3fr]">
      <aside className="relative hidden flex-col justify-between overflow-hidden bg-gradient-to-br from-teal-700 to-emerald-800 p-7 text-white md:flex">
        <div aria-hidden="true" className="pointer-events-none absolute -bottom-24 -right-16 h-60 w-60 rounded-full bg-white/10 blur-2xl" />
        <Link href="/" className="relative text-lg font-bold tracking-tight">
          Mini Marketplace
        </Link>
        <div aria-hidden="true" className="relative grid place-items-center">
          <span className="grid h-28 w-28 place-items-center rounded-3xl bg-white/15 ring-1 ring-white/25 backdrop-blur-sm">
            <ShoppingBag className="h-14 w-14 text-white" strokeWidth={1.4} />
          </span>
        </div>
        <p className="relative text-sm text-white/80">Thanh toán trên trang này là mô phỏng, không dùng tiền hay thẻ thật.</p>
      </aside>

      <section className="p-6 sm:p-8">
        <div className="mx-auto grid w-full max-w-sm gap-5">
          <header className="grid gap-1">
            <h1 className="text-2xl font-bold tracking-tight text-text">{title}</h1>
            <p className="text-sm text-muted">{subtitle}</p>
          </header>
          {children}
          <p className="text-center text-sm text-muted">{footer}</p>
        </div>
      </section>
    </div>
  );
}
