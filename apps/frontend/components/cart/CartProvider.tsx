'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState
} from 'react';
import type { CartEntry, Product } from '../../lib/types';

interface CartContextValue {
  items: CartEntry[];
  totalQuantity: number;
  totalPrice: number;
  addItem: (product: Product, quantity: number) => void;
  updateItemQuantity: (productId: number, quantity: number) => void;
  removeItem: (productId: number) => void;
  clearCart: () => void;
}

const CartContext = createContext<CartContextValue | undefined>(undefined);

const STORAGE_KEY = 'mini-marketplace-cart';

function loadPersistedCart(): CartEntry[] {
  if (typeof window === 'undefined') {
    return [];
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }

    const parsed = JSON.parse(raw) as CartEntry[];
    if (!Array.isArray(parsed)) {
      return [];
    }

    return parsed.map((entry) => ({
      product: entry.product,
      quantity: entry.quantity
    }));
  } catch (error) {
    console.warn('Failed to parse cart state from storage', error);
    return [];
  }
}

function persistCart(items: CartEntry[]) {
  if (typeof window === 'undefined') {
    return;
  }

  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  } catch (error) {
    console.warn('Failed to persist cart state', error);
  }
}

export function CartProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<CartEntry[]>([]);

  useEffect(() => {
    setItems(loadPersistedCart());
  }, []);

  useEffect(() => {
    persistCart(items);
  }, [items]);

  const addItem = useCallback((product: Product, quantity: number) => {
    setItems((prev) => {
      const existing = prev.find((entry) => entry.product.id === product.id);
      if (existing) {
        return prev.map((entry) =>
          entry.product.id === product.id
            ? { ...entry, quantity: Math.max(1, entry.quantity + quantity) }
            : entry
        );
      }
      return [
        ...prev,
        {
          product,
          quantity: Math.max(1, quantity)
        }
      ];
    });
  }, []);

  const updateItemQuantity = useCallback((productId: number, quantity: number) => {
    setItems((prev) =>
      prev
        .map((entry) =>
          entry.product.id === productId
            ? { ...entry, quantity: Math.max(1, quantity) }
            : entry
        )
        .filter((entry) => entry.quantity > 0)
    );
  }, []);

  const removeItem = useCallback((productId: number) => {
    setItems((prev) => prev.filter((entry) => entry.product.id !== productId));
  }, []);

  const clearCart = useCallback(() => {
    setItems([]);
  }, []);

  const totalQuantity = useMemo(
    () => items.reduce((total, entry) => total + entry.quantity, 0),
    [items]
  );

  const totalPrice = useMemo(
    () => items.reduce((total, entry) => total + (entry.product.price ?? 0) * entry.quantity, 0),
    [items]
  );

  const value = useMemo(
    () => ({
      items,
      totalQuantity,
      totalPrice,
      addItem,
      updateItemQuantity,
      removeItem,
      clearCart
    }),
    [items, totalQuantity, totalPrice, addItem, updateItemQuantity, removeItem, clearCart]
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
