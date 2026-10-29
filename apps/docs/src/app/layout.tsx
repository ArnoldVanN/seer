import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import { Provider } from '@/components/provider';
import './global.css';

const inter = Inter({
  subsets: ['latin'],
});

export const metadata: Metadata = {
  // Absolute base for OG images; set at image build time (see Dockerfile).
  metadataBase: new URL(process.env.SITE_URL ?? 'http://localhost:3200'),
  title: {
    template: '%s | Seer',
    default: 'Seer',
  },
  description: 'Highly available, self-hosted status pages and uptime monitoring.',
  icons: { icon: '/seer-icon.svg' },
};

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" className={inter.className} suppressHydrationWarning>
      <body className="flex flex-col min-h-screen">
        <Provider>{children}</Provider>
      </body>
    </html>
  );
}
