'use client';

import Link from 'next/link';
import { use, useState } from 'react';
import { Mono, OrderStatusBadge } from '@waypoint/ui';
import type { CustomerOrder } from '@waypoint/shared-types';
import { formatDay, formatKg, formatM3, plural } from '../../../../lib/format';
import { api } from '../../../../lib/api';
import { useStoreScope } from '../../_components/store-shell';
import {
  BrandChip,
  ButtonLink,
  Card,
  ErrorState,
  Eyebrow,
  Fact,
  LoadingState,
  TempChip,
  buttonClass,
} from '../../_components/ui';
import { useMyOrder } from '../../_lib/use-store';
import { readableStoreError } from '../../_lib/store';

/**
 * S-04 / S-05 · One order.
 *
 * Status, lines and totals are the order the server returned for the caller's
 * outlet. The arrival window shown is the outlet's own delivery window, which
 * is real reference data; the API does not expose a per-order ETA, so the
 * screen does not fake one. A delivered order links to its GRN (S-06).
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

      {order.status === 'DEFERRED' && <DeferredNotice order={order} />}
      {order.status === 'PLACED' && (
        <ConfirmOrderAction order={order} onConfirmed={state.reload} />
      )}
      {order.status === 'CONFIRMED' && <ConfirmedNotice />}
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

      <DeliveryNote orderId={order.orderId} status={order.status} />
    </div>
  );
}

/**
 * S-03/S-04 · Explicit confirmation.
 *
 * A placed order is not yet part of the planning queue. The store manager must
 * confirm it deliberately; only then does the order move PLACED → CONFIRMED and
 * become visible to the dispatcher's queue and planning run. The button is
 * disabled while the request is in flight so a double click cannot submit twice,
 * and the success copy never claims the order is already planned — planning
 * remains the dispatcher's job.
 */
function ConfirmOrderAction({
  order,
  onConfirmed,
}: {
  order: { orderId: string; requestedDeliveryDate: string };
  onConfirmed: () => void;
}) {
  const [status, setStatus] = useState<'idle' | 'confirming' | 'confirmed'>(
    'idle',
  );
  const [error, setError] = useState<string | null>(null);

  async function confirm() {
    setStatus('confirming');
    setError(null);
    try {
      await api.confirmOrder(order.orderId);
      setStatus('confirmed');
      onConfirmed();
    } catch (err) {
      setStatus('idle');
      setError(readableStoreError(err, 'This order'));
    }
  }

  if (status === 'confirmed') {
    return (
      <Card className="gap-1 border-success bg-success-bg">
        <h2 className="text-sm font-semibold text-success">Order confirmed</h2>
        <p className="text-sm text-ink">
          The dispatcher can now include it in planning for{' '}
          <Mono>{formatDay(order.requestedDeliveryDate)}</Mono>. Its status
          becomes ALLOCATED once a plan is confirmed.
        </p>
      </Card>
    );
  }

  return (
    <Card className="gap-3 border-brand bg-info-bg">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold text-ink">Confirm this order</h2>
        <p className="text-sm text-ink-muted">
          An order is not planned until you confirm it. Confirm to send it to
          the dispatcher’s queue for planning on{' '}
          <Mono>{formatDay(order.requestedDeliveryDate)}</Mono>.
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-error-strong">
          {error}
        </p>
      )}
      <button
        type="button"
        onClick={confirm}
        disabled={status === 'confirming'}
        className={buttonClass('ink', 'w-full sm:w-auto')}
      >
        {status === 'confirming' ? 'Confirming…' : 'Confirm order'}
      </button>
    </Card>
  );
}

/** A confirmed order is queued for planning; it is not yet allocated. */
function ConfirmedNotice() {
  return (
    <Card className="gap-1 border-success bg-success-bg">
      <h2 className="text-sm font-semibold text-success">Order confirmed</h2>
      <p className="text-sm text-ink">
        It is in the dispatcher’s planning queue. It becomes allocated once the
        dispatcher confirms a plan.
      </p>
    </Card>
  );
}

/**
 * S-05's notice. When the server returns the deferral the dispatcher recorded,
 * the store sees the reason, the binding constraint's code and the run it moved
 * to. A deferral record that has not reached the read model yet is shown as a
 * fact with a pointer to the log, never an invented reason.
 */
function DeferredNotice({ order }: { order: CustomerOrder }) {
  const deferral = order.deferral;
  return (
    <Card className="gap-1 border-warning bg-warning-bg">
      {/* S-05: the fact first, then the reason, then who/when. */}
      <h2 className="text-base font-semibold text-warning-ink">
        {deferral?.deferredToDate ? (
          <>
            Moved to <Mono>{formatDay(deferral.deferredToDate)}</Mono>
          </>
        ) : (
          'This order was deferred'
        )}
      </h2>
      {deferral ? (
        <>
          <p className="text-sm text-ink">{deferral.reasonText}</p>
          <p className="text-xs text-ink-muted">
            {deferral.constraintCode ? (
              <>
                Blocking rule <Mono>{deferral.constraintCode}</Mono> ·{' '}
              </>
            ) : null}
            decided by the dispatcher on{' '}
            <Mono>{formatDay(deferral.decidedAt.slice(0, 10))}</Mono>
          </p>
        </>
      ) : (
        <p className="text-sm text-ink">
          It could not be served on its planned day and is held for a later run.
          The dispatcher’s deferral log holds the reason and the new date.
        </p>
      )}
    </Card>
  );
}

/**
 * What a delivery outcome means for the store. A delivered order leads to its
 * goods received note (S-06); a received one to the recorded GRN (S-06b).
 */
function DeliveryNote({
  orderId,
  status,
}: {
  orderId: string;
  status: string;
}) {
  if (status === 'FAILED') {
    return (
      <Card>
        <p className="text-sm text-ink-muted">
          The driver recorded this delivery as failed. The dispatcher can
          re-plan the stop or defer it.
        </p>
      </Card>
    );
  }
  if (status === 'DELIVERED') {
    return (
      <Card className="gap-3 border-brand bg-info-bg">
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-semibold text-ink">
            Delivered — confirm what arrived
          </h2>
          <p className="text-sm text-ink-muted">
            Count each line against what left the depot and report anything
            damaged or short. The driver’s proof of delivery is attached.
          </p>
        </div>
        <ButtonLink href={`/store/orders/${orderId}/receipt`} variant="ink">
          Open the GRN
        </ButtonLink>
      </Card>
    );
  }
  if (status === 'RECEIVED') {
    return (
      <Card className="gap-3">
        <p className="text-sm text-ink-muted">
          This delivery has been received.
        </p>
        <ButtonLink href={`/store/orders/${orderId}/receipt`}>
          View the GRN
        </ButtonLink>
      </Card>
    );
  }
  return null;
}
