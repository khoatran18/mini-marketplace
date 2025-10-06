import type {
  CreateOrderInput,
  CreateOrderOutput,
  CreateProductInput,
  CreateProductOutput,
  GetProductByIdOutput,
  GetProductsOutput,
  LoginInput,
  LoginOutput,
  RefreshTokenOutput,
  RegisterInput,
  RegisterOutput
} from './types';

const DEFAULT_BASE_URL = 'http://localhost:8080';

function getBaseUrl() {
  if (process.env.NEXT_PUBLIC_API_BASE_URL) {
    return process.env.NEXT_PUBLIC_API_BASE_URL;
  }
  return DEFAULT_BASE_URL;
}

interface FetchOptions extends RequestInit {
  token?: string | null;
}

async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
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
    throw new Error(message);
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

export async function refreshTokenRequest(refreshToken: string): Promise<RefreshTokenOutput> {
  return apiFetch<RefreshTokenOutput>('/auth/refresh-token', {
    method: 'POST',
    body: JSON.stringify({ refresh_token: refreshToken })
  });
}

export async function getProductsRequest(
  page: number,
  pageSize: number,
  token?: string | null
): Promise<GetProductsOutput> {
  const params = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize)
  });
  return apiFetch<GetProductsOutput>(`/products?${params.toString()}`, {
    method: 'GET',
    token
  });
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

export async function createProductRequest(
  input: CreateProductInput,
  token?: string | null
): Promise<CreateProductOutput> {
  return apiFetch<CreateProductOutput>('/products', {
    method: 'POST',
    body: JSON.stringify(input),
    token
  });
}

export async function createOrderRequest(
  input: CreateOrderInput,
  token?: string | null
): Promise<CreateOrderOutput> {
  return apiFetch<CreateOrderOutput>('/orders', {
    method: 'POST',
    body: JSON.stringify(input),
    token
  });
}
