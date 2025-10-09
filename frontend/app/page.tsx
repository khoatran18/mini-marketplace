import Link from 'next/link';

export default function HomePage() {
  return (
    <div className="grid gap-8">
      <section className="card grid gap-6 bg-gradient-to-tr from-indigo-500 to-violet-500 text-white">
        <div className="grid gap-3">
          <h1 className="text-4xl font-extrabold">Mini Marketplace</h1>
          <p className="text-base leading-relaxed md:text-lg">
            Trải nghiệm giao diện web hiện đại cho hệ thống marketplace của bạn. Đăng ký, quản lý sản phẩm và tạo
            đơn hàng chỉ với vài cú click.
          </p>
        </div>
        <div className="flex flex-wrap gap-3">
          <Link
            href="/register"
            className="rounded-xl bg-slate-900 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-slate-800"
          >
            Bắt đầu ngay
          </Link>
          <Link
            href="/products"
            className="rounded-xl border border-white/60 px-5 py-2.5 font-semibold text-white transition hover:bg-white/10"
          >
            Khám phá sản phẩm
          </Link>
        </div>
      </section>
      <section className="grid gap-4">
        <h2 className="text-2xl font-bold text-slate-900">Tính năng chính</h2>
        <div className="grid gap-4 md:grid-cols-3">
          <article className="card">
            <h3 className="text-lg font-semibold text-slate-900">Đăng nhập & phân quyền</h3>
            <p className="text-sm text-slate-600">
              Hỗ trợ đăng nhập với đầy đủ vai trò <strong>buyer</strong>, <strong>seller_admin</strong> và
              <strong> seller_employee</strong>. Phiên đăng nhập được lưu trữ an toàn để sử dụng cho các yêu cầu cần xác
              thực.
            </p>
          </article>
          <article className="card">
            <h3 className="text-lg font-semibold text-slate-900">Quản lý sản phẩm</h3>
            <p className="text-sm text-slate-600">
              Người bán có thể tạo mới, cập nhật và xem chi tiết sản phẩm với phần thuộc tính hiển thị đẹp mắt, dễ đọc.
            </p>
          </article>
          <article className="card">
            <h3 className="text-lg font-semibold text-slate-900">Giỏ hàng & đơn hàng</h3>
            <p className="text-sm text-slate-600">
              Thêm sản phẩm vào giỏ, chỉnh số lượng theo nhu cầu rồi hoàn tất thanh toán chỉ với vài thao tác.
            </p>
          </article>
        </div>
      </section>
    </div>
  );
}
