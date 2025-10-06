export type Role = 'buyer' | 'seller_admin' | 'seller_employee';

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

export interface Product {
  id?: number;
  name: string;
  price: number;
  inventory: number;
  seller_id: number;
  attributes?: Record<string, unknown>;
}

export interface CartEntry {
  product: Product;
  quantity: number;
}

export interface GetProductsOutput {
  success?: boolean;
  message?: string;
  products?: Product[];
}

export interface OrderItem {
  id?: number;
  order_id?: number;
  product_id: number;
  name?: string;
  price: number;
  quantity: number;
}

export interface Order {
  id?: number;
  buyer_id: number;
  status?: string;
  total_price?: number;
  order_items: OrderItem[];
}

export interface CreateProductInput {
  name: string;
  price: number;
  inventory: number;
  seller_id: number;
  attributes?: Record<string, unknown>;
}

export interface CreateProductOutput {
  success?: boolean;
  message?: string;
}

export interface CreateOrderInput {
  order: Order;
}

export interface CreateOrderOutput {
  success?: boolean;
  message?: string;
}

export interface GetProductByIdOutput {
  success?: boolean;
  message?: string;
  product?: Product;
}
