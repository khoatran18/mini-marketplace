export type Role = 'buyer' | 'seller_admin' | 'seller_employee' | 'admin';

// Roles that can sign up from the UI; admin accounts are created by the operator (ADMIN_BOOTSTRAP_*).
export type RegisterableRole = Exclude<Role, 'admin'>;

export interface LoginInput {
  username: string;
  password: string;
  role: Role;
}

export interface LoginOutput {
  access_token?: string;
  refresh_token?: string;
  message?: string;
  success?: boolean;
  role?: Role;
  username?: string;
  user_id?: number | string;
}

export interface RefreshTokenInput {
  refresh_token: string;
}

export interface RefreshTokenOutput {
  access_token?: string;
  refresh_token?: string;
  message?: string;
  success?: boolean;
}

export interface RegisterInput {
  username: string;
  password: string;
  role: Role;
}

export interface RegisterOutput {
  success?: boolean;
  message?: string;
}

export interface ChangePasswordInput {
  username: string;
  old_password: string;
  new_password: string;
  role: Role;
}

export interface ChangePasswordOutput {
  success?: boolean;
  message?: string;
}

export interface BuyerProfile {
  user_id: number;
  name: string;
  gender: string;
  date_of_birth: string;
  phone: string;
  address: string;
}

export interface BuyerProfilePayload {
  buyer: BuyerProfile;
}

export interface BuyerProfileResponse {
  success?: boolean;
  message?: string;
  buyer?: BuyerProfile;
}

export type CreateBuyerProfileInput = BuyerProfilePayload;
export type UpdateBuyerProfileInput = BuyerProfilePayload;
export type CreateBuyerProfileOutput = BuyerProfileResponse;
export type UpdateBuyerProfileOutput = BuyerProfileResponse;
export type GetBuyerProfileOutput = BuyerProfileResponse;

export interface SellerProfile {
  id?: number;
  name: string;
  bank_account: string;
  tax_code: string;
  description: string;
  date_of_birth: string;
  phone: string;
  address: string;
}

export interface CreateSellerProfileInput {
  seller: SellerProfile;
  user_id: number;
}

export interface UpdateSellerProfileInput {
  seller: SellerProfile;
  user_id?: number;
}

export interface SellerProfileResponse {
  success?: boolean;
  message?: string;
  seller?: SellerProfile;
}

export type CreateSellerProfileOutput = SellerProfileResponse;
export type UpdateSellerProfileOutput = SellerProfileResponse;
export type GetSellerProfileOutput = SellerProfileResponse;

export type ProductStatus = 'draft' | 'active' | 'hidden' | 'banned';
export type StockLevel = 'none' | 'low' | 'ok';

// Product as served by the gateway (proto field names). `inventory` is capped at 20 for people who do not own it.
export interface Product {
  id?: number;
  name: string;
  price: number;
  inventory: number;
  seller_id: number;
  attributes?: Record<string, unknown>;
  description?: string;
  category_id?: number;
  category_name?: string;
  brand?: string;
  tags?: string[];
  image_urls?: string[];
  status?: ProductStatus | string;
  sku?: string;
  low_stock_threshold?: number;
  weight_g?: number;
  reserved?: number;
  sold?: number;
  version?: number;
  created_at?: string;
  updated_at?: string;
  stock_level?: StockLevel | string;
}

export interface GetProductsOutput {
  success?: boolean;
  message?: string;
  products?: Product[];
}

export interface GetProductByIdOutput {
  success?: boolean;
  message?: string;
  product?: Product;
}

export interface SearchProductsOutput extends GetProductsOutput {
  total?: number;
}

export interface SearchParams {
  q?: string;
  category_id?: number;
  price_min?: number;
  price_max?: number;
  brand?: string;
  in_stock?: boolean;
  sort?: string;
  page?: number;
  page_size?: number;
}

export interface Category {
  id: number;
  parent_id: number;
  name: string;
  slug: string;
  sort: number;
  active: boolean;
  product_count: number;
}

export interface ListCategoriesOutput {
  categories?: Category[];
}

export interface CategoryInput {
  parent_id?: number;
  name: string;
  slug: string;
  sort?: number;
  active?: boolean;
}

export interface CreateProductInput {
  name: string;
  price: number;
  inventory: number;
  seller_id?: number; // ignored by the gateway: the store comes from the token
  attributes?: Record<string, unknown>;
  description?: string;
  category_id?: number;
  brand?: string;
  tags?: string[];
  image_urls?: string[];
  status?: string;
  sku?: string;
  low_stock_threshold?: number;
  weight_g?: number;
}

export interface CreateProductOutput {
  success?: boolean;
  message?: string;
  id?: number;
}

export interface LedgerEntry {
  id: number;
  product_id: number;
  delta: number;
  reason: string;
  ref_type: string;
  ref_id: string;
  balance_after: number;
  at: string;
}

// ---- cart & checkout -------------------------------------------------------------------------------------------

export interface CartLine {
  product_id: number;
  quantity: number;
  name: string;
  image_url?: string;
  store_id?: number;
  unit_price: number;
  line_total: number;
  stock_level?: StockLevel | string;
  available?: boolean;
  issue?: string;
}

