'use client';

import { AuthProvider } from './auth/AuthProvider';
import { ChatProvider } from './ai/ChatProvider';
import { CartProvider } from './cart/CartProvider';
import { TrackingProvider } from './tracking/TrackingProvider';

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <AuthProvider>
      <CartProvider>
        <TrackingProvider />
        <ChatProvider>{children}</ChatProvider>
      </CartProvider>
    </AuthProvider>
  );
}
