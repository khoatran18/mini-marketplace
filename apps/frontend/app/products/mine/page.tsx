import { redirect } from 'next/navigation';

// old URL: the seller product list now lives in the seller console
export default function MyProductsPage() {
  redirect('/seller/products');
}
