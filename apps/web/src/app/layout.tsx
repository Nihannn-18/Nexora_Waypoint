import type { Metadata, Viewport } from 'next';
import './global.css';

export const metadata: Metadata = {
  title: {
    default: 'Waypoint — Delivery Planning',
    template: '%s · Waypoint',
  },
  description:
    'One plan, four roles, every outlet served — or explained. Delivery planning for Waypoint Group.',
  applicationName: 'Waypoint',
};

export const viewport: Viewport = {
  // The Driver and Loader surfaces are used on phones; zoom stays enabled
  // because a loader may need to magnify a SKU line on a dock tablet.
  width: 'device-width',
  initialScale: 1,
  themeColor: '#019EFF',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="min-h-dvh bg-page text-ink antialiased">{children}</body>
    </html>
  );
}
