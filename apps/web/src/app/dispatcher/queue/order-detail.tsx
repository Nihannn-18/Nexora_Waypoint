'use client';

import Link from 'next/link';
import { ClockTimeText, Mono, OrderStatusBadge } from '@waypoint/ui';
import type { CustomerOrder, Outlet } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import {
  formatDateTime,
  formatDay,
  formatKg,
  formatM3,
  humanize,
  plural,
} from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import { ErrorState, LoadingState } from '../../../components/states';
import { indexBy, loadItems, loadOutlets } from '../_components/data';
import { BrandChip, ConstraintChip, Fact, TempChip } from '../_components/ui';

/** Statuses at which an order sits on a confirmed route. */
const ON_A_ROUTE = new Set([
  'ALLOCATED',
  'LOADED',
  'IN_TRANSIT',
  'DELIVERED',
  'RECEIVED',
  'FAILED',
]);

/**
 * D-02a — one order in full: where it goes, what is in it, the access and
 * window constraints the planner checks, and what happened to it.
 */
export function OrderDetail({
  orderId,
  onClose,
}: {
  orderId: string;
  onClose?: () => void;
}) {
  const detail = useApiQuery(`order:${orderId}`, async () => {
    const order = await api.getOrder(orderId);
    const [items, outlets] = await Promise.all([loadItems(), loadOutlets()]);
    return {
      order,
      items: indexBy(items, 'itemId'),
      outlet: indexBy(outlets, 'outletId').get(order.outletId),
    };
  });

  if (detail.loading) return <LoadingState label="Loading order…" />;
  if (detail.error || !detail.data) {
    return <ErrorState error={detail.error} onRetry={detail.reload} />;
  }
  const { order, items, outlet } = detail.data;
  const lines = order.lines ?? [];

  return (
    <article
      aria-label={`Order ${order.orderNumber}`}
      className="rounded-card bg-card ring-1 ring-ink/10"
    >
      <header className="border-b border-ink/10 p-4">
        <div className="flex items-start justify-between gap-2">
          <Mono className="rounded-chip bg-ink/[0.06] px-1.5 py-0.5 text-xs font-semibold text-ink">
            {order.orderNumber}
          </Mono>
          {onClose && (
            <button
              type="button"
              onClick={onClose}
              className="tap-target -m-2 grid place-items-center rounded-control text-ink-muted hover:bg-page"
              aria-label="Close order details"
            >
              ✕
            </button>
          )}
        </div>
        <h2 className="mt-2 text-lg font-semibold leading-snug text-ink">
          {outlet?.name ?? order.outletId}
        </h2>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <OrderStatusBadge status={order.status} />
          <BrandChip brand={order.brand} />
          <TempChip temp={order.temperatureRequirement} />
          {order.afterCutoff && (
            <span className="rounded-chip bg-warning/10 px-1.5 py-0.5 text-xs font-medium text-ink ring-1 ring-warning/30">
              ! Placed after the 16:00 cutoff
            </span>
          )}
        </div>
      </header>

      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 bg-brand/5 p-4">
        <Fact label="Destination">
          <Mono>{order.outletId}</Mono>
          {outlet && (
            <span className="block text-xs text-ink-muted">
              {outlet.district}
            </span>
          )}
        </Fact>
        <Fact label="Delivery window">
          {outlet ? (
            <>
              <ClockTimeText value={outlet.windowOpenTime} />–
              <ClockTimeText value={outlet.windowCloseTime} />
            </>
          ) : (
            '—'
          )}
        </Fact>
        <Fact label="Access">
          {outlet ? <AccessText outlet={outlet} /> : '—'}
        </Fact>
        <Fact label="Delivery day">
          {formatDay(order.requestedDeliveryDate)}
          <span className="block text-xs text-ink-muted">
            Ordered {formatDay(order.orderDate)}
          </span>
        </Fact>
      </dl>

      <section className="border-t border-ink/10 p-4" aria-label="Allocation">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
          Allocation
        </h3>
        <AllocationStatus order={order} />
      </section>

      <section className="border-t border-ink/10 p-4" aria-label="Order lines">
        <div className="flex items-baseline justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
            Order lines
          </h3>
          <span className="text-xs text-ink-muted">
            {plural(order.totalUnits, 'unit')} total
          </span>
        </div>
        <div className="mt-2 overflow-x-auto">
          <table className="w-full min-w-[420px] text-left text-sm">
            <thead className="text-[0.6875rem] uppercase tracking-wide text-ink-muted">
              <tr className="border-b border-ink/10">
                <th className="py-1.5 pr-2 font-semibold">SKU</th>
                <th className="py-1.5 pr-2 font-semibold">Item</th>
                <th className="py-1.5 pr-2 text-right font-semibold">Units</th>
                <th className="py-1.5 pr-2 text-right font-semibold">Gross</th>
                <th className="py-1.5 text-right font-semibold">Volume</th>
              </tr>
            </thead>
            <tbody>
              {lines.map((ln) => {
                const item = items.get(ln.itemId);
                return (
                  <tr key={ln.orderItemId} className="border-b border-ink/5">
                    <td className="py-1.5 pr-2">
                      <Mono className="text-xs font-semibold">
                        {item?.sku ?? '—'}
                      </Mono>
                    </td>
                    <td className="py-1.5 pr-2 text-ink">
                      {item?.name ?? 'Unknown item'}
                      {item && item.temperatureRequirement !== 'AMBIENT' && (
                        <span className="ml-1 text-xs text-link">
                          ❄ {humanize(item.temperatureRequirement)}
                        </span>
                      )}
                    </td>
                    <td className="whitespace-nowrap py-1.5 pr-2 text-right">
                      <Mono>{ln.quantity}</Mono>
                    </td>
                    <td className="whitespace-nowrap py-1.5 pr-2 text-right">
                      <Mono>{formatKg(ln.totalWeightKg)}</Mono>
                    </td>
                    <td className="whitespace-nowrap py-1.5 text-right">
                      <Mono>{formatM3(ln.totalVolumeM3)}</Mono>
                    </td>
                  </tr>
                );
              })}
            </tbody>
            <tfoot>
              <tr className="font-semibold text-ink">
                <td className="pt-2" colSpan={2}>
                  Order total
                </td>
                <td className="whitespace-nowrap pt-2 text-right">
                  <Mono>{order.totalUnits}</Mono>
                </td>
                <td className="whitespace-nowrap pt-2 text-right">
                  <Mono>{formatKg(order.totalWeightKg)}</Mono>
                </td>
                <td className="whitespace-nowrap pt-2 text-right">
                  <Mono>{formatM3(order.totalVolumeM3)}</Mono>
                </td>
              </tr>
            </tfoot>
          </table>
        </div>
        <p className="mt-2 text-xs text-ink-muted">
          Totals are computed by the server from the lines, using each SKU’s
          dimensions as they were when the order was placed.
        </p>
        {order.notes && (
          <p className="mt-3 rounded-control bg-page p-2.5 text-sm text-ink">
            <span className="font-semibold">Store note:</span> {order.notes}
          </p>
        )}
      </section>
    </article>
  );
}

