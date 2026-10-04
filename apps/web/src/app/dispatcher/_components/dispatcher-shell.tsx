'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import { Mono } from '@waypoint/ui';
import { ORDER_CUTOFF_LABEL } from '@waypoint/shared-types';
import {
  BellIcon,
  BoxIcon,
  CalendarIcon,
  ClockIcon,
  DepotIcon,
  GridIcon,
  ListIcon,
  RouteIcon,
  TruckIcon,
  UsersIcon,
  AssignmentIcon,
} from '../../../components/icons';
import {
  formatCountdown,
  formatDay,
  formatInstantDay,
} from '../../../lib/format';
import { endSession } from '../../../lib/session';
import {
  DispatcherScopeProvider,
  useApiNow,
  useDispatcherScope,
} from './dispatcher-context';
import { DemoControls } from './demo-controls';

/**
 * Dispatcher shell — designed at 1440 with a 252px sidebar and a 60px top bar.
 * Priyantha works this on a large office screen all day, so the nav is always
 * visible on desktop; below 1024 it becomes a scrolling strip.
 *
 * The nav follows the working day in order: review orders, plan, confirm,
 * watch the routes, account for deferrals, then notifications and audit.
 */
const NAV = [
  { href: '/dispatcher', label: 'Dashboard', screen: 'D-01', icon: GridIcon },
  { href: '/dispatcher/queue', label: 'Orders', screen: 'D-02', icon: BoxIcon },
  {
    href: '/dispatcher/plan',
    label: 'Plan & allocate',
    screen: 'D-03',
    icon: RouteIcon,
  },
  {
    href: '/dispatcher/tracker',
    label: 'Routes & trips',
    screen: 'D-06',
    icon: TruckIcon,
  },
  {
    href: '/dispatcher/deferrals',
    label: 'Deferrals',
    screen: 'D-07',
    icon: ClockIcon,
  },
  {
    href: '/dispatcher/fleet',
    label: 'Fleet',
    screen: 'D-09',
    icon: TruckIcon,
  },
  {
    href: '/dispatcher/vehicles',
    label: 'Vehicles',
    screen: null,
    icon: TruckIcon,
  },
  {
    href: '/dispatcher/outlets',
    label: 'Outlets',
    screen: null,
    icon: DepotIcon,
  },
  {
    href: '/dispatcher/users',
    label: 'Users',
    screen: null,
    icon: UsersIcon,
  },
  {
    href: '/dispatcher/assignments',
    label: 'Assignments',
    screen: null,
    icon: AssignmentIcon,
  },
  {
    href: '/dispatcher/notifications',
    label: 'Notifications',
    screen: null,
    icon: BellIcon,
  },
  {
    href: '/dispatcher/audit',
    label: 'Audit trail',
    screen: null,
    icon: ListIcon,
  },
] as const;

function isActive(pathname: string, href: string): boolean {
  return href === '/dispatcher' ? pathname === href : pathname.startsWith(href);
}

export function DispatcherShell({ children }: { children: ReactNode }) {
  return (
    <DispatcherScopeProvider>
      <div className="flex min-h-dvh bg-page">
        <Sidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <TopBar />
          <MobileNav />
          <main className="min-w-0 flex-1 p-4 lg:p-6">{children}</main>
        </div>
      </div>
    </DispatcherScopeProvider>
  );
}

function Sidebar() {
  const pathname = usePathname();
  const { unreadCount } = useDispatcherScope();
  return (
    <nav
      aria-label="Dispatcher"
      className="sticky top-0 hidden h-dvh w-sidebar shrink-0 flex-col border-r border-ink/10 bg-card lg:flex"
    >
      <div className="flex h-topbar items-center gap-2.5 border-b border-ink/10 px-4">
        <span
          aria-hidden="true"
          className="grid size-9 place-items-center rounded-chip bg-action text-sm font-semibold text-card"
        >
          W
        </span>
        <span className="leading-tight">
          <span className="block text-sm font-semibold tracking-tight text-ink">
            WAYPOINT
          </span>
          <span className="block text-[0.6875rem] font-semibold uppercase tracking-wider text-ink-muted">
            Dispatch control
          </span>
        </span>
      </div>
      <ul className="flex flex-1 flex-col gap-0.5 p-2">
        {NAV.map((item) => {
          const active = isActive(pathname, item.href);
          const Icon = item.icon;
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                aria-current={active ? 'page' : undefined}
                className={`tap-target flex items-center gap-3 rounded-control px-3 text-sm transition ${
                  active
                    ? 'bg-ink/[0.07] font-semibold text-ink'
                    : 'text-ink-muted hover:bg-page hover:text-ink'
                }`}
              >
                <Icon className="shrink-0" />
                <span className="flex-1">{item.label}</span>
                {item.href === '/dispatcher/notifications' && unreadCount ? (
                  <span className="rounded-pill bg-error px-1.5 text-[0.6875rem] font-semibold leading-5 text-card">
                    {unreadCount}
                    <span className="sr-only"> unread</span>
                  </span>
                ) : item.screen ? (
                  <Mono className="text-[0.6875rem] text-ink-muted/70">
                    {item.screen}
                  </Mono>
                ) : null}
              </Link>
            </li>
          );
        })}
      </ul>
      <div className="border-t border-ink/10 px-4 py-3">
        <p className="text-xs text-ink-muted">Priyantha W. · Dispatcher</p>
        <button
          type="button"
          onClick={() => {
            void endSession();
          }}
          className="tap-target mt-1 flex w-full items-center rounded-control px-2 text-left text-sm text-ink-muted transition hover:bg-page hover:text-ink"
        >
          Sign out
        </button>
      </div>
    </nav>
  );
}