export interface CartOutput {
  lines: CartLine[];
  subtotal: number;
  item_count: number;
}

export interface PreviewLine extends Omit<CartLine, 'store_id'> {}

export interface PreviewGroup {
  store_id: number;
  lines: PreviewLine[];
  subtotal: number;
  shipping_fee: number;
  total: number;
}

export interface PreviewOutput {
  groups: PreviewGroup[];
  subtotal: number;
  shipping_fee: number;
  grand_total: number;
  can_order: boolean;
}

export interface Address {
  id?: number;
  label: string;
  receiver_name: string;
  phone: string;
  line1: string;
  ward: string;
  district: string;
  city: string;
  is_default: boolean;
}

export interface ListAddressesOutput {
  addresses?: Address[];
}

export interface PaymentMethodInfo {
  code: string;
  label: string;
  online: boolean;
  simulated: boolean;
  test_cards?: { number: string; result: string }[];
}

export interface PaymentMethodsOutput {
  methods: PaymentMethodInfo[];
}

export interface CheckoutInput {
  from_cart: boolean;
  items?: { product_id: number; quantity: number }[];
  address_id: number;
  payment_method: string;
  note?: string;
}

// ---- orders ----------------------------------------------------------------------------------------------------

export type OrderStatus =
  | 'PENDING'
  | 'AWAITING_PAYMENT'
  | 'CONFIRMED'
  | 'PAID'
  | 'SHIPPED'
  | 'DELIVERED'
  | 'REFUND_REQUESTED'
  | 'REFUNDED'
  | 'CANCELED'
  | 'EXPIRED'
  | 'FAILED';

export interface OrderItem {
  id?: number;
  order_id?: number;
  product_id: number;
  name: string;
  sku?: string;
  image_url?: string;
  store_id?: number;
  quantity: number;
  unit_price: number;
  line_total: number;
  status?: string;
}

export interface StatusHistory {
  from_status: string;
  to_status: string;
  actor_type: string;
  actor_id: number;
  reason: string;
  at: string;
}

export interface ShippingAddress {
  label?: string;
  receiver_name?: string;
  phone?: string;
  line1?: string;
  ward?: string;
  district?: string;
  city?: string;
}

export interface Order {
  id: number;
  checkout_id: string;
  buyer_id: number;
  store_id: number;
  status: OrderStatus | string;
  payment_method: string;
  payment_status: string;
  subtotal: number;
  shipping_fee: number;
  total_price: number;
  shipping_address?: ShippingAddress | null;
  note?: string;
  expires_at?: string;
  created_at: string;
  updated_at?: string;
  paid_at?: string;
  shipped_at?: string;
  delivered_at?: string;
  canceled_at?: string;
  cancel_reason?: string;
  canceled_by?: string;
  carrier?: string;
  tracking_code?: string;
  return_reason?: string;
  items: OrderItem[];
  history?: StatusHistory[];
}

export interface ListOrdersOutput {
  orders: Order[];
  total: number;
}

export interface CountOrdersOutput {
  by_status: Record<string, number>;
}

export interface Payment {
  id: number;
  checkout_id: string;
  buyer_id?: number;
  method: string;
  status: string;
  amount_minor: number;
  amount: number;
  currency: string;
  provider?: string;
  provider_ref?: string;
  failure_code?: string;
  card_last4?: string;
  expires_at?: string;
  paid_at?: string;
  created_at?: string;
  refunded_minor: number;
  refunded: number;
  next_action: string;
  order_ids?: number[];
  pay_url: string;
  simulated: boolean;
  attempts?: { id: number; scenario: string; status: string; failure_code: string; at: string }[];
  refunds?: { id: number; payment_id: number; order_id: number; amount_minor: number; reason: string; status: string; at: string }[];
}

export interface CheckoutOutput {
  checkout_id: string;
  orders: Order[];
  grand_total: number;
  payment_method: string;
  replayed: boolean;
  payment?: Payment;
}

export interface OrderActionInput {
  reason?: string;
  carrier?: string;
  tracking_code?: string;
}

export interface ListPaymentsOutput {
  payments: Payment[];
  total: number;
}

// ---- analytics (response envelope: { as_of, tz, source, data }) ------------------------------------------------

export interface AnalyticsEnvelope<T = unknown> {
  as_of?: string;
  tz?: string;
  source?: string;
  data?: T;
  [key: string]: unknown;
}

export type OrdersScope = 'buyer' | 'seller' | 'admin';

// GET /admin/system/health
export type ServiceStatus = 'ready' | 'degraded' | 'not_ready' | 'unreachable';

export interface ServiceHealth {
  name: string;
  status: ServiceStatus;
  latency_ms: number;
  version?: string;
  uptime_s?: number;
  checks?: Record<string, { status: 'ok' | 'fail'; latency_ms?: number; critical?: boolean; error?: string }>;
}

export interface SystemHealthOutput {
  as_of: string;
  status: ServiceStatus;
  services: ServiceHealth[];
}
