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
            href="/register"
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
              Hỗ trợ đăng nhập với đầy đủ vai trò <strong>buyer</strong>, <strong>seller_admin</strong>{' '}
              và <strong>seller_employee</strong> theo OpenAPI của API Gateway. Phiên đăng nhập được lưu
              trữ an toàn để sử dụng cho các yêu cầu cần xác thực.
            </p>
          </article>
          <article className="card">
            <h3 style={{ marginTop: 0 }}>Quản lý sản phẩm</h3>
            <p>
              Người bán có thể tạo mới, cập nhật và xem chi tiết sản phẩm với phần thuộc tính hiển thị đẹp
              mắt, dễ đọc.
            </p>
          </article>
          <article className="card">
            <h3 style={{ marginTop: 0 }}>Giỏ hàng & đơn hàng</h3>
            <p>
              Thêm sản phẩm vào giỏ, chỉnh số lượng theo nhu cầu rồi checkout để gọi API tạo đơn hàng chỉ
              với một cú nhấp chuột.
            </p>
          </article>
        </div>
      </section>
    </div>
  );
}
