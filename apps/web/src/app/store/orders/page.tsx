'use client';

import { useEffect, useMemo, useState } from 'react';
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
  deferredOrders,
  ongoingOrders,
  readableStoreError,
} from '../_lib/store';

type Tab = 'ongoing' | 'deferred' | 'completed';

/** `?tab=` values, so Store home's tiles open the view they name. */
const TAB_PARAM: Record<string, Tab> = {
  live: 'ongoing',
  deferred: 'deferred',
  completed: 'completed',
};
const PARAM_OF: Record<Tab, string> = {
  ongoing: 'live',
  deferred: 'deferred',
  completed: 'completed',
};

/**
 * S-07 · My orders.
 *
 * Three views of one list: what is still coming, what was deferred to a later
 * run (and still has to arrive, so it is not "completed"), and what reached an
 * outcome in the last seven days. Deferred is a filter of this list rather than
 * its own screen — it is the same orders and the same API. The list is whatever
 * GET /orders returns for the authenticated outlet — no outlet filter is ever
 * sent, so the store manager cannot widen it.
 */
export default function MyOrdersPage() {
  const { meta } = useStoreScope();
  const orders = useMyOrders();
  const [tab, setTabState] = useState<Tab>('ongoing');
  const [search, setSearch] = useState('');

  // Read ?tab= once on mount (a plain read keeps the page statically
  // renderable), and keep the URL in step so refresh and back restore the view.
  useEffect(() => {
    const param = new URLSearchParams(window.location.search).get('tab');
    const initial = param ? TAB_PARAM[param] : undefined;
    if (initial) setTabState(initial);
  }, []);
  const setTab = (next: Tab) => {
    setTabState(next);
    const url = new URL(window.location.href);
    url.searchParams.set('tab', PARAM_OF[next]);
    window.history.replaceState(window.history.state, '', url);
  };

  const today = meta?.now.slice(0, 10);
  const all = orders.data?.orders ?? [];

  const ongoing = useMemo(() => ongoingOrders(all), [all]);
  const deferred = useMemo(() => deferredOrders(all), [all]);
  const completed = useMemo(
    () =>
      today
        ? completedOrders(all, today).filter((o) => o.status !== 'DEFERRED')
        : [],
    [all, today],
  );

  const showing =
    tab === 'ongoing' ? ongoing : tab === 'deferred' ? deferred : completed;
  const filtered = search.trim()
    ? showing.filter((order) =>
        order.orderNumber.toLowerCase().includes(search.trim().toLowerCase()),
      )
    : showing;

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>My orders</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">
          {tab === 'deferred' ? 'Deferred orders' : 'Orders'}
        </h1>
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
            active={tab === 'deferred'}
            onClick={() => setTab('deferred')}
            label={`Deferred (${deferred.length})`}
          />
          <TabButton
            active={tab === 'completed'}
            onClick={() => setTab('completed')}
            label={`Completed (${completed.length})`}
          />
        </div>
        <label className="flex w-full items-center gap-2 text-sm sm:w-auto">
          <span className="sr-only">Search by order number</span>
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search order number"
            className="h-10 w-full rounded-control bg-card px-3 text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand sm:w-64"
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
                : tab === 'deferred'
                  ? 'No deferred orders'
                  : 'Nothing completed in the last 7 days'
          }
        >
          {search.trim()
            ? 'Clear the search to see every order.'
            : tab === 'ongoing'
              ? 'Orders you place appear here until they are delivered, failed or deferred.'
              : tab === 'deferred'
                ? 'When the dispatcher cannot fit an order on its day, it shows here with the reason and the run it moved to.'
                : 'Delivered and failed orders show here for seven days.'}
        </EmptyState>
      ) : (
        <>
          <p className="px-1 text-xs text-ink-muted">
            {plural(filtered.length, 'order')}
            {tab === 'completed'
              ? ' in the last 7 days'
              : tab === 'deferred'
                ? ' held for a later run'
                : ''}
          </p>
          <ul className="grid gap-3 lg:grid-cols-2">
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
