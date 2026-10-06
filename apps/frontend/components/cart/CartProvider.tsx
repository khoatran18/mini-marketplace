'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState
} from 'react';
import type { CartLine, CartOutput, Product } from '../../lib/types';
import {
  clearCartRequest,
  getCartRequest,
  mergeCartRequest,
  removeCartItemRequest,
  setCartItemRequest
} from '../../lib/api';
import { track } from '../../lib/tracking';
import { useAuth } from '../auth/AuthProvider';

// The cart of a signed-in buyer lives on the server (GET /cart ...). Visitors who are not signed in keep a small
// guest cart in localStorage that is merged into the server cart (POST /cart/merge) as soon as they sign in.

interface CartContextValue {
  lines: CartLine[];
  subtotal: number;
  totalQuantity: number;
  loading: boolean;
  error: string | null;
  /** true while the cart only exists in this browser (not signed in) */
  isGuest: boolean;
  addItem: (product: Product, quantity: number) => Promise<void>;
  setQuantity: (productId: number, quantity: number) => Promise<void>;
  removeItem: (productId: number) => Promise<void>;
  /** empties the server cart (or the guest cart) */
  emptyCart: () => Promise<void>;
  /** forgets the in-memory cart, e.g. on logout */
  clearCart: () => void;
  refresh: () => Promise<void>;
}

const CartContext = createContext<CartContextValue | undefined>(undefined);

const STORAGE_KEY = 'mini-marketplace-cart';
const MAX_QTY = 99;

interface GuestLine {
  product_id: number;
  quantity: number;
  name: string;
  image_url?: string;
  unit_price: number;
  store_id?: number;
  stock_level?: string;
}

function loadGuest(): GuestLine[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    const lines: GuestLine[] = [];
    for (const entry of parsed as Record<string, unknown>[]) {
      // older versions stored { product, quantity }
      const legacy = entry.product as Product | undefined;
      const productId = Number(entry.product_id ?? legacy?.id);
      const quantity = Number(entry.quantity);
      if (!Number.isFinite(productId) || productId <= 0 || !Number.isFinite(quantity) || quantity <= 0) continue;
      lines.push({
        product_id: productId,
        quantity: Math.min(MAX_QTY, quantity),
        name: String(entry.name ?? legacy?.name ?? `Sản phẩm #${productId}`),
        image_url: (entry.image_url as string | undefined) ?? legacy?.image_urls?.[0],
        unit_price: Number(entry.unit_price ?? legacy?.price ?? 0),
        store_id: Number(entry.store_id ?? legacy?.seller_id ?? 0) || undefined,
        stock_level: (entry.stock_level as string | undefined) ?? legacy?.stock_level
      });
    }
    return lines;
  } catch {
    return [];
  }
}

function saveGuest(lines: GuestLine[]) {
  if (typeof window === 'undefined') return;
  try {
    if (lines.length === 0) {
      window.localStorage.removeItem(STORAGE_KEY);
    } else {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(lines));
    }
  } catch {
    // storage blocked: the cart just does not survive a reload
  }
}

function guestToCart(lines: GuestLine[]): CartOutput {
  const mapped: CartLine[] = lines.map((line) => ({
    ...line,
    line_total: line.unit_price * line.quantity,
    available: true
  }));
  return {
    lines: mapped,
    subtotal: mapped.reduce((total, line) => total + line.line_total, 0),
    item_count: mapped.reduce((total, line) => total + line.quantity, 0)
  };
}

const emptyCart: CartOutput = { lines: [], subtotal: 0, item_count: 0 };

