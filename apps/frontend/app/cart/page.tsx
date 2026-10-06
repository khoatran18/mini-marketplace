import { CartPageClient } from './CartPageClient';

// public: visitors who are not signed in have a local guest cart that is merged into the server cart on login
export default function CartPage() {
  return <CartPageClient />;
}
