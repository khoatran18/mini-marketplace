import type {
  AnalyticsEnvelope,
  Address,
  CartOutput,
  Category,
  CategoryInput,
  ChangePasswordInput,
  ChangePasswordOutput,
  CheckoutInput,
  CheckoutOutput,
  CountOrdersOutput,
  CreateBuyerProfileInput,
  CreateBuyerProfileOutput,
  CreateProductInput,
  CreateProductOutput,
  CreateSellerProfileInput,
  CreateSellerProfileOutput,
  GetBuyerProfileOutput,
  GetProductByIdOutput,
  GetProductsOutput,
  GetSellerProfileOutput,
  LedgerEntry,
  ListAddressesOutput,
  ListCategoriesOutput,
  ListOrdersOutput,
  ListPaymentsOutput,
  LoginInput,
  LoginOutput,
  Order,
  OrderActionInput,
  OrdersScope,
  Payment,
  PaymentMethodsOutput,
  PreviewOutput,
  Product,
  RefreshTokenOutput,
  RegisterInput,
  RegisterOutput,
  SearchParams,
  SearchProductsOutput,
  StockLevel,
  SystemHealthOutput,
  UpdateBuyerProfileInput,
  UpdateBuyerProfileOutput,
  UpdateSellerProfileInput,
  UpdateSellerProfileOutput
} from './types';

const DEFAULT_BASE_URL = 'http://localhost:8080';

export function getBaseUrl() {
  if (process.env.NEXT_PUBLIC_API_BASE_URL) {
    return process.env.NEXT_PUBLIC_API_BASE_URL;
  }
  return DEFAULT_BASE_URL;
}

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export interface FetchOptions extends RequestInit {
  token?: string | null;
}

export async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
  const { token, headers, ...rest } = options;
  const response = await fetch(`${getBaseUrl()}${path}`, {
    ...rest,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers
    },
    cache: 'no-store'
  });

  const contentType = response.headers.get('content-type');
  const isJson = contentType?.includes('application/json');
  const payload = isJson ? await response.json() : undefined;

  if (!response.ok) {
    const message = (payload as { error?: string; message?: string } | undefined)?.error ??
      (payload as { message?: string } | undefined)?.message ??
      response.statusText;
    throw new ApiError(message, response.status);
  }

  return payload as T;
}

export async function loginRequest(input: LoginInput): Promise<LoginOutput> {
  return apiFetch<LoginOutput>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export async function registerRequest(input: RegisterInput): Promise<RegisterOutput> {
  return apiFetch<RegisterOutput>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export async function changePasswordRequest(
  input: ChangePasswordInput,
  token?: string | null
): Promise<ChangePasswordOutput> {
  return apiFetch<ChangePasswordOutput>('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify(input),
    token
  });
}

export async function refreshTokenRequest(refreshToken: string): Promise<RefreshTokenOutput> {
  return apiFetch<RefreshTokenOutput>('/auth/refresh-token', {
    method: 'POST',
    body: JSON.stringify({ refresh_token: refreshToken })
  });
}

export async function createBuyerProfileRequest(
  input: CreateBuyerProfileInput,
  token?: string | null
): Promise<CreateBuyerProfileOutput> {
  return apiFetch<CreateBuyerProfileOutput>('/users/buyers', {
    method: 'POST',
    body: JSON.stringify(input),
    token
  });
}

export async function getBuyerProfileRequest(
  userId: number,
  token?: string | null
): Promise<GetBuyerProfileOutput> {
  return apiFetch<GetBuyerProfileOutput>(`/users/buyers/${userId}`, {
    method: 'GET',
    token
  });
}

export async function updateBuyerProfileRequest(
  userId: number,
  input: UpdateBuyerProfileInput,
  token?: string | null
): Promise<UpdateBuyerProfileOutput> {
  return apiFetch<UpdateBuyerProfileOutput>(`/users/buyers/${userId}`, {
    method: 'PUT',
    body: JSON.stringify(input),
    token
  });
}

export async function createSellerProfileRequest(
  input: CreateSellerProfileInput,
  token?: string | null
): Promise<CreateSellerProfileOutput> {
  return apiFetch<CreateSellerProfileOutput>('/users/sellers', {
    method: 'POST',
    body: JSON.stringify(input),
    token
  });
}

export async function getSellerProfileRequest(
  userId: number,
  token?: string | null
): Promise<GetSellerProfileOutput> {
  return apiFetch<GetSellerProfileOutput>(`/users/sellers/${userId}`, {
    method: 'GET',
    token
  });
}

export async function updateSellerProfileRequest(
  userId: number,
  input: UpdateSellerProfileInput,
  token?: string | null
): Promise<UpdateSellerProfileOutput> {
  return apiFetch<UpdateSellerProfileOutput>(`/users/sellers/${userId}`, {
    method: 'PUT',
    body: JSON.stringify(input),
    token
  });
}

// ---- catalog ----------------------------------------------------------------------------------------------------

export function toQuery(params: Record<string, string | number | boolean | undefined | null>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '' || value === false) continue;
    query.set(key, String(value));
  }
  const text = query.toString();
  return text ? `?${text}` : '';
}

