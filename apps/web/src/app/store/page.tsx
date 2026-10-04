'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import { Mono, OrderStatusBadge } from '@waypoint/ui';
import { formatDay, plural } from '../../lib/format';
import { OrderCard } from './_components/order-card';
import { StoreDayLine, useStoreScope } from './_components/store-shell';
import {
  ButtonLink,
  Card,
  EmptyState,
  ErrorState,
  Eyebrow,
  LoadingState,
  SectionHeading,
} from './_components/ui';
import { useMyOrders } from './_lib/use-store';
import {
  deferredOrders,
  isOngoingOrder,
  nextDeliveryDay,
  ordersOn,
  readableStoreError,
} from './_lib/store';
import type { CustomerOrder } from '@waypoint/shared-types';

/**
 * S-01 · Store home.
 *
 * The store's standing question: is the delivery coming, and when? The outlet
 * shown is the authenticated caller's own, and every order below is scoped by
 * the server to that outlet. A deferred order is shown as deferred; the reason
 * lives on the dispatcher's deferral log, which the store manager cannot read,
 * so the screen never invents one.
 */
export default function StoreHomePage() {
  const { outlet, outletLoading, outletError, reloadOutlet, meta } =
    useStoreScope();
  const orders = useMyOrders();

  if (outletError && !outlet) {
    return (
      <ErrorState
        title="Couldn't load your outlet"
        message={readableStoreError(outletError, 'Your outlet')}
        onRetry={reloadOutlet}
      />
    );
  }
  if (outletLoading && !outlet) {
    return <LoadingState label="Loading your outlet…" />;
  }

  const today = meta?.now.slice(0, 10);
  const list = orders.data?.orders;

  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>Store home</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">
          {outlet ? `Hello, ${outlet.name}` : 'Hello'}
        </h1>
        <StoreDayLine />
        {outlet && (
          <p className="text-sm text-ink-muted">
            Your delivery window is{' '}
            <Mono>
              {outlet.windowOpenTime}–{outlet.windowCloseTime}
            </Mono>
            {outlet.mallWindow
              ? ` · mall access ${outlet.mallWindow.open}–${outlet.mallWindow.close}`
              : ''}
          </p>
        )}
      </header>

      <div className="grid gap-3 sm:grid-cols-3">
        <Tile
          href="/store/order"
          label="Place an order"
          value="New"
          hint="Before the 16:00 cutoff"
        />
        <Tile
          href="/store/orders"
          label="Live orders"
          value={list ? `${list.filter(isOngoingOrder).length}` : '—'}
          hint="Still expected"
        />
        <Tile
          href="/store/orders"
          label="Deferred"
          value={list ? `${deferredOrders(list).length}` : '—'}
          hint="Held for a later run"
        />
      </div>

      {orders.loading && !list ? (
        <LoadingState label="Loading your orders…" />
      ) : orders.error && !list ? (
        <ErrorState
          title="Couldn't load your orders"
          message={readableStoreError(orders.error, 'Your orders')}
          onRetry={orders.reload}
        />
      ) : !today ? (
        <LoadingState label="Syncing the delivery day…" />
      ) : (
        <>
          <OrderSection
            title="Tomorrow's delivery"
            date={nextDeliveryDay(today)}
            orders={ordersOn(list ?? [], nextDeliveryDay(today))}
            emptyTitle="No orders for tomorrow yet"
            emptyBody="Place an order before the 16:00 cutoff and it appears here straight away."
            action={<ButtonLink href="/store/order">Place an order</ButtonLink>}
          />
          <OrderSection
            title="Today's delivery"
            date={today}
            orders={ordersOn(list ?? [], today)}
            emptyTitle="Nothing scheduled for today"
            emptyBody="When an order is out for delivery, its status and your arrival window show here."
          />
          <DeferredSection orders={deferredOrders(list ?? [])} />
        </>
      )}
    </div>
  );
}

function Tile({
  href,
  label,
  value,
  hint,
}: {
  href: string;
  label: string;
  value: string;
  hint: string;
}) {
  return (
    <Link
      href={href}
      className="rounded-card border border-line bg-card p-4 transition hover:border-line-strong"
    >
      <Eyebrow>{label}</Eyebrow>
      <p className="mt-1 font-mono text-2xl font-semibold tabular-nums text-ink">
        {value}
      </p>
      <p className="mt-0.5 text-xs text-ink-muted">{hint}</p>
    </Link>
  );
}

function OrderSection({
  title,
  date,
  orders,
  emptyTitle,
  emptyBody,
  action,
}: {
  title: string;
  date: string;
  orders: readonly CustomerOrder[];
  emptyTitle: string;
  emptyBody: string;
  action?: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3">
      <SectionHeading
        aside={<Mono className="text-xs">{formatDay(date)}</Mono>}
      >
        {title}
      </SectionHeading>
      {orders.length === 0 ? (
        <EmptyState title={emptyTitle} action={action}>
          {emptyBody}
        </EmptyState>
      ) : (
        <ul className="flex flex-col gap-3">
          {orders.map((order) => (
            <li key={order.orderId}>
              <OrderCard order={order} />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function DeferredSection({ orders }: { orders: readonly CustomerOrder[] }) {
  if (orders.length === 0) return null;
  return (
    <section className="flex flex-col gap-3">
      <SectionHeading>Deferred orders</SectionHeading>
      <Card className="gap-2">
        <p className="text-sm text-ink-muted">
          These could not be served on their planned day. The dispatcher’s
          deferral log records the reason and the date they were moved to.
        </p>
        <ul className="flex flex-col divide-y divide-line">
          {orders.map((order) => (
            <li
              key={order.orderId}
              className="flex items-center justify-between gap-2 py-2"
            >
              <span className="flex items-center gap-2">
                <Mono className="text-sm">{order.orderNumber}</Mono>
                <span className="text-xs text-ink-muted">
                  {plural(order.totalUnits, 'unit')}
                </span>
              </span>
              <OrderStatusBadge status={order.status} />
            </li>
          ))}
        </ul>
      </Card>
    </section>
  );
}
