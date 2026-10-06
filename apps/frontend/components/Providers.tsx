'use client';

import { AuthProvider } from './auth/AuthProvider';
import { CartProvider } from './cart/CartProvider';
import { TrackingProvider } from './tracking/TrackingProvider';

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <AuthProvider>
      <CartProvider>
        <TrackingProvider />
        {children}
      </CartProvider>
    </AuthProvider>
  );
}