export function CartProvider({ children }: { children: React.ReactNode }) {
  const { ready, accessToken, role, getValidAccessToken } = useAuth();
  const [cart, setCart] = useState<CartOutput>(emptyCart);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isBuyer = role === 'buyer' && Boolean(accessToken);
  const isGuest = !accessToken;

  const refresh = useCallback(async () => {
    if (!ready) return;
    if (!accessToken) {
      setCart(guestToCart(loadGuest()));
      setError(null);
      return;
    }
    if (role !== 'buyer') {
      setCart(emptyCart);
      return;
    }
    setLoading(true);
    try {
      const token = await getValidAccessToken();
      if (!token) return;
      const guest = loadGuest();
      let next: CartOutput;
      if (guest.length > 0) {
        next = await mergeCartRequest(
          guest.map((line) => ({ product_id: line.product_id, quantity: line.quantity })),
          token
        );
        saveGuest([]);
      } else {
        next = await getCartRequest(token);
      }
      setCart({ ...next, lines: next.lines ?? [] });
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [ready, accessToken, role, getValidAccessToken]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const withToken = useCallback(
    async (fn: (token: string) => Promise<CartOutput>) => {
      const token = await getValidAccessToken();
      if (!token) throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      const next = await fn(token);
      setCart({ ...next, lines: next.lines ?? [] });
    },
    [getValidAccessToken]
  );

  const quantityOf = useCallback((productId: number) => cart.lines.find((line) => line.product_id === productId)?.quantity ?? 0, [cart.lines]);

  const setQuantity = useCallback(
    async (productId: number, quantity: number) => {
      const qty = Math.max(0, Math.min(MAX_QTY, Math.floor(quantity)));
      const before = quantityOf(productId);
      if (isBuyer) {
        await withToken((token) => (qty === 0 ? removeCartItemRequest(productId, token) : setCartItemRequest(productId, qty, token)));
      } else {
        const guest = loadGuest().filter((line) => line.product_id !== productId || qty > 0).map((line) => (line.product_id === productId ? { ...line, quantity: qty } : line));
        saveGuest(guest);
        setCart(guestToCart(guest));
      }
      track(qty === 0 ? 'remove_from_cart' : 'cart_update', { qty, prev_qty: before }, { surface: 'cart', item: { product_id: productId } });
    },
    [isBuyer, quantityOf, withToken]
  );

  const addItem = useCallback(
    async (product: Product, quantity: number) => {
      if (typeof product.id !== 'number') throw new Error('Sản phẩm chưa có mã hợp lệ, không thể thêm vào giỏ.');
      const qty = Math.max(1, Math.floor(quantity));
      const next = Math.min(MAX_QTY, quantityOf(product.id) + qty);
      if (isBuyer) {
        await withToken((token) => setCartItemRequest(product.id as number, next, token));
      } else {
        const guest = loadGuest();
        const existing = guest.find((line) => line.product_id === product.id);
        if (existing) {
          existing.quantity = next;
        } else {
          guest.push({
            product_id: product.id,
            quantity: next,
            name: product.name,
            image_url: product.image_urls?.[0],
            unit_price: product.price,
            store_id: product.seller_id,
            stock_level: product.stock_level
          });
        }
        saveGuest(guest);
        setCart(guestToCart(guest));
      }
      track('add_to_cart', { qty, price_seen: product.price }, { item: { product_id: product.id } });
    },
    [isBuyer, quantityOf, withToken]
  );

  const removeItem = useCallback((productId: number) => setQuantity(productId, 0), [setQuantity]);

  const emptyCartAction = useCallback(async () => {
    if (isBuyer) {
      await withToken((token) => clearCartRequest(token));
    } else {
      saveGuest([]);
      setCart(emptyCart);
    }
  }, [isBuyer, withToken]);

  const clearCart = useCallback(() => {
    setCart(emptyCart);
    setError(null);
  }, []);

  const value = useMemo<CartContextValue>(
    () => ({
      lines: cart.lines,
      subtotal: cart.subtotal,
      totalQuantity: cart.item_count ?? cart.lines.reduce((total, line) => total + line.quantity, 0),
      loading,
      error,
      isGuest,
      addItem,
      setQuantity,
      removeItem,
      emptyCart: emptyCartAction,
      clearCart,
      refresh
    }),
    [cart, loading, error, isGuest, addItem, setQuantity, removeItem, emptyCartAction, clearCart, refresh]
  );

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart() {
  const context = useContext(CartContext);
  if (!context) {
    throw new Error('useCart must be used within a CartProvider');
  }
  return context;
}
