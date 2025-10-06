import type { Metadata } from 'next';
import './globals.css';
import { Providers } from '../components/Providers';
import { NavBar } from '../components/NavBar';

export const metadata: Metadata = {
  title: 'Mini Marketplace',
  description: 'Frontend for the mini marketplace platform powered by the Go services.',
  icons: {
    icon: '/favicon.ico'
  }
};

export default function RootLayout({
  children
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <Providers>
          <NavBar />
          <main>{children}</main>
        </Providers>
      </body>
    </html>
  );
}
