import Link from 'next/link';
import type { Metadata } from 'next';

export const metadata: Metadata = { title: 'My store' };

const NAV = [
  { href: '/store', label: 'Store home' },
  { href: '/store/order', label: 'Place order' },
  { href: '/store/orders', label: 'My orders' },
] as const;

/** Store manager shell — desktop or phone at the outlet. */
export default function StoreLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh flex-col">
      <header className="flex h-topbar shrink-0 items-center gap-4 border-b border-ink/10 bg-card px-4">
        {/* Placeholder until auth lands: Ishara's outlet in the Day 5 design. */}
        <span className="text-sm font-medium text-ink">
          OUT014 · Colombo Central
        </span>
        <nav aria-label="Store" className="flex gap-1">
          {NAV.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="tap-target flex items-center rounded-control px-3 text-sm text-ink-muted transition hover:bg-page hover:text-ink"
            >
              {item.label}
            </Link>
          ))}
        </nav>
      </header>
      <main className="mx-auto w-full max-w-4xl flex-1 p-4">{children}</main>
    </div>
  );
}
