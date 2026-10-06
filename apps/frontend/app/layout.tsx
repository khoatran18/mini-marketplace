import type { Metadata } from 'next';
import { cookies } from 'next/headers';
import './globals.css';
import { Providers } from '../components/Providers';
import { NavBar } from '../components/NavBar';
import { THEME_COOKIE, themeInitScript } from '../lib/theme';

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
  // The cookie lets the server render the right class straight away; the inline script covers "system" mode
  // (and a missing cookie) before first paint, so there is no flash of the wrong theme.
  const theme = cookies().get(THEME_COOKIE)?.value;
  return (
    <html lang="vi" className={theme === 'dark' ? 'dark' : undefined} style={theme === 'dark' ? { colorScheme: 'dark' } : undefined} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body className="min-h-screen bg-bg font-sans text-text antialiased">
        <Providers>
          <NavBar />
          <main className="w-full">{children}</main>
        </Providers>
      </body>
    </html>
  );
}
