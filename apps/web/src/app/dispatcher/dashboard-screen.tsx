'use client';

import Link from 'next/link';
import { LIVE_REFRESH_MS, Mono } from '@waypoint/ui';
import type { OrderStatus } from '@waypoint/shared-types';
import { api } from '../../lib/api';
import { formatDateTime, formatDay, plural } from '../../lib/format';
import { useApiQuery } from '../../lib/use-api-query';
import { EmptyState, ErrorState, LoadingState } from '../../components/states';
import {
  BoxIcon,
  ClockIcon,
  RouteIcon,
  TruckIcon,
} from '../../components/icons';
import { useDispatcherScope } from './_components/dispatcher-context';
import {
  ButtonLink,
  MetricCard,
  PageHeader,
  SectionHeading,
} from './_components/ui';
import {
  notificationLink,
  notificationType,
} from './notifications/notification-model';

const COUNTED: readonly OrderStatus[] = [
  'PLACED',
  'CONFIRMED',
  'ALLOCATED',
  'DEFERRED',
];

/**
 * D-01 — Priyantha's first screen: where today's delivery day stands, and what
 * needs a decision. Every number is a live API count for the selected depot
 * and delivery day; nothing here is decorative.
 */
export function DashboardScreen() {
  const { depot, deliveryDate } = useDispatcherScope();
  const key = depot && deliveryDate ? `${depot.depotId}:${deliveryDate}` : null;

  const counts = useApiQuery(
    key && `dash-counts:${key}`,
    async () => {
      const base = { deliveryDate, depotId: depot?.depotId, limit: 1 };
      const [all, ...byStatus] = await Promise.all([
        api.listOrders(base),
        ...COUNTED.map((status) => api.listOrders({ ...base, status })),
      ]);
      return {
        total: all.total,
        ...Object.fromEntries(
          COUNTED.map((s, i) => [s, byStatus[i]?.total ?? 0]),
        ),
      } as Record<'total' | OrderStatus, number>;
    },
    { pollMs: LIVE_REFRESH_MS },
  );
  const routes = useApiQuery(
    key && `dash-routes:${key}`,
    () =>
      api.listRoutes({ date: deliveryDate as string, depotId: depot?.depotId }),
    { pollMs: LIVE_REFRESH_MS },
  );
  const fleet = useApiQuery(key && `dash-fleet:${key}`, () =>
    api.getVehicles({ depotId: depot?.depotId, date: deliveryDate }),
  );
  const alerts = useApiQuery(
    'dash-alerts',
    () => api.listNotifications({ limit: 20 }),
    { pollMs: LIVE_REFRESH_MS },
  );

  const c = counts.data;
  const trips = routes.data ?? [];
  const legs = trips.flatMap((r) => r.legs ?? []);
  const delivered = legs.filter((l) => l.status === 'DELIVERED').length;
  const failed = legs.filter((l) => l.status === 'FAILED').length;
  const available = fleet.data?.filter((v) => v.status === 'AVAILABLE').length;
  const unreadAlerts = (alerts.data ?? []).filter((n) => !n.read);
  const dash = (n: number | undefined) => (n === undefined ? '—' : n);

  // Where the day is in the workflow, from the real counts.
  const planned = trips.length > 0;
  const stage = !c
    ? -1
    : c.total === 0
      ? 0
      : !planned
        ? c.CONFIRMED > 0
          ? 1
          : 0
        : delivered + failed < legs.length
          ? 3
          : 4;

  return (
    <>
      <PageHeader
        screenId="D-01"
        title="Dispatch control"
        description={
          <>
            {depot?.name ?? '…'} · delivery day{' '}
            {deliveryDate ? formatDay(deliveryDate) : '…'}
          </>
        }
      />

      <Workflow stage={stage} />

      {counts.error ? (
        <div className="mb-4">
          <ErrorState error={counts.error} onRetry={counts.reload} />
        </div>
      ) : null}

      <section aria-labelledby="overview-heading" className="mb-6">
        <SectionHeading>
          <span id="overview-heading">System overview</span>
        </SectionHeading>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <MetricCard
            icon={BoxIcon}
            label="Orders for the day"
            value={dash(c?.total)}
            unit="orders"
            hint={
              c
                ? `${c.PLACED} placed, not yet confirmed by the store`
                : undefined
            }
            href="/dispatcher/queue"
            linkLabel="Check orders"
          />
          <MetricCard
            icon={RouteIcon}
            label="Waiting for planning"
            value={dash(c?.CONFIRMED)}
            unit="confirmed"
            hint={
              c && c.CONFIRMED > 0
                ? planned
                  ? 'Confirmed after the plan — run planning again'
                  : 'Ready to plan'
                : undefined
            }
            hintTone={c && c.CONFIRMED > 0 ? 'warning' : 'muted'}
            href="/dispatcher/plan"
            linkLabel="Plan the day"
          />
          <MetricCard
            icon={TruckIcon}
            label="Vehicles available"
            value={dash(available)}
            unit={fleet.data ? `of ${fleet.data.length}` : undefined}
            hint={fleet.error ? 'Could not load the fleet' : undefined}
            hintTone={fleet.error ? 'error' : 'muted'}
            href="/dispatcher/fleet"
            linkLabel="Check fleet"
          />
          <MetricCard
            icon={BoxIcon}
            label="Allocated orders"
            value={dash(c?.ALLOCATED)}
            unit="orders"
            href="/dispatcher/tracker"
            linkLabel="Check routes"
          />
          <MetricCard
            icon={TruckIcon}
            label="Trips confirmed"
            value={routes.data ? trips.length : '—'}
            unit="trips"
            hint={
              routes.data && legs.length > 0
                ? `${delivered} / ${legs.length} stops delivered${failed ? ` · ${failed} failed` : ''}`
                : undefined
            }
            hintTone={failed > 0 ? 'error' : 'muted'}
            href="/dispatcher/tracker"
            linkLabel="Check trips"
          />
          <MetricCard
            icon={ClockIcon}
            label="Deferred orders"
            value={dash(c?.DEFERRED)}
            unit="deferred"
            hint={
              c && c.DEFERRED > 0
                ? 'Each carries its rule and reason'
                : undefined
            }
            href="/dispatcher/deferrals"
            linkLabel="Check log"
          />
        </div>
      </section>

      <section aria-labelledby="alerts-heading">
        <SectionHeading
          aside={
            <Link
              href="/dispatcher/notifications"
              className="text-xs font-medium text-link hover:underline"
            >
              All notifications
            </Link>
          }
        >
          <span id="alerts-heading">Needs attention</span>
        </SectionHeading>
        {alerts.loading ? (
          <LoadingState label="Loading alerts…" />
        ) : alerts.error ? (
          <ErrorState error={alerts.error} onRetry={alerts.reload} />
        ) : unreadAlerts.length === 0 ? (
          <EmptyState title="Nothing needs a decision right now">
            Shortfalls from loading and failed or delayed deliveries appear here
            as they happen.
          </EmptyState>
        ) : (
          <ul className="space-y-2">
            {unreadAlerts.slice(0, 5).map((n) => {
              const type = notificationType(n.type);
              const link = notificationLink(n);
              const severe = type.tone === 'error';
              return (
                <li
                  key={n.id}
                  className={`flex flex-wrap items-center gap-3 rounded-card p-4 ring-1 ${
                    severe
                      ? 'bg-error/5 ring-error/25'
                      : 'bg-warning/5 ring-warning/30'
                  }`}
                >
                  <span
                    aria-hidden="true"
                    className={`grid size-7 shrink-0 place-items-center rounded-chip text-sm font-bold ${
                      severe
                        ? 'bg-error/10 text-error'
                        : 'bg-warning/10 text-warning'
                    }`}
                  >
                    !
                  </span>
                  <div className="min-w-0 flex-1">
                    <p
                      className={`text-[0.6875rem] font-bold uppercase tracking-wide ${severe ? 'text-error' : 'text-warning'}`}
                    >
                      {type.label}
                    </p>
                    <p className="font-semibold text-ink">{n.title}</p>
                    <p className="text-sm text-ink-muted">
                      {n.message}{' '}
                      <Mono className="text-xs">
                        {formatDateTime(n.createdAt)}
                      </Mono>
                    </p>
                  </div>
                  {link && (
                    <ButtonLink href={link.href} variant="primary">
                      {link.label}
                    </ButtonLink>
                  )}
                </li>
              );
            })}
            {unreadAlerts.length > 5 && (
              <li className="text-sm text-ink-muted">
                and {plural(unreadAlerts.length - 5, 'more unread alert')} in{' '}
                <Link
                  href="/dispatcher/notifications"
                  className="text-link hover:underline"
                >
                  Notifications
                </Link>
              </li>
            )}
          </ul>
        )}
      </section>
    </>
  );
}

