import { HeroActions } from './HeroActions';
import { HomeSections } from './HomeSections';
import { SearchBar } from '../components/shop/SearchBar';

export default function HomePage() {
  return (
    <div className="grid gap-8">
      <section className="relative overflow-hidden rounded-2xl bg-gradient-to-br from-teal-700 to-emerald-800 px-6 py-8 text-white sm:px-10">
        <div aria-hidden="true" className="pointer-events-none absolute -right-16 -top-24 h-64 w-64 rounded-full bg-white/10 blur-3xl" />
        <div className="relative grid max-w-xl gap-4">
          <h1 className="text-3xl font-extrabold leading-tight tracking-tight sm:text-4xl">Mua sắm đơn giản</h1>
          <p className="text-white/85">Tìm sản phẩm, đặt hàng và theo dõi đơn.</p>
          <SearchBar className="[&_input]:border-transparent [&_input]:bg-white [&_input]:text-slate-900 [&_input]:placeholder:text-slate-500 [&_button]:border-transparent [&_button]:bg-slate-900 [&_button]:text-white [&_button]:hover:bg-slate-800" />
          <HeroActions />
        </div>
      </section>
      <HomeSections />
    </div>
  );
}