function AccessText({ outlet }: { outlet: Outlet }) {
  const dock = humanize(outlet.dockType);
  if (outlet.parkingConstraint === 'VAN_ONLY') {
    return (
      <>
        Van only{' '}
        <span className="block text-xs text-ink-muted">
          {dock} · trucks cannot reach it
        </span>
      </>
    );
  }
  if (outlet.parkingConstraint === 'MALL_DOCK' && outlet.mallWindow) {
    return (
      <>
        Mall dock{' '}
        <span className="block text-xs text-ink-muted">
          Gate open <ClockTimeText value={outlet.mallWindow.open} />–
          <ClockTimeText value={outlet.mallWindow.close} />
        </span>
      </>
    );
  }
  return <>{dock}</>;
}

function AllocationStatus({ order }: { order: CustomerOrder }) {
  const onRoute = ON_A_ROUTE.has(order.status);
  const deferred = order.status === 'DEFERRED';

  const route = useApiQuery(
    onRoute ? `order-route:${order.orderId}` : null,
    async () => {
      const routes = await api.listRoutes({
        date: order.requestedDeliveryDate,
      });
      for (const r of routes) {
        const legs = r.legs ?? [];
        const idx = legs.findIndex((l) => l.orderId === order.orderId);
        if (idx >= 0) return { route: r, stop: idx + 1, stops: legs.length };
      }
      return null;
    },
  );

  const deferral = useApiQuery(
    deferred ? `order-deferral:${order.orderId}` : null,
    async () => {
      const entries = await api.getDeferrals({ outletId: order.outletId });
      // Newest first; the latest deferral of this order is the one in force.
      return entries.find((e) => e.orderId === order.orderId) ?? null;
    },
  );

  if (order.status === 'PLACED') {
    return (
      <p className="mt-1 text-sm text-ink">
        Not confirmed by the store yet, so it is not planned. It joins the next
        planning run once it is confirmed.
      </p>
    );
  }
  if (order.status === 'CONFIRMED') {
    return (
      <p className="mt-1 text-sm text-ink">
        Waiting for planning on {formatDay(order.requestedDeliveryDate)}.{' '}
        <Link
          href="/dispatcher/plan"
          className="font-medium text-link hover:underline"
        >
          Plan this day
        </Link>
      </p>
    );
  }
  if (deferred) {
    if (deferral.loading)
      return (
        <p className="mt-1 text-sm text-ink-muted">Loading the deferral…</p>
      );
    if (deferral.error)
      return (
        <ErrorState error={deferral.error} onRetry={deferral.reload} compact />
      );
    const d = deferral.data;
    if (!d)
      return (
        <p className="mt-1 text-sm text-ink">
          Deferred. No deferral record was found.
        </p>
      );
    return (
      <div className="mt-1 space-y-1.5 text-sm">
        <p className="font-medium text-ink">
          {d.deferredToDate
            ? `Moved to ${formatDay(d.deferredToDate)}`
            : 'Re-enters the next planning run'}
        </p>
        {d.constraintCode && <ConstraintChip code={d.constraintCode} />}
        <p className="text-ink">{d.reasonText}</p>
        <p className="text-xs text-ink-muted">
          Decided {formatDateTime(d.decidedAt)}
          {d.decidedBy && (
            <>
              {' '}
              by <Mono>{d.decidedBy}</Mono>
            </>
          )}
        </p>
      </div>
    );
  }
  if (onRoute) {
    if (route.loading)
      return <p className="mt-1 text-sm text-ink-muted">Finding its route…</p>;
    if (route.error)
      return <ErrorState error={route.error} onRetry={route.reload} compact />;
    const r = route.data;
    if (!r)
      return (
        <p className="mt-1 text-sm text-ink">
          {humanize(order.status)}. Route not found.
        </p>
      );
    return (
      <p className="mt-1 text-sm text-ink">
        On{' '}
        <Link
          href={`/dispatcher/tracker/${r.route.routeId}`}
          className="font-medium text-link hover:underline"
        >
          <Mono>{r.route.vehicleId}</Mono> · trip {r.route.tripNo}
        </Link>
        , stop {r.stop} of {r.stops} · {humanize(order.status)}
      </p>
    );
  }
  return <p className="mt-1 text-sm text-ink">{humanize(order.status)}.</p>;
}
