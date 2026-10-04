'use client';

import Link from 'next/link';
import { use } from 'react';
import { Mono, OrderStatusBadge } from '@waypoint/ui';
import { formatDay, formatKg, formatM3, plural } from '../../../../lib/format';
import { useStoreScope } from '../../_components/store-shell';
import {
  BrandChip,
  Card,
  ErrorState,
  Eyebrow,
  Fact,
  LoadingState,
  TempChip,
} from '../../_components/ui';
import { useMyOrder } from '../../_lib/use-store';
import { readableStoreError } from '../../_lib/store';

/**
 * S-04 / S-05 · One order.
 *
 * Status, lines and totals are the order the server returned for the caller's
 * outlet. The arrival window shown is the outlet's own delivery window, which
 * is real reference data; the API does not expose a per-order ETA, a deferral
 * reason or a receipt to the store manager yet, so the screen does not fake
 * any of them.
 */
export default function OrderDetailPage({
  params,
}: {
  params: Promise<{ orderId: string }>;
}) {
  const { orderId } = use(params);
  const state = useMyOrder(orderId);
  const { outlet } = useStoreScope();

  if (state.loading && !state.data) {
    return <LoadingState label="Loading this order…" />;
  }
  if (state.error && !state.data) {
    return (
      <ErrorState
        title="Couldn't open this order"
        message={readableStoreError(state.error, 'This order')}
        onRetry={state.reload}
      />
    );
  }
  const order = state.data;
  if (!order) {
    return (
      <ErrorState
        title="Couldn't open this order"
        message="This order was not found for your outlet."
        onRetry={state.reload}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Link href="/store/orders" className="text-sm font-medium text-link">
        ← All orders
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex flex-col gap-1">
          <Eyebrow>Order</Eyebrow>
          <h1 className="text-xl font-semibold text-ink">
            <Mono>{order.orderNumber}</Mono>
          </h1>
          <div className="flex flex-wrap items-center gap-2">
            <BrandChip brand={order.brand} />
            <TempChip temp={order.temperatureRequirement} />
          </div>
        </div>
        <OrderStatusBadge status={order.status} />
      </header>

      {order.status === 'DEFERRED' && <DeferredNotice />}
      {order.afterCutoff && (
        <p className="rounded-card border border-warning bg-warning-bg p-3 text-sm text-warning-ink">
          Placed after the 16:00 cutoff. It was accepted for the next operating
          run rather than rejected.
        </p>
      )}

      <Card as="section">
        <h2 className="text-sm font-semibold text-ink">Delivery</h2>
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <Fact label="Delivery day">
            <Mono>{formatDay(order.requestedDeliveryDate)}</Mono>
          </Fact>
          <Fact label="Ordered on">
            <Mono>{formatDay(order.orderDate)}</Mono>
          </Fact>
          <Fact label="Units">
            <Mono>{order.totalUnits}</Mono>
          </Fact>
          <Fact label="Your window">
            {outlet ? (
              <Mono>
                {outlet.windowOpenTime}–{outlet.windowCloseTime}
              </Mono>
            ) : (
              <span className="text-ink-muted">—</span>
            )}
          </Fact>
        </dl>
        {outlet?.mallWindow && (
          <p className="text-xs text-ink-muted">
            Mall access is fixed at{' '}
            <Mono>
              {outlet.mallWindow.open}–{outlet.mallWindow.close}
            </Mono>
            .
          </p>
        )}
        <p className="text-xs text-ink-muted">
          Weight <Mono>{formatKg(order.totalWeightKg)}</Mono> · volume{' '}
          <Mono>{formatM3(order.totalVolumeM3)}</Mono>
          {order.notes ? ` · note: ${order.notes}` : ''}
        </p>
      </Card>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-ink">
          {plural(order.lines?.length ?? 0, 'line')}
        </h2>
        {order.lines && order.lines.length > 0 ? (
          <Card className="gap-0 p-0">
            <ul className="flex flex-col divide-y divide-line">
              {order.lines.map((line) => (
                <li
                  key={line.orderItemId}
                  className="flex flex-wrap items-center justify-between gap-2 p-4"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-ink">{line.name}</p>
                    <p className="text-xs text-ink-muted">
                      <Mono>{line.sku}</Mono>
                    </p>
                  </div>
                  <div className="text-right">
                    <p className="font-mono text-sm font-semibold tabular-nums text-ink">
                      ×{line.quantity}
                    </p>
                    <p className="text-xs text-ink-muted">
                      <Mono>{formatKg(line.totalWeightKg)}</Mono> ·{' '}
                      <Mono>{formatM3(line.totalVolumeM3)}</Mono>
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          </Card>
        ) : (
          <Card>
            <p className="text-sm text-ink-muted">
              No line detail was returned for this order.
            </p>
          </Card>
        )}
      </section>

      <DeliveryNote status={order.status} />
    </div>
  );
}

/**
 * S-05's notice, honestly scoped. The store cannot yet read the dispatcher's
 * deferral reason or acknowledge it through the API, so this states the fact
 * and where the reason lives instead of inventing either.
 */
function DeferredNotice() {
  return (
    <Card className="gap-1 border-warning bg-warning-bg">
      <h2 className="text-sm font-semibold text-warning-ink">
        This order was deferred
      </h2>
      <p className="text-sm text-ink">
        It could not be served on its planned day and is held for a later run.
        The dispatcher’s deferral log holds the reason and the new date.
      </p>
    </Card>
  );
}

/**
 * What a delivery outcome means for the store today. A receipt (GRN) and the
 * driver's proof of delivery are not exposed to the store manager by the API
 * yet, so the screen says so rather than showing an empty receipt form.
 */
function DeliveryNote({ status }: { status: string }) {
  if (status !== 'DELIVERED' && status !== 'RECEIVED' && status !== 'FAILED') {
    return null;
  }
  const copy =
    status === 'FAILED'
      ? 'The driver recorded this delivery as failed. The dispatcher can re-plan the stop or defer it.'
      : status === 'RECEIVED'
        ? 'This delivery has been received.'
        : 'The driver recorded this delivery. Receipt confirmation and proof of delivery are not yet exposed to the store manager.';
  return (
    <Card>
      <p className="text-sm text-ink-muted">{copy}</p>
    </Card>
  );
}
