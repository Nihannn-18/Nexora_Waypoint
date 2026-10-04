'use client';

import Link from 'next/link';
import { Mono, OrderStatusBadge } from '@waypoint/ui';
import type { CustomerOrder } from '@waypoint/shared-types';
import { formatDay, formatKg, formatM3, plural } from '../../../lib/format';
import { BrandChip, Card, TempChip } from './ui';

/**
 * One order, as a store manager scans it. Every figure comes from the order
 * record the server returned for the caller's outlet; nothing is inferred.
 *
 * A deferred order is called out explicitly, because "will it come?" is the
 * question the store actually has: the card leads with the run it moved to when
 * the server returned the deferral, and never invents a reason.
 */
export function OrderCard({ order }: { order: CustomerOrder }) {
  return (
    <Link
      href={`/store/orders/${order.orderId}`}
      className="block rounded-card focus:outline-2 focus:outline-offset-2 focus:outline-action"
    >
      <Card as="article" className="gap-2">
        <div className="flex items-start justify-between gap-2">
          <div className="flex flex-col gap-1">
            <div className="flex flex-wrap items-center gap-2">
              <Mono className="text-sm font-semibold text-ink">
                {order.orderNumber}
              </Mono>
              <BrandChip brand={order.brand} />
              <TempChip temp={order.temperatureRequirement} />
            </div>
            <p className="text-sm text-ink-muted">
              Delivery <Mono>{formatDay(order.requestedDeliveryDate)}</Mono>
            </p>
          </div>
          <OrderStatusBadge status={order.status} />
        </div>

        <p className="text-sm text-ink-muted">
          <Mono>{plural(order.totalUnits, 'unit')}</Mono> ·{' '}
          <Mono>{formatKg(order.totalWeightKg)}</Mono> ·{' '}
          <Mono>{formatM3(order.totalVolumeM3)}</Mono>
          {order.lines ? ` · ${plural(order.lines.length, 'line')}` : ''}
        </p>

        {order.status === 'DEFERRED' && (
          <p className="rounded-control bg-warning-bg px-3 py-2 text-xs font-medium text-warning-ink">
            {order.deferral?.deferredToDate ? (
              <>
                Moved to <Mono>{formatDay(order.deferral.deferredToDate)}</Mono>{' '}
                — open for the reason.
              </>
            ) : (
              'Not on a route this day — open for the reason and new date.'
            )}
          </p>
        )}
        {order.afterCutoff && order.status === 'PLACED' && (
          <p className="rounded-control bg-page px-3 py-2 text-xs text-ink-muted">
            Placed after the 16:00 cutoff — accepted for the next operating run.
          </p>
        )}
      </Card>
    </Link>
  );
}
