// Cấu hình e2e đọc từ biến môi trường (có mặc định cho dev cục bộ).
const env = process.env;
const bool = (v, d = false) => (v === undefined || v === '' ? d : ['1', 'true', 'yes'].includes(String(v).toLowerCase()));

export const config = {
  api: (env.API_URL ?? 'http://localhost:8080').replace(/\/$/, ''),
  // Admin: tài khoản được tạo bởi ADMIN_BOOTSTRAP_* lúc khởi động auth-service. Không có => bỏ qua các bước cần admin.
  adminUser: env.ADMIN_USERNAME ?? env.ADMIN_BOOTSTRAP_USERNAME ?? '',
  adminPass: env.ADMIN_PASSWORD ?? env.ADMIN_BOOTSTRAP_PASSWORD ?? '',
  password: 'E2ePass_1', // 8-16 ký tự, [A-Za-z0-9_]
  // Webhook payment-service nằm ở cổng nội bộ :8081 (không qua gateway); chỉ chạy khi cấu hình đủ.
  paymentWebhookUrl: env.PAYMENT_WEBHOOK_URL ?? '', // vd http://payment-service:8081/internal/payments/webhook
  paymentWebhookSecret: env.PAYMENT_WEBHOOK_SECRET ?? '',
  // Gateway ở ENV=dev mới có /admin/dev/payments/:id/force
  devMode: bool(env.E2E_DEV_MODE, false),
  // Chờ hết hạn giữ hàng: cần ORDER_PAYMENT_TTL_MIN nhỏ (vd 1). Giá trị = số phút TTL đang cấu hình; 0 = bỏ qua.
  expiryTtlMin: Number(env.E2E_EXPIRY_TTL_MIN ?? 0),
  // Bật bước kiểm tra rate limit (cần AUTH_RATE_LIMIT_PER_MINUTE thấp, mặc định 20)
  rateLimitTest: bool(env.RATE_LIMIT_TEST, false),
  // Thời gian chờ ghi nhận analytics (flush ClickHouse)
  analyticsWaitMs: Number(env.E2E_ANALYTICS_WAIT_MS ?? 60000),
  // Webhook ví/chuyển khoản mô phỏng trễ 3s/8s mặc định; nới ra nếu cấu hình lớn hơn
  asyncPayWaitMs: Number(env.E2E_ASYNC_PAY_WAIT_MS ?? 45000),
  runId: Date.now().toString(36).slice(-6), // hậu tố username: username ≤16 ký tự
};
