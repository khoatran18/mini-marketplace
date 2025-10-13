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

export type OrderStatus = 'PENDING' | 'FAILED' | 'SUCCESS';

export interface Order {
  id?: number;
  buyer_id: number;
  status?: OrderStatus;
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

export interface GetOrdersByBuyerStatusOutput {
  success?: boolean;
  message?: string;
  orders?: Order[];
}

export interface GetProductByIdOutput {
  success?: boolean;
  message?: string;
  product?: Product;
}
