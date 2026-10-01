import Link from 'next/link';
import type { Metadata } from 'next';

export const metadata: Metadata = { title: 'Dispatch Control' };

/**
 * Dispatcher shell — designed at 1440 with a 252px sidebar and a 60px top bar.
 * Priyantha works this on a large office screen all day, so the nav is always
 * visible rather than hidden behind a menu.
 */
const NAV = [
  { href: '/dispatcher', label: 'Dispatch Control', screen: 'D-01' },
  { href: '/dispatcher/queue', label: 'Order Queue', screen: 'D-02' },
  { href: '/dispatcher/plan', label: 'Plan & Allocate', screen: 'D-03' },
  { href: '/dispatcher/fleet', label: 'Fleet', screen: 'D-09' },
  { href: '/dispatcher/tracker', label: 'Trip Tracker', screen: 'D-06' },
  { href: '/dispatcher/deferrals', label: 'Deferral Logs', screen: 'D-07' },
  { href: '/dispatcher/forecast', label: 'Capacity Forecast', screen: 'D-08' },
] as const;

export default function DispatcherLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh">
      <nav
        aria-label="Dispatcher"
        className="hidden w-sidebar shrink-0 flex-col gap-1 border-r border-ink/10 bg-card p-3 lg:flex"
      >
        <span className="px-2 pb-3 pt-1 text-xs font-semibold uppercase tracking-widest text-brand">
          Waypoint
        </span>
        {NAV.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            className="tap-target flex items-center justify-between rounded-control px-3 text-sm text-ink-muted transition hover:bg-page hover:text-ink"
          >
            {item.label}
            <span className="tabular text-[0.6875rem] text-ink-muted/60">
              {item.screen}
            </span>
          </Link>
        ))}
      </nav>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-topbar shrink-0 items-center border-b border-ink/10 bg-card px-4">
          <span className="text-sm font-medium text-ink">
            Dispatcher · Peliyagoda
          </span>
        </header>
        <main className="min-w-0 flex-1 p-4">{children}</main>
      </div>
    </div>
  );
}
