'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react';
import { Mono } from '@waypoint/ui';
import type { MetaResponse, Outlet } from '@waypoint/shared-types';
import {
  formatCountdown,
  formatInstantDay,
  msUntilCutoff,
} from '../../../lib/format';
import { endSession } from '../../../lib/session';
import { useApiClock, useStoreOutlet } from '../_lib/use-store';
import { BrandChip } from './ui';

/**
 * Store-manager shell.
 *
 * The outlet in the header is the authenticated caller's own outlet, fetched
 * from GET /outlets, which the Go API pins to `app_user.outlet_id`. There is no
 * outlet picker anywhere in the workspace — a store manager cannot look at, or
 * order for, another outlet, and the request carries no outlet or depot filter
 * that could widen what the server returns.
 */
interface StoreScope {
  readonly outlet: Outlet | undefined;
  readonly outletLoading: boolean;
  readonly outletError: unknown;
  readonly reloadOutlet: () => void;
  readonly meta: MetaResponse | undefined;
}

const Ctx = createContext<StoreScope | null>(null);

export function useStoreScope(): StoreScope {
  const scope = useContext(Ctx);
  if (!scope) {
    throw new Error('useStoreScope must be used inside StoreShell');
  }
  return scope;
}

const NAV = [
  { href: '/store', label: 'Store home' },
  { href: '/store/order', label: 'Place order' },
  { href: '/store/orders', label: 'My orders' },
] as const;

function isActive(pathname: string, href: string): boolean {
  return href === '/store' ? pathname === '/store' : pathname.startsWith(href);
}

export function StoreShell({ children }: { children: ReactNode }) {
  const outletQuery = useStoreOutlet();
  const metaQuery = useApiClock();

  const value: StoreScope = {
    outlet: outletQuery.data?.[0],
    outletLoading: outletQuery.loading,
    outletError: outletQuery.error,
    reloadOutlet: outletQuery.reload,
    meta: metaQuery.data,
  };

  return (
    <Ctx.Provider value={value}>
      <div className="flex min-h-dvh flex-col bg-page">
        <StoreHeader />
        <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-5 sm:px-6 lg:py-8">
          {children}
        </main>
      </div>
    </Ctx.Provider>
  );
}

function StoreHeader() {
  const pathname = usePathname();
  const { outlet, outletLoading } = useStoreScope();

  return (
    <header className="sticky top-0 z-20 border-b border-line bg-card">
      {/* Phone: identity + sign out, then the cutoff, then the nav.
          Tablet: identity, cutoff, sign out, nav below. Desktop: one row. */}
      <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-2 sm:px-6">
        <div className="order-1 flex min-w-0 flex-1 flex-col sm:flex-none">
          {outlet ? (
            <>
              <span className="flex items-center gap-2 text-sm font-semibold text-ink">
                <Mono>{outlet.outletId}</Mono>
                <span className="truncate">{outlet.name}</span>
                <BrandChip brand={outlet.brand} />
              </span>
              <span className="text-xs text-ink-muted">
                {outlet.district} · window{' '}
                <Mono>
                  {outlet.windowOpenTime}–{outlet.windowCloseTime}
                </Mono>
              </span>
            </>
          ) : (
            <span className="text-sm text-ink-muted" role="status">
              {outletLoading ? 'Loading your outlet…' : 'Your outlet'}
            </span>
          )}
        </div>

        <div className="order-3 w-full sm:order-2 sm:ml-auto sm:w-auto lg:order-3">
          <CutoffPill />
        </div>
        <button
          type="button"
          onClick={() => {
            void endSession();
          }}
          className="tap-target order-2 whitespace-nowrap rounded-control px-2 text-sm font-medium text-ink-muted hover:bg-page hover:text-ink sm:order-3 lg:order-4"
        >
          Sign out
        </button>

        <nav
          aria-label="Store"
          className="order-4 -mx-1 flex w-full gap-1 overflow-x-auto lg:order-2 lg:mx-0 lg:w-auto"
        >
          {NAV.map((item) => {
            const active = isActive(pathname, item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                aria-current={active ? 'page' : undefined}
                className={`tap-target flex items-center whitespace-nowrap rounded-control px-3 text-sm transition ${
                  active
                    ? 'bg-ink/[0.07] font-semibold text-ink'
                    : 'text-ink-muted hover:bg-page hover:text-ink'
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
      </div>
    </header>
  );
}

/**
 * Live countdown to today's 16:00 cutoff. After it, late orders are still
 * accepted for the next run, so the pill says that rather than counting into
 * negative time. It ticks on its own so the rest of the shell does not
 * re-render every second.
 */
export function CutoffPill() {
  // The device's real clock, ticking locally — no API call per second.
  const [remaining, setRemaining] = useState<number | undefined>(undefined);
  useEffect(() => {
    const update = () => setRemaining(msUntilCutoff(new Date()));
    update();
    const id = window.setInterval(update, 1000);
    return () => window.clearInterval(id);
  }, []);
  if (remaining === undefined) return null;

  if (remaining <= 0) {
    return (
      <span
        role="timer"
        className="flex items-center gap-1.5 rounded-control bg-page px-2.5 py-1.5 text-xs font-semibold text-ink-muted ring-1 ring-ink/10"
      >
        Cutoff passed · late orders roll to the next run
      </span>
    );
  }
  return (
    <span
      role="timer"
      aria-label={`${formatCountdown(remaining)} until the 16:00 order cutoff`}
      className="flex items-center gap-1.5 rounded-control bg-error/10 px-2.5 py-1.5 text-xs font-bold text-error"
    >
      <Mono>{formatCountdown(remaining)} TO 16:00 CUTOFF</Mono>
    </span>
  );
}

/** Shared header strip used by the inner screens: today on the API clock. */
export function StoreDayLine() {
  const { meta } = useStoreScope();
  if (!meta) return null;
  return (
    <p className="text-sm text-ink-muted">
      Today is <Mono>{formatInstantDay(meta.now)}</Mono>
      {meta.demoMode ? ' · demo clock' : ''}
    </p>
  );
}
