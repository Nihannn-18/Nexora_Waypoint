'use client';

import Link from 'next/link';
import {
  ClockTimeText,
  LIVE_REFRESH_MS,
  Mono,
  OrderStatusBadge,
} from '@waypoint/ui';
import type { CustomerOrder } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import {
  formatDay,
  formatKg,
  formatKm,
  formatLitres,
  formatM3,
  formatMinutes,
  humanize,
  plural,
} from '../../../../lib/format';
import { useApiQuery } from '../../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../../components/states';
import { indexBy, loadOutlets } from '../../_components/data';
import {
  BrandChip,
  ButtonLink,
  Card,
  Fact,
  LegStatusBadge,
  Meter,
  PageHeader,
  RouteStatusBadge,
  SectionHeading,
} from '../../_components/ui';
import { routeFigures } from '../route-model';

/**
 * One confirmed route: summary (vehicle, depot, trip, district, brand,
 * capacity, time, fuel) then its stops in delivery order. The loader loads in
 * reverse of this order; the driver works it top to bottom.
 */
export function RouteDetail({ routeId }: { routeId: string }) {
  const detail = useApiQuery(
    `route:${routeId}`,
    async () => {
      const route = await api.getRoute(routeId);
      const [vehicles, outlets, dayRoutes, orders, depots] = await Promise.all([
        api.getVehicles({ depotId: route.depotId, date: route.routeDate }),
        loadOutlets(route.depotId),
        api.listRoutes({ date: route.routeDate, depotId: route.depotId }),
        Promise.all((route.legs ?? []).map((l) => api.getOrder(l.orderId))),
        api.getDepots(),
      ]);
      return {
        route,
        vehicle: vehicles.find((v) => v.vehicleId === route.vehicleId),
        outlets: indexBy(outlets, 'outletId'),
        dayRoutes,
        orders: indexBy(orders, 'orderId') as Map<string, CustomerOrder>,
        depotName: depots.find((d) => d.depotId === route.depotId)?.name,
      };
    },
    { pollMs: LIVE_REFRESH_MS },
  );

  if (detail.loading) return <LoadingState label="Loading route…" />;
  if (detail.error || !detail.data) {
    return (
      <>
        <PageHeader screenId="D-06" title="Route" />
        <ErrorState error={detail.error} onRetry={detail.reload} />
        <div className="mt-4">
          <ButtonLink href="/dispatcher/tracker">Back to routes</ButtonLink>
        </div>
      </>
    );
  }

  const { route, vehicle, outlets, dayRoutes, orders, depotName } = detail.data;
  const f = routeFigures(route, vehicle, dayRoutes);
  const legs = route.legs ?? [];

  return (
    <>
      <PageHeader
        screenId="D-06"
        title={`${route.vehicleId} · trip ${route.tripNo}`}
        description={`${formatDay(route.routeDate)} · ${route.district} · refreshes every 30 s`}
        actions={
          <ButtonLink href="/dispatcher/tracker">← All routes</ButtonLink>
        }
      />

      <div className="grid gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
        <Card className="self-start">
          <div className="flex flex-wrap items-center gap-2">
            <RouteStatusBadge status={route.status} />
            <BrandChip brand={route.brand} />
            <span className="text-xs text-ink-muted">
              Version {route.routeVersion}
            </span>
          </div>
          <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3">
            <Fact label="Vehicle">
              <Mono>{route.vehicleId}</Mono>
              {vehicle && (
                <span className="block text-xs text-ink-muted">
                  {vehicle.tempClass === 'REEFER' ? 'Reefer' : 'Dry-box'}{' '}
                  {humanize(vehicle.type).toLowerCase()}
                </span>
              )}
            </Fact>
            <Fact label="Depot">{depotName ?? '—'}</Fact>
            <Fact label="Trip">Trip {route.tripNo}</Fact>
            <Fact label="District">{route.district}</Fact>
          </dl>
          <div className="mt-4 space-y-3">
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
                  ? 'Vehicle’s Fresh minutes today'
                  : 'Vehicle’s Style + Tech minutes today'
              }
            />
          </div>
          <div className="mt-4 rounded-control bg-page p-3 text-xs text-ink">
            <p className="font-semibold">Trip time (official formula)</p>
            <p className="mt-1 font-mono">
              {route.outboundMin} outbound + {route.interStopMin} between stops
              + {route.handlingMin} handling = {route.totalTripMin} min
            </p>
            <p className="mt-2 font-semibold">Distance and fuel</p>
            <p className="mt-1 font-mono">
              {formatKm(route.distanceKm)}
              {f.fuelLitres !== undefined && vehicle && (
                <>
                  {' '}
                  ÷ {vehicle.kmPerL} km/L ≈ {formatLitres(f.fuelLitres)}
                </>
              )}
            </p>
            {vehicle && (
              <p className="mt-1 text-ink-muted">
                Weekly fuel quota {formatLitres(vehicle.weeklyFuelQuotaL)}. The
                return leg is not counted, matching the supplied route records.
              </p>
            )}
          </div>
        </Card>

        <section aria-labelledby="stops-heading" className="min-w-0">
          <SectionHeading
            aside={
              <span className="text-xs text-ink-muted">
                {f.stopsDelivered} / {f.stopsTotal} delivered
                {f.stopsFailed > 0 && ` · ${f.stopsFailed} failed`}
              </span>
            }
          >
            <span id="stops-heading">
              Stops in delivery order ({plural(legs.length, 'stop')})
            </span>
          </SectionHeading>
          {legs.length === 0 ? (
            <EmptyState title="This route has no stops" />
          ) : (
            <ol className="space-y-2">
              {legs.map((leg, i) => {
                const order = orders.get(leg.orderId);
                const outlet = outlets.get(leg.toOutletId);
                return (
                  <li
                    key={leg.legId}
                    className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-card bg-card p-3 ring-1 ring-ink/10"
                  >
                    <span
                      aria-label={`Stop ${i + 1}`}
                      className="grid size-7 shrink-0 place-items-center rounded-pill bg-action text-sm font-semibold text-card"
                    >
                      {i + 1}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-ink">
                        <Mono>{leg.toOutletId}</Mono>
                        {outlet && <> · {outlet.name}</>}
                      </p>
                      <p className="text-xs text-ink-muted">
                        {order ? (
                          <Link
                            href={`/dispatcher/queue?order=${order.orderId}&days=all`}
                            className="text-link hover:underline"
                          >
                            <Mono>{order.orderNumber}</Mono>
                          </Link>
                        ) : (
                          'Order unavailable'
                        )}
                        {order && (
                          <>
                            {' '}
                            · <Mono>
                              {formatKg(order.totalWeightKg)}
                            </Mono> ·{' '}
                            <Mono>{formatM3(order.totalVolumeM3)}</Mono>
                          </>
                        )}
                        {outlet && (
                          <>
                            {' '}
                            · window{' '}
                            <ClockTimeText value={outlet.windowOpenTime} />–
                            <ClockTimeText value={outlet.windowCloseTime} />
                          </>
                        )}{' '}
                        · <Mono>{formatKm(leg.distanceKm)}</Mono> from{' '}
                        {leg.fromPoint === 'DEPOT' ? (
                          'depot'
                        ) : (
                          <Mono>{leg.fromPoint}</Mono>
                        )}
                      </p>
                    </div>
                    {order && <OrderStatusBadge status={order.status} />}
                    <LegStatusBadge status={leg.status} />
                  </li>
                );
              })}
            </ol>
          )}
        </section>
      </div>
    </>
  );
}
