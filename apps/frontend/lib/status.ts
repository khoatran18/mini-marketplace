// The single mapping from order / payment / stock / product statuses to a Vietnamese label and a colour tone.
// Tones map to the design tokens in globals.css, so every badge works in light and dark mode.

export type Tone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'brand';

export interface StatusMeta {
  label: string;
  tone: Tone;
}

export const toneClass: Record<Tone, string> = {
  neutral: 'bg-surface2 text-muted',
  info: 'bg-info-soft text-info',
  success: 'bg-success-soft text-success',
  warning: 'bg-warning-soft text-warning',
  danger: 'bg-danger-soft text-danger',
  brand: 'bg-brand-soft text-brand'
};

export const orderStatuses: Record<string, StatusMeta> = {
  PENDING: { label: 'Đang giữ hàng', tone: 'neutral' },
  AWAITING_PAYMENT: { label: 'Chờ thanh toán', tone: 'warning' },
  SUCCESS: { label: 'Chờ thanh toán', tone: 'warning' }, // legacy value of AWAITING_PAYMENT
  CONFIRMED: { label: 'Đã xác nhận (COD)', tone: 'info' },
  PAID: { label: 'Đã thanh toán', tone: 'success' },
  SHIPPED: { label: 'Đang giao', tone: 'brand' },
  DELIVERED: { label: 'Đã giao', tone: 'success' },
  REFUND_REQUESTED: { label: 'Yêu cầu hoàn trả', tone: 'warning' },
  REFUNDED: { label: 'Đã hoàn tiền', tone: 'neutral' },
  CANCELED: { label: 'Đã huỷ', tone: 'danger' },
  EXPIRED: { label: 'Hết hạn thanh toán', tone: 'danger' },
  FAILED: { label: 'Thất bại', tone: 'danger' }
};

export const paymentStatuses: Record<string, StatusMeta> = {
  REQUIRES_ACTION: { label: 'Chờ thanh toán', tone: 'warning' },
  PROCESSING: { label: 'Đang xử lý', tone: 'info' },
  SUCCEEDED: { label: 'Thành công', tone: 'success' },
  FAILED: { label: 'Thất bại', tone: 'danger' },
  CANCELED: { label: 'Đã huỷ', tone: 'neutral' },
  EXPIRED: { label: 'Hết hạn', tone: 'danger' },
  PARTIALLY_REFUNDED: { label: 'Hoàn một phần', tone: 'warning' },
  REFUNDED: { label: 'Đã hoàn tiền', tone: 'neutral' },
  // order.payment_status
  UNPAID: { label: 'Chưa thanh toán', tone: 'warning' },
  PAID: { label: 'Đã thanh toán', tone: 'success' }
};

export const stockLevels: Record<string, StatusMeta> = {
  ok: { label: 'Còn hàng', tone: 'success' },
  low: { label: 'Còn ít', tone: 'warning' },
  none: { label: 'Hết hàng', tone: 'danger' }
};

export const productStatuses: Record<string, StatusMeta> = {
  draft: { label: 'Nháp', tone: 'neutral' },
  active: { label: 'Đang bán', tone: 'success' },
  hidden: { label: 'Đang ẩn', tone: 'warning' },
  banned: { label: 'Bị khoá', tone: 'danger' }
};

export const paymentMethods: Record<string, string> = {
  COD: 'Thanh toán khi nhận hàng (COD)',
  MOCK_CARD: 'Thẻ (MÔ PHỎNG)',
  MOCK_WALLET: 'Ví điện tử (MÔ PHỎNG)',
  MOCK_BANK_TRANSFER: 'Chuyển khoản (MÔ PHỎNG)'
};

export const failureCodes: Record<string, string> = {
  card_declined: 'Thẻ bị từ chối.',
  insufficient_funds: 'Thẻ không đủ số dư.',
  provider_error: 'Lỗi tạm thời từ nhà cung cấp, vui lòng thử lại.',
  otp_incorrect: 'Mã OTP không đúng.',
  user_cancelled: 'Bạn đã từ chối thanh toán.',
  do_not_honor: 'Thẻ không được chấp nhận.'
};

function lookup(table: Record<string, StatusMeta>, status: string | undefined | null): StatusMeta {
  if (!status) return { label: '–', tone: 'neutral' };
  return table[status] ?? table[status.toUpperCase()] ?? { label: status, tone: 'neutral' };
}

export const orderStatusMeta = (status?: string | null) => lookup(orderStatuses, status);
export const paymentStatusMeta = (status?: string | null) => lookup(paymentStatuses, status);
export const stockLevelMeta = (level?: string | null) => lookup(stockLevels, level);
export const productStatusMeta = (status?: string | null) => lookup(productStatuses, status);
export const paymentMethodLabel = (code?: string | null) => (code ? paymentMethods[code] ?? code : '–');
export const failureCodeLabel = (code?: string | null) => (code ? failureCodes[code] ?? code : '');

/** Order lifecycle steps of the happy path, used by the timeline (04-orders-payments.md §1). */
export const orderStepsOnline = ['AWAITING_PAYMENT', 'PAID', 'SHIPPED', 'DELIVERED'] as const;
export const orderStepsCod = ['CONFIRMED', 'SHIPPED', 'DELIVERED'] as const;
export const terminalStatuses = ['FAILED', 'EXPIRED', 'CANCELED', 'REFUNDED', 'DELIVERED'];

// What each role may do with an order in a given status (the gateway/order-service enforce this for real).
export const buyerCanCancel = (status?: string) => ['AWAITING_PAYMENT', 'SUCCESS', 'PAID', 'CONFIRMED'].includes(status ?? '');
export const buyerCanConfirmReceived = (status?: string) => status === 'SHIPPED';
export const buyerCanReturn = (status?: string) => status === 'DELIVERED';
export const sellerCanShip = (status?: string) => ['PAID', 'CONFIRMED'].includes(status ?? '');
export const sellerCanDeliver = (status?: string) => status === 'SHIPPED';
export const sellerCanCancel = (status?: string) => ['AWAITING_PAYMENT', 'SUCCESS', 'PAID', 'CONFIRMED'].includes(status ?? '');
export const canDecideReturn = (status?: string) => status === 'REFUND_REQUESTED';
