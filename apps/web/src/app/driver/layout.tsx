import Link from 'next/link';
import type { Metadata } from 'next';
import { OfflineBanner } from '@waypoint/ui';

export const metadata: Metadata = { title: 'Run sheet' };

/**
 * Driver shell — phone at 402, used only while safely stopped. Primary actions
 * get 56px targets rather than 48px: Kasun taps these with one thumb, quickly.
 */
const TABS = [
  { href: '/driver/deliveries', label: 'Deliveries' },
  { href: '/driver', label: 'Cockpit' },
  { href: '/driver/vehicle', label: 'Vehicle' },
  { href: '/driver/log', label: 'Log' },
] as const;

export default function DriverLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh flex-col">
      {/* Offline is neutral, never red: losing signal in hill country is expected. */}
      <OfflineBanner />

      <main className="flex-1 p-3 pb-20">{children}</main>

      <nav
        aria-label="Driver"
        className="fixed inset-x-0 bottom-0 flex border-t border-ink/10 bg-card"
      >
        {TABS.map((tab) => (
          <Link
            key={tab.href}
            href={tab.href}
            className="tap-target-driver flex flex-1 items-center justify-center text-xs font-medium text-ink-muted transition hover:text-ink"
          >
            {tab.label}
          </Link>
        ))}
      </nav>
    </div>
  );
}