const json = (body: unknown) => JSON.stringify(body);

/** Public: sends the token only when there is one (a malformed token is rejected by the gateway). */
export async function searchProductsRequest(params: SearchParams, token?: string | null): Promise<SearchProductsOutput> {
  return apiFetch<SearchProductsOutput>(`/search${toQuery({ ...params })}`, { method: 'GET', token });
}

export async function getCategoriesRequest(): Promise<ListCategoriesOutput> {
  return apiFetch<ListCategoriesOutput>('/categories', { method: 'GET' });
}

export async function getProductByIdRequest(
  id: number,
  token?: string | null
): Promise<GetProductByIdOutput> {
  return apiFetch<GetProductByIdOutput>(`/products/${id}`, {
    method: 'GET',
    token
  });
}

export async function getProductStockRequest(id: number): Promise<{ product_id: number; level: StockLevel }> {
  return apiFetch<{ product_id: number; level: StockLevel }>(`/products/${id}/stock`, { method: 'GET' });
}

// ---- seller: products & inventory --------------------------------------------------------------------------------

export async function createProductRequest(input: CreateProductInput, token?: string | null): Promise<CreateProductOutput> {
  return apiFetch<CreateProductOutput>('/products', { method: 'POST', body: json(input), token });
}

export async function updateProductRequest(id: number, product: Product, token?: string | null): Promise<{ success?: boolean; message?: string }> {
  return apiFetch(`/products/${id}`, { method: 'PUT', body: json({ product }), token });
}

export async function setProductStatusRequest(id: number, status: string, token?: string | null, admin = false) {
  return apiFetch<{ success?: boolean; message?: string }>(`${admin ? '/admin' : ''}/products/${id}/status`, {
    method: 'PATCH',
    body: json({ status }),
    token
  });
}

export async function getSellerProductsRequest(params: SearchParams, token: string): Promise<SearchProductsOutput> {
  return apiFetch<SearchProductsOutput>(`/seller/products${toQuery({ ...params })}`, { method: 'GET', token });
}

export async function getLowStockRequest(token: string, threshold?: number): Promise<GetProductsOutput> {
  return apiFetch<GetProductsOutput>(`/seller/inventory/low-stock${toQuery({ threshold })}`, { method: 'GET', token });
}

export async function adjustInventoryRequest(id: number, delta: number, reason: string, token: string) {
  return apiFetch<{ success?: boolean; message?: string; inventory?: number }>(`/seller/products/${id}/inventory/adjust`, {
    method: 'POST',
    body: json({ delta, reason }),
    token
  });
}

export async function getInventoryLedgerRequest(id: number, token: string, page = 1, pageSize = 50) {
  return apiFetch<{ entries?: LedgerEntry[] }>(`/seller/products/${id}/inventory/ledger${toQuery({ page, page_size: pageSize })}`, {
    method: 'GET',
    token
  });
}

// ---- admin: categories --------------------------------------------------------------------------------------------

export async function getAdminCategoriesRequest(token: string): Promise<ListCategoriesOutput> {
  return apiFetch<ListCategoriesOutput>('/admin/categories', { method: 'GET', token });
}

export async function saveCategoryRequest(input: CategoryInput, token: string, id?: number): Promise<{ category?: Category }> {
  return apiFetch(id ? `/admin/categories/${id}` : '/admin/categories', { method: id ? 'PUT' : 'POST', body: json(input), token });
}

// ---- cart & checkout -----------------------------------------------------------------------------------------------

export async function getCartRequest(token: string): Promise<CartOutput> {
  return apiFetch<CartOutput>('/cart', { method: 'GET', token });
}

export async function setCartItemRequest(productId: number, quantity: number, token: string): Promise<CartOutput> {
  return apiFetch<CartOutput>(`/cart/items/${productId}`, { method: 'PUT', body: json({ quantity }), token });
}

export async function removeCartItemRequest(productId: number, token: string): Promise<CartOutput> {
  return apiFetch<CartOutput>(`/cart/items/${productId}`, { method: 'DELETE', token });
}

export async function clearCartRequest(token: string): Promise<CartOutput> {
  return apiFetch<CartOutput>('/cart', { method: 'DELETE', token });
}

export async function mergeCartRequest(items: { product_id: number; quantity: number }[], token: string): Promise<CartOutput> {
  return apiFetch<CartOutput>('/cart/merge', { method: 'POST', body: json({ items }), token });
}

export async function previewCheckoutRequest(token: string): Promise<PreviewOutput> {
  return apiFetch<PreviewOutput>('/checkout/preview', { method: 'POST', body: json({ from_cart: true }), token });
}

// The Idempotency-Key makes a retry of the same checkout safe (no second order).
export async function checkoutRequest(input: CheckoutInput, idempotencyKey: string, token: string): Promise<CheckoutOutput> {
  return apiFetch<CheckoutOutput>('/orders', {
    method: 'POST',
    body: json(input),
    token,
    headers: { 'Idempotency-Key': idempotencyKey }
  });
}