function MobileNav() {
  const pathname = usePathname();
  return (
    <nav
      aria-label="Dispatcher sections"
      className="border-b border-ink/10 bg-card lg:hidden"
    >
      <ul className="flex gap-1 overflow-x-auto px-2 py-1.5">
        {NAV.map((item) => {
          const active = isActive(pathname, item.href);
          return (
            <li key={item.href} className="shrink-0">
              <Link
                href={item.href}
                aria-current={active ? 'page' : undefined}
                className={`tap-target flex items-center rounded-control px-3 text-sm whitespace-nowrap ${
                  active
                    ? 'bg-ink/[0.07] font-semibold text-ink'
                    : 'text-ink-muted'
                }`}
              >
                {item.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function TopBar() {
  const scope = useDispatcherScope();
  const {
    meta,
    today,
    depots,
    depot,
    setDepotId,
    deliveryDate,
    setDeliveryDate,
    unreadCount,
  } = scope;

  return (
    <header className="sticky top-0 z-20 flex min-h-topbar shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b border-ink/10 bg-card px-4 py-2 lg:px-6">
      <div className="flex items-center gap-2 text-sm text-ink">
        <CalendarIcon className="text-ink-muted" />
        {today ? (
          <Mono>{formatInstantDay(meta?.now ?? today)}</Mono>
        ) : (
          <span className="text-ink-muted">Syncing clock…</span>
        )}
        {meta?.demoMode && (
          <span
            className="rounded-chip bg-brand/10 px-1.5 py-0.5 text-[0.6875rem] font-semibold uppercase tracking-wide text-link"
            title="The API runs on the demo clock, so the seeded delivery day is 'today'."
          >
            Demo clock
          </span>
        )}
      </div>

      <DemoControls />

      <label className="flex items-center gap-2 text-sm">
        <span className="text-ink-muted">Depot</span>
        <select
          className="h-9 rounded-control bg-page px-2 text-sm font-medium text-ink ring-1 ring-ink/15"
          value={depot?.depotId ?? ''}
          onChange={(e) => setDepotId(e.target.value)}
          disabled={!depots || depots.length === 0}
        >
          {!depots && <option value="">Loading…</option>}
          {depots?.map((d) => (
            <option key={d.depotId} value={d.depotId}>
              {d.name}
            </option>
          ))}
        </select>
      </label>

      <label className="flex items-center gap-2 text-sm">
        <span className="text-ink-muted">Delivery day</span>
        <input
          type="date"
          className="h-9 rounded-control bg-page px-2 font-mono text-sm text-ink ring-1 ring-ink/15"
          value={deliveryDate ?? ''}
          onChange={(e) => e.target.value && setDeliveryDate(e.target.value)}
        />
        {deliveryDate && (
          <span className="hidden text-ink-muted xl:inline">
            {formatDay(deliveryDate)}
          </span>
        )}
      </label>

      <div className="ml-auto flex items-center gap-3">
        <CutoffPill />
        <Link
          href="/dispatcher/notifications"
          className="tap-target relative grid place-items-center rounded-control text-ink-muted hover:bg-page hover:text-ink"
          aria-label={
            unreadCount
              ? `Notifications, ${unreadCount} unread`
              : 'Notifications'
          }
        >
          <BellIcon width={20} height={20} />
          {unreadCount ? (
            <span className="absolute right-1.5 top-1.5 min-w-4 rounded-pill bg-error px-1 text-center text-[0.625rem] font-bold leading-4 text-card">
              {unreadCount > 99 ? '99+' : unreadCount}
            </span>
          ) : null}
        </Link>
        {/* Sign out is reachable on every surface: the sidebar footer on
            desktop, and here where the sidebar is collapsed. */}
        <button
          type="button"
          onClick={() => {
            void endSession();
          }}
          className="tap-target rounded-control px-2 text-sm font-medium text-ink-muted hover:bg-page hover:text-ink"
        >
          Sign out
        </button>
      </div>
    </header>
  );
}

/**
 * Live countdown to today's 16:00 order cutoff on the API clock. After the
 * cutoff, late orders are still accepted for the following operating day, so
 * the pill says that rather than counting into negative time.
 */
function CutoffPill() {
  const { today, meta } = useDispatcherScope();
  const now = useApiNow();
  if (!now || !today || !meta) return null;
  // The API reports `now` with the business timezone's offset; reuse it so the
  // cutoff instant is 16:00 in Asia/Colombo, not in the browser's zone.
  const offset = /[+-]\d{2}:\d{2}$/.exec(meta.now)?.[0] ?? 'Z';
  const cutoff = Date.parse(`${today}T${ORDER_CUTOFF_LABEL}:00${offset}`);
  const remaining = cutoff - now.getTime();

  if (remaining <= 0) {
    return (
      <span
        role="timer"
        className="flex items-center gap-1.5 rounded-control bg-page px-2.5 py-1.5 text-xs font-semibold text-ink-muted ring-1 ring-ink/10"
      >
        <ClockIcon />
        {ORDER_CUTOFF_LABEL} cutoff passed · late orders roll to the next run
      </span>
    );
  }
  return (
    <span
      role="timer"
      aria-label={`${formatCountdown(remaining)} until the ${ORDER_CUTOFF_LABEL} order cutoff`}
      className="flex items-center gap-1.5 rounded-control bg-error/10 px-2.5 py-1.5 text-xs font-bold text-error"
    >
      <ClockIcon />
      <Mono>
        {formatCountdown(remaining)} TO {ORDER_CUTOFF_LABEL} CUTOFF
      </Mono>
    </span>
  );
}
