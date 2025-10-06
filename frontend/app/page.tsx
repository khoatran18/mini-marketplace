import Link from 'next/link';

export default function HomePage() {
  return (
    <div className="grid" style={{ gap: '2rem' }}>
      <section
        className="card"
        style={{
          display: 'grid',
          gap: '1.5rem',
          background: 'linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%)',
          color: 'white'
        }}
      >
        <div>
          <h1 style={{ margin: 0, fontSize: '2.5rem', fontWeight: 800 }}>Mini Marketplace</h1>
          <p style={{ margin: '0.5rem 0 0', fontSize: '1.1rem', lineHeight: 1.6 }}>
            Trải nghiệm giao diện web hiện đại cho hệ thống marketplace của bạn. Đăng ký, quản lý sản
            phẩm và tạo đơn hàng chỉ với vài cú click.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
          <Link
            href="/(auth)/register"
            style={{
              backgroundColor: '#1f2937',
              color: 'white',
              padding: '0.75rem 1.5rem',
              borderRadius: '0.75rem',
              fontWeight: 600
            }}
          >
            Bắt đầu ngay
          </Link>
          <Link
            href="/products"
            style={{
              padding: '0.75rem 1.5rem',
              borderRadius: '0.75rem',
              border: '1px solid rgba(255,255,255,0.5)',
              color: 'white',
              fontWeight: 600
            }}
          >
            Khám phá sản phẩm
          </Link>
        </div>
      </section>
      <section className="grid" style={{ gap: '1rem' }}>
        <h2 style={{ margin: 0 }}>Tính năng chính</h2>
        <div className="grid" style={{ gap: '1rem' }}>
          <article className="card">
            <h3 style={{ marginTop: 0 }}>Đăng nhập & phân quyền</h3>
            <p>
              Hỗ trợ đăng nhập với các role khác nhau (admin, buyer, seller) theo OpenAPI của API Gateway.
              Token được lưu trữ an toàn trên trình duyệt cho các yêu cầu bảo mật.
            </p>
          </article>
          <article className="card">
            <h3 style={{ marginTop: 0 }}>Quản lý sản phẩm</h3>
            <p>
              Tạo mới, xem danh sách và truy cập chi tiết sản phẩm với đầy đủ thông tin thuộc tính, tồn
              kho và giá bán.
            </p>
          </article>
          <article className="card">
            <h3 style={{ marginTop: 0 }}>Xử lý đơn hàng</h3>
            <p>
              Giao diện tạo đơn hàng thân thiện cho phép bạn thêm nhiều sản phẩm, tính tổng giá trị và gửi
              dữ liệu tới API.
            </p>
          </article>
        </div>
      </section>
    </div>
  );
}
