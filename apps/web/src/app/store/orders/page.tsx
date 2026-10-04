'use client';

import { useMemo, useState } from 'react';
import { Mono } from '@waypoint/ui';
import { plural } from '../../../lib/format';
import { OrderCard } from '../_components/order-card';
import { StoreDayLine, useStoreScope } from '../_components/store-shell';
import {
  EmptyState,
  ErrorState,
  Eyebrow,
  LoadingState,
} from '../_components/ui';
import { useMyOrders } from '../_lib/use-store';
import {
  completedOrders,
  ongoingOrders,
  readableStoreError,
} from '../_lib/store';

type Tab = 'ongoing' | 'completed';

/**
 * S-07 · My orders.
 *
 * Two views the store actually uses: what is still coming, and what has been
 * closed out in the last seven days. The list is whatever GET /orders returns
 * for the authenticated outlet — no outlet filter is ever sent, so the store
 * manager cannot widen it.
 */
export default function MyOrdersPage() {
  const { meta } = useStoreScope();
  const orders = useMyOrders();
  const [tab, setTab] = useState<Tab>('ongoing');
  const [search, setSearch] = useState('');

  const today = meta?.now.slice(0, 10);
  const all = orders.data?.orders ?? [];

  const ongoing = useMemo(() => ongoingOrders(all), [all]);
  const completed = useMemo(
    () => (today ? completedOrders(all, today) : []),
    [all, today],
  );

  const showing = tab === 'ongoing' ? ongoing : completed;
  const filtered = search.trim()
    ? showing.filter((order) =>
        order.orderNumber.toLowerCase().includes(search.trim().toLowerCase()),
      )
    : showing;

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>My orders</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">Orders</h1>
        <StoreDayLine />
      </header>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-1 rounded-control bg-card p-1 ring-1 ring-ink/10">
          <TabButton
            active={tab === 'ongoing'}
            onClick={() => setTab('ongoing')}
            label={`Live (${ongoing.length})`}
          />
          <TabButton
            active={tab === 'completed'}
            onClick={() => setTab('completed')}
            label={`Completed (${completed.length})`}
          />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <span className="sr-only">Search by order number</span>
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search order number"
            className="h-10 rounded-control bg-card px-3 text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
          />
        </label>
      </div>

      {orders.loading && orders.data === undefined ? (
        <LoadingState label="Loading your orders…" />
      ) : orders.error && orders.data === undefined ? (
        <ErrorState
          title="Couldn't load your orders"
          message={readableStoreError(orders.error, 'Your orders')}
          onRetry={orders.reload}
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          title={
            search.trim()
              ? 'No order matches that number'
              : tab === 'ongoing'
                ? 'No live orders'
                : 'Nothing completed in the last 7 days'
          }
        >
          {search.trim()
            ? 'Clear the search to see every order.'
            : tab === 'ongoing'
              ? 'Orders you place appear here until they are delivered, failed or deferred.'
              : 'Delivered, failed and deferred orders show here for seven days.'}
        </EmptyState>
      ) : (
        <>
          <p className="px-1 text-xs text-ink-muted">
            {plural(filtered.length, 'order')}
            {tab === 'completed' ? ' in the last 7 days' : ''}
          </p>
          <ul className="flex flex-col gap-3">
            {filtered.map((order) => (
              <li key={order.orderId}>
                <OrderCard order={order} />
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  label,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`tap-target rounded-control px-3 text-sm transition ${
        active
          ? 'bg-action font-semibold text-white'
          : 'text-ink-muted hover:text-ink'
      }`}
    >
      <Mono>{label}</Mono>
    </button>
  );
}