export async function getCheckoutRequest(checkoutId: string, token: string): Promise<CheckoutOutput> {
  return apiFetch<CheckoutOutput>(`/checkouts/${encodeURIComponent(checkoutId)}`, { method: 'GET', token });
}

export async function getAddressesRequest(token: string): Promise<ListAddressesOutput> {
  return apiFetch<ListAddressesOutput>('/users/me/addresses', { method: 'GET', token });
}

export async function saveAddressRequest(address: Address, token: string): Promise<{ address?: Address }> {
  const { id, ...body } = address;
  return apiFetch(id ? `/users/me/addresses/${id}` : '/users/me/addresses', { method: id ? 'PUT' : 'POST', body: json(body), token });
}

export async function deleteAddressRequest(id: number, token: string) {
  return apiFetch<{ success?: boolean }>(`/users/me/addresses/${id}`, { method: 'DELETE', token });
}

// ---- orders (buyer / seller / admin views share one shape) -------------------------------------------------------

const ordersBase: Record<OrdersScope, string> = { buyer: '/orders', seller: '/seller/orders', admin: '/admin/orders' };

export async function listOrdersRequest(scope: OrdersScope, params: { status?: string; page?: number; page_size?: number }, token: string): Promise<ListOrdersOutput> {
  return apiFetch<ListOrdersOutput>(`${ordersBase[scope]}${toQuery(params)}`, { method: 'GET', token });
}

// buyer and seller only (the admin route has no summary)
export async function countOrdersRequest(scope: 'buyer' | 'seller', token: string): Promise<CountOrdersOutput> {
  return apiFetch<CountOrdersOutput>(`${ordersBase[scope]}/summary`, { method: 'GET', token });
}

export async function getOrderRequest(scope: OrdersScope, id: number, token: string): Promise<Order> {
  return apiFetch<Order>(`${ordersBase[scope]}/${id}`, { method: 'GET', token });
}

export type OrderAction = 'cancel' | 'confirm-received' | 'return' | 'ship' | 'deliver' | 'return/approve' | 'return/reject';

export async function orderActionRequest(scope: OrdersScope, id: number, action: OrderAction, body: OrderActionInput, token: string): Promise<Order> {
  return apiFetch<Order>(`${ordersBase[scope]}/${id}/${action}`, { method: 'POST', body: json(body), token });
}

// ---- payments (SIMULATED) -------------------------------------------------------------------------------------------

export async function getPaymentMethodsRequest(): Promise<PaymentMethodsOutput> {
  return apiFetch<PaymentMethodsOutput>('/payments/methods', { method: 'GET' });
}

export async function getPaymentRequest(id: number, token: string): Promise<Payment> {
  return apiFetch<Payment>(`/payments/${id}`, { method: 'GET', token });
}

export interface ConfirmPaymentInput {
  card_number?: string;
  card_exp?: string;
  card_cvc?: string;
  otp?: string;
  approve?: boolean;
}

export async function confirmPaymentRequest(id: number, input: ConfirmPaymentInput, token: string): Promise<Payment> {
  return apiFetch<Payment>(`/payments/${id}/confirm`, { method: 'POST', body: json(input), token });
}

export async function cancelPaymentRequest(id: number, token: string): Promise<Payment> {
  return apiFetch<Payment>(`/payments/${id}/cancel`, { method: 'POST', token });
}

export async function listAdminPaymentsRequest(params: { status?: string; method?: string; page?: number; page_size?: number }, token: string): Promise<ListPaymentsOutput> {
  return apiFetch<ListPaymentsOutput>(`/admin/payments${toQuery(params)}`, { method: 'GET', token });
}

export async function getAdminPaymentRequest(id: number, token: string): Promise<Payment> {
  return apiFetch<Payment>(`/admin/payments/${id}`, { method: 'GET', token });
}

export async function refundPaymentRequest(id: number, amountMinor: number, reason: string, token: string): Promise<Payment> {
  return apiFetch<Payment>(`/admin/payments/${id}/refund`, { method: 'POST', body: json({ amount_minor: amountMinor, reason }), token });
}

// dev only: payment-service refuses it unless ENV=dev
export async function forcePaymentStatusRequest(id: number, status: string, token: string): Promise<Payment> {
  return apiFetch<Payment>(`/admin/dev/payments/${id}/force`, { method: 'POST', body: json({ status }), token });
}

// ---- analytics (endpoints are built in parallel: callers must tolerate errors and unknown shapes) --------------

export async function getAnalyticsRequest(
  scope: 'seller' | 'admin',
  report: string,
  params: Record<string, string | number | undefined>,
  token: string
): Promise<AnalyticsEnvelope> {
  return apiFetch<AnalyticsEnvelope>(`/${scope}/analytics/${report}${toQuery(params)}`, { method: 'GET', token });
}

export async function getSystemHealthRequest(token: string): Promise<SystemHealthOutput> {
  return apiFetch<SystemHealthOutput>('/admin/system/health', { method: 'GET', token });
}