const STAGES = [
  { label: 'Orders', href: '/dispatcher/queue' },
  { label: 'Plan the day', href: '/dispatcher/plan' },
  { label: 'Confirm routes', href: '/dispatcher/plan' },
  { label: 'Monitor trips', href: '/dispatcher/tracker' },
  { label: 'Review & audit', href: '/dispatcher/audit' },
];

/** The dispatcher's day in order, with the current stage marked from real counts. */
function Workflow({ stage }: { stage: number }) {
  return (
    <nav aria-label="Dispatch workflow" className="mb-6 overflow-x-auto">
      <ol className="flex min-w-max items-center gap-1">
        {STAGES.map((s, i) => {
          const state =
            stage < 0
              ? 'todo'
              : i < stage
                ? 'done'
                : i === stage
                  ? 'current'
                  : 'todo';
          return (
            <li key={s.label} className="flex items-center gap-1">
              {i > 0 && (
                <span aria-hidden="true" className="text-ink-muted">
                  →
                </span>
              )}
              <Link
                href={s.href}
                aria-current={state === 'current' ? 'step' : undefined}
                className={`flex h-9 items-center gap-1.5 rounded-pill px-3 text-xs font-medium ring-1 ${
                  state === 'current'
                    ? 'bg-action text-card ring-action'
                    : state === 'done'
                      ? 'bg-success/10 text-success ring-success/30'
                      : 'bg-card text-ink-muted ring-ink/15 hover:text-ink'
                }`}
              >
                <span aria-hidden="true">{state === 'done' ? '✓' : i + 1}</span>
                {s.label}
              </Link>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
