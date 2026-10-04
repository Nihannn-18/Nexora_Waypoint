'use client';

import Link from 'next/link';
import { useMemo } from 'react';
import { LIVE_REFRESH_MS, Mono } from '@waypoint/ui';
import type { Route, Vehicle } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import {
  formatDay,
  formatKg,
  formatKm,
  formatLitres,
  formatM3,
  formatMinutes,
  plural,
} from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { ArrowRightIcon } from '../../../components/icons';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { indexBy } from '../_components/data';
import {
  BrandChip,
  ButtonLink,
  Meter,
  PageHeader,
  RouteStatusBadge,
} from '../_components/ui';
import { routeFigures } from './route-model';

/**
 * D-06 — the confirmed routes for the day: Route → Vehicle → Trip → District →
 * Brand → Stops, with how full and how long each is. Refreshes every 30 s.
 *
 * Driver, ETA and sync status per trip need GET /routes/live, which the API
 * does not serve yet; progress here comes from each stop's recorded status.
 */
export function RoutesScreen() {
  const { depot, deliveryDate } = useDispatcherScope();
  const key = depot && deliveryDate ? `${depot.depotId}:${deliveryDate}` : null;

  const routes = useApiQuery(
    key && `routes:${key}`,
    () =>
      api.listRoutes({ date: deliveryDate as string, depotId: depot?.depotId }),
    { pollMs: LIVE_REFRESH_MS },
  );
  const vehicles = useApiQuery(key && `vehicles:${key}`, () =>
    api.getVehicles({ depotId: depot?.depotId, date: deliveryDate }),
  );
  const vehicleById = useMemo(
    () => indexBy(vehicles.data ?? [], 'vehicleId') as Map<string, Vehicle>,
    [vehicles.data],
  );
  const list = routes.data ?? [];
  const stopsTotal = list.reduce((n, r) => n + (r.legs?.length ?? 0), 0);
  const stopsDone = list.reduce(
    (n, r) => n + (r.legs ?? []).filter((l) => l.status === 'DELIVERED').length,
    0,
  );
  const stopsFailed = list.reduce(
    (n, r) => n + (r.legs ?? []).filter((l) => l.status === 'FAILED').length,
    0,
  );

  return (
    <>
      <PageHeader
        screenId="D-06"
        title="Routes & trips"
        description={
          <>
            Confirmed routes at {depot?.name ?? '…'} for{' '}
            {deliveryDate ? formatDay(deliveryDate) : '…'}. Refreshes every 30 s
            {routes.refreshing && routes.data ? ' · updating…' : '.'}
          </>
        }
      />

      {routes.loading ? (
        <LoadingState label="Loading routes…" />
      ) : routes.error ? (
        <ErrorState error={routes.error} onRetry={routes.reload} />
      ) : list.length === 0 ? (
        <EmptyState
          title={`No confirmed routes for ${deliveryDate ? formatDay(deliveryDate) : 'this day'}`}
          action={
            <ButtonLink href="/dispatcher/plan" variant="primary">
              Plan this day
            </ButtonLink>
          }
        >
          Routes exist only after a plan is confirmed. Run planning, give each
          deferral a reason, then confirm.
        </EmptyState>
      ) : (
        <>
          <dl className="mb-4 flex flex-wrap gap-2">
            {[
              ['Trips', list.length],
              ['Vehicles', new Set(list.map((r) => r.vehicleId)).size],
              ['Stops delivered', `${stopsDone} / ${stopsTotal}`],
              ['Stops failed', stopsFailed],
            ].map(([label, value]) => (
              <div
                key={label}
                className="rounded-control bg-card px-3 py-1.5 text-sm ring-1 ring-ink/10"
              >
                <dt className="inline text-ink-muted">{label}: </dt>
                <dd className="inline font-mono font-semibold text-ink">
                  {value}
                </dd>
              </div>
            ))}
          </dl>
          {vehicles.error ? (
            <div className="mb-3">
              <ErrorState
                error={vehicles.error}
                onRetry={vehicles.reload}
                compact
              />
            </div>
          ) : null}
          <ul className="grid gap-3 lg:grid-cols-2">
            {list.map((r) => (
              <li key={r.routeId}>
                <RouteCard
                  route={r}
                  vehicle={vehicleById.get(r.vehicleId)}
                  sameDay={list}
                />
              </li>
            ))}
          </ul>
        </>
      )}
    </>
  );
}

function RouteCard({
  route,
  vehicle,
  sameDay,
}: {
  route: Route;
  vehicle: Vehicle | undefined;
  sameDay: readonly Route[];
}) {
  const f = routeFigures(route, vehicle, sameDay);
  return (
    <article className="h-full rounded-card bg-card ring-1 ring-ink/10">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-ink/10 px-4 py-3">
        <Mono className="text-base font-bold text-ink">{route.vehicleId}</Mono>
        <span className="rounded-chip bg-ink/[0.06] px-1.5 py-0.5 text-xs font-semibold text-ink">
          Trip {route.tripNo}
        </span>
        <BrandChip brand={route.brand} />
        <span className="text-sm font-medium text-ink">{route.district}</span>
        <span className="ml-auto">
          <RouteStatusBadge status={route.status} />
        </span>
      </header>
      <div className="space-y-3 px-4 py-3">
        <p className="text-sm text-ink">
          <Mono className="font-semibold">
            {f.stopsDelivered} / {f.stopsTotal}
          </Mono>{' '}
          stops delivered
          {f.stopsFailed > 0 && (
            <span className="font-medium text-error">
              {' '}
              · ✕ {plural(f.stopsFailed, 'failed stop')}
            </span>
          )}
        </p>
        <div className="grid gap-3 sm:grid-cols-3">
          <Meter
            label="Weight"
            value={route.totalWeightKg}
            limit={vehicle?.weightCapKg}
            format={formatKg}
          />
          <Meter
            label="Volume"
            value={route.totalVolumeM3}
            limit={vehicle?.volumeCapM3}
            format={formatM3}
          />
          <Meter
            label="Time budget"
            value={f.vehicleDayMinutes}
            limit={f.budgetMinutes}
            format={formatMinutes}
            note={
              route.brand === 'FRESH'
                ? 'Fresh minutes today'
                : 'Style + Tech minutes today'
            }
          />
        </div>
        <p className="text-xs text-ink-muted">
          <Mono className="text-ink">{formatKm(route.distanceKm)}</Mono>{' '}
          outbound + between stops
          {f.fuelLitres !== undefined && (
            <>
              {' '}
              · est. fuel{' '}
              <Mono className="text-ink">{formatLitres(f.fuelLitres)}</Mono>
            </>
          )}
          {vehicle && (
            <>
              {' '}
              · weekly quota{' '}
              <Mono className="text-ink">
                {formatLitres(vehicle.weeklyFuelQuotaL)}
              </Mono>
            </>
          )}
        </p>
      </div>
      <Link
        href={`/dispatcher/tracker/${route.routeId}`}
        className="tap-target flex items-center justify-end gap-1 border-t border-ink/10 px-4 text-sm font-medium text-link hover:bg-page"
      >
        Open route and stops <ArrowRightIcon />
      </Link>
    </article>
  );
}
