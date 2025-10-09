'use client';

import { AuthProvider } from './auth/AuthProvider';
import { CartProvider } from './cart/CartProvider';

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <AuthProvider>
      <CartProvider>{children}</CartProvider>
    </AuthProvider>
  );
}
