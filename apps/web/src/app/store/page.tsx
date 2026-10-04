'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import { Mono } from '@waypoint/ui';
import { formatDay } from '../../lib/format';
import { OrderCard } from './_components/order-card';
import { StoreDayLine, useStoreScope } from './_components/store-shell';
import {
  ButtonLink,
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
 * the server to that outlet. A deferred order links to its detail (S-05), which
 * shows the dispatcher's recorded reason and new date — never an invented one.
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
    <div className="flex flex-col gap-6">
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

      <div className="grid grid-cols-3 gap-2 sm:gap-4">
        <Tile
          href="/store/order"
          label="Place an order"
          value="New"
          hint="Before the 16:00 cutoff"
        />
        <Tile
          href="/store/orders?tab=live"
          label="Live orders"
          value={list ? `${list.filter(isOngoingOrder).length}` : '—'}
          hint="Still expected"
        />
        <Tile
          href="/store/orders?tab=deferred"
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
          <div className="grid gap-6 lg:grid-cols-2 lg:items-start">
            <OrderSection
              title="Tomorrow's delivery"
              date={nextDeliveryDay(today)}
              orders={ordersOn(list ?? [], nextDeliveryDay(today))}
              emptyTitle="No orders for tomorrow yet"
              emptyBody="Place an order before the 16:00 cutoff and it appears here straight away."
              action={
                <ButtonLink href="/store/order">Place an order</ButtonLink>
              }
            />
            <OrderSection
              title="Today's delivery"
              date={today}
              orders={ordersOn(list ?? [], today)}
              emptyTitle="Nothing scheduled for today"
              emptyBody="When an order is out for delivery, its status and your arrival window show here."
            />
          </div>
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
      className="flex flex-col rounded-card border border-line bg-card p-3 transition hover:border-line-strong sm:p-4"
    >
      <Eyebrow>{label}</Eyebrow>
      <p className="mt-auto pt-1 font-mono text-xl font-semibold tabular-nums text-ink sm:text-2xl">
        {value}
      </p>
      <p className="mt-0.5 hidden text-xs text-ink-muted sm:block">{hint}</p>
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
      <SectionHeading
        aside={
          <Link
            href="/store/orders?tab=deferred"
            className="text-sm font-medium text-link hover:underline"
          >
            View all
          </Link>
        }
      >
        Deferred orders
      </SectionHeading>
      <p className="text-sm text-ink-muted">
        These could not be served on their planned day. Open one for the reason
        and the run it moved to.
      </p>
      <ul className="grid gap-3 lg:grid-cols-2">
        {orders.map((order) => (
          <li key={order.orderId}>
            <OrderCard order={order} />
          </li>
        ))}
      </ul>
    </section>
  );
}
