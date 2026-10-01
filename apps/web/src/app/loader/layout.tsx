import Link from 'next/link';
import type { Metadata } from 'next';
import { OfflineBanner } from '@waypoint/ui';

export const metadata: Metadata = { title: 'Loading' };

/**
 * Loader shell — a phone at 402 and a shared dock tablet at 1024, where the
 * design switches to master–detail (L-T1…L-T3). Bottom tabs because Nadeesha
 * works one-handed with the other hand on a trolley.
 */
const TABS = [
  { href: '/loader', label: 'Trips' },
  { href: '/loader/load-list', label: 'Load list' },
  { href: '/loader/flags', label: 'Flags' },
  { href: '/loader/account', label: 'Account' },
] as const;

export default function LoaderLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh flex-col">
      <OfflineBanner />
      <header className="flex h-topbar shrink-0 items-center border-b border-ink/10 bg-card px-4">
        <span className="text-sm font-medium text-ink">
          Loader · Peliyagoda dock
        </span>
      </header>

      <main className="flex-1 p-3 pb-20">{children}</main>

      <nav
        aria-label="Loader"
        className="fixed inset-x-0 bottom-0 flex border-t border-ink/10 bg-card"
      >
        {TABS.map((tab) => (
          <Link
            key={tab.href}
            href={tab.href}
            className="tap-target flex flex-1 items-center justify-center text-xs font-medium text-ink-muted transition hover:text-ink"
          >
            {tab.label}
          </Link>
        ))}
      </nav>
    </div>
  );
}
