import Link from 'next/link';
import { HomeSections } from './HomeSections';
import { SearchBar } from '../components/shop/SearchBar';

export default function HomePage() {
  return (
    <div className="grid gap-10">
      <section className="card grid gap-6 border-0 bg-gradient-to-tr from-indigo-600 to-violet-600 text-white shadow-none">
        <div className="grid gap-3">
          <h1 className="text-4xl font-extrabold">Mini Marketplace</h1>
          <p className="text-base leading-relaxed md:text-lg">
            Tìm sản phẩm theo từ khoá, danh mục và giá. Đặt hàng, thanh toán thử (mô phỏng) và theo dõi đơn ngay trên một giao diện.
          </p>
        </div>
        <SearchBar className="max-w-xl [&_input]:border-white/30" />
        <div className="flex flex-wrap gap-3">
          <Link href="/products" className="rounded-xl bg-white px-5 py-2.5 font-semibold text-indigo-700 shadow-sm transition hover:bg-indigo-50">
            Khám phá sản phẩm
          </Link>
          <Link href="/register" className="rounded-xl border border-white/60 px-5 py-2.5 font-semibold text-white transition hover:bg-white/10">
            Tạo tài khoản
          </Link>
        </div>
      </section>
      <HomeSections />
    </div>
  );
}
