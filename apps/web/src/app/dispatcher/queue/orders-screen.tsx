'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useMemo, useState } from 'react';
import {
  ClockTimeText,
  Mono,
  OrderStatusBadge,
  formatOrderStatus,
} from '@waypoint/ui';
import {
  BRANDS,
  ORDER_STATUSES,
  type Brand,
  type CustomerOrder,
  type OrderStatus,
  type Outlet,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { formatDay, formatKg, formatM3 } from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { indexBy, loadOutlets } from '../_components/data';
import {
  BrandChip,
  PageHeader,
  TempChip,
  buttonClass,
} from '../_components/ui';
import { OrderDetail } from './order-detail';

const PAGE_SIZE = 25;

/**
 * D-02 — every order for the depot, filtered and paged by the API. The total
 * shown is the server's count for the same filter, so "of 186" is never a
 * guess from what happens to be loaded.
 */
export function OrdersScreen() {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const { depot, deliveryDate } = useDispatcherScope();

  const selected = params.get('order');
  const status = params.get('status') as OrderStatus | null;
  const brand = params.get('brand') as Brand | null;
  const district = params.get('district');
  const search = params.get('q') ?? '';
  const allDays = params.get('days') === 'all';
  const page = Math.max(0, Number(params.get('page') ?? '0') || 0);

  const setParams = (
    updates: Record<string, string | null>,
    resetPage = true,
  ) => {
    const next = new URLSearchParams(params.toString());
    for (const [k, v] of Object.entries(updates)) {
      if (v === null || v === '') next.delete(k);
      else next.set(k, v);
    }
    if (resetPage) next.delete('page');
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  };

  // Debounce the search box so typing does not fire a request per keystroke.
  const [searchDraft, setSearchDraft] = useState(search);
  useEffect(() => setSearchDraft(search), [search]);
  useEffect(() => {
    if (searchDraft === search) return;
    const id = window.setTimeout(
      () => setParams({ q: searchDraft.trim() || null }),
      300,
    );
    return () => window.clearTimeout(id);
    // Only the draft changing should restart the debounce.
  }, [searchDraft]);

  const outlets = useApiQuery(depot ? `outlets:${depot.depotId}` : null, () =>
    loadOutlets(depot?.depotId),
  );
  const outletById = useMemo(
    () => indexBy(outlets.data ?? [], 'outletId') as Map<string, Outlet>,
    [outlets.data],
  );
  const districts = useMemo(
    () => [...new Set((outlets.data ?? []).map((o) => o.district))].sort(),
    [outlets.data],
  );

  const query = {
    depotId: depot?.depotId,
    deliveryDate: allDays ? undefined : deliveryDate,
    status: status ?? undefined,
    brand: brand ?? undefined,
    district: district ?? undefined,
    search: search || undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  };
  const ready = depot && (allDays || deliveryDate);
  const list = useApiQuery(
    ready ? `orders:${JSON.stringify(query)}` : null,
    () => api.listOrders(query),
  );

  const total = list.data?.total ?? 0;
  const first = total === 0 ? 0 : page * PAGE_SIZE + 1;
  const last = Math.min(total, (page + 1) * PAGE_SIZE);
  const filtered = Boolean(status || brand || district || search);

  return (
    <>
      <PageHeader
        screenId="D-02"
        title="Orders"
        description={
          <>
            {depot?.name ?? 'Depot'} ·{' '}
            {allDays
              ? 'all delivery days'
              : deliveryDate
                ? formatDay(deliveryDate)
                : '…'}
            . Only CONFIRMED orders are planned; PLACED orders wait for the
            store to confirm.
          </>
        }
      />

      <div className="mb-4 flex flex-wrap items-end gap-3 rounded-card bg-card p-3 ring-1 ring-ink/10">
        <label className="flex min-w-56 flex-1 flex-col gap-1 text-xs font-medium text-ink-muted">
          Search
          <input
            type="search"
            value={searchDraft}
            onChange={(e) => setSearchDraft(e.target.value)}
            placeholder="Order number or outlet id"
            className="h-10 rounded-control bg-page px-3 text-sm text-ink ring-1 ring-ink/15"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
          Status
          <select
            value={status ?? ''}
            onChange={(e) => setParams({ status: e.target.value || null })}
            className="h-10 rounded-control bg-page px-2 text-sm text-ink ring-1 ring-ink/15"
          >
            <option value="">All statuses</option>
            {ORDER_STATUSES.map((s) => (
              <option key={s} value={s}>
                {formatOrderStatus(s)}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
          District
          <select
            value={district ?? ''}
            onChange={(e) => setParams({ district: e.target.value || null })}
            className="h-10 rounded-control bg-page px-2 text-sm text-ink ring-1 ring-ink/15"
          >
            <option value="">All districts</option>
            {districts.map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>
        </label>
        <label className="flex h-10 items-center gap-2 text-sm text-ink">
          <input
            type="checkbox"
            checked={allDays}
            onChange={(e) =>
              setParams({ days: e.target.checked ? 'all' : null })
            }
            className="size-4"
          />
          All delivery days
        </label>
        <div role="group" aria-label="Brand" className="flex gap-1">
          {[null, ...BRANDS].map((b) => (
            <button
              key={b ?? 'all'}
              type="button"
              aria-pressed={brand === b}
              onClick={() => setParams({ brand: b })}
              className={`h-10 rounded-control px-3 text-xs font-medium ring-1 ${
                brand === b
                  ? 'bg-action text-card ring-action'
                  : 'bg-card text-ink ring-ink/15 hover:bg-page'
              }`}
            >
              {b ? `${b.charAt(0)}${b.slice(1).toLowerCase()}` : 'All brands'}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section aria-label="Order list" className="min-w-0">
          <div className="mb-2 flex items-center justify-between text-sm text-ink-muted">
            <span aria-live="polite">
              {list.data ? (
                total === 0 ? (
                  'No orders'
                ) : (
                  <>
                    Showing <Mono>{first}</Mono>–<Mono>{last}</Mono> of{' '}
                    <Mono>{total}</Mono>
                  </>
                )
              ) : (
                ' '
              )}
            </span>
            {list.refreshing && list.data && (
              <span className="text-xs">Updating…</span>
            )}
          </div>

          {list.loading ? (
            <LoadingState label="Loading orders…" />
          ) : list.error ? (
            <ErrorState error={list.error} onRetry={list.reload} />
          ) : total === 0 ? (
            <EmptyState
              title={
                filtered
                  ? 'No orders match these filters'
                  : 'No orders for this day yet'
              }
              action={
                filtered ? (
                  <button
                    type="button"
                    className={buttonClass.secondary}
                    onClick={() =>
                      setParams({
                        status: null,
                        brand: null,
                        district: null,
                        q: null,
                      })
                    }
                  >
                    Clear filters
                  </button>
                ) : undefined
              }
            >
              {filtered
                ? 'Try a wider filter, or tick “All delivery days”.'
                : 'Stores order before 16:00 for the next operating day. Orders appear here as they are placed.'}
            </EmptyState>
          ) : (
            <>
              <ul className="space-y-2">
                {list.data?.orders.map((o) => (
                  <OrderRow
                    key={o.orderId}
                    order={o}
                    outlet={outletById.get(o.outletId)}
                    selected={o.orderId === selected}
                    onSelect={() => setParams({ order: o.orderId }, false)}
                  />
                ))}
              </ul>
              <nav
                aria-label="Pages"
                className="mt-3 flex items-center justify-between gap-2"
              >
                <button
                  type="button"
                  className={buttonClass.secondary}
                  disabled={page === 0}
                  onClick={() => setParams({ page: String(page - 1) }, false)}
                >
                  ← Previous
                </button>
                <span className="text-xs text-ink-muted">
                  Page {page + 1} of {Math.max(1, Math.ceil(total / PAGE_SIZE))}
                </span>
                <button
                  type="button"
                  className={buttonClass.secondary}
                  disabled={last >= total}
                  onClick={() => setParams({ page: String(page + 1) }, false)}
                >
                  Next →
                </button>
              </nav>
            </>
          )}
        </section>

        {/* Below xl the panel stacks; put a selected order above the list. */}
        <section
          aria-label="Order details"
          className={`min-w-0 ${selected ? 'order-first xl:order-none' : ''}`}
        >
          {selected ? (
            <div className="xl:sticky xl:top-24">
              <OrderDetail
                orderId={selected}
                onClose={() => setParams({ order: null }, false)}
              />
            </div>
          ) : (
            <div className="hidden xl:block">
              <EmptyState title="Select an order">
                Its lines, delivery window, access constraints and allocation
                appear here.
              </EmptyState>
            </div>
          )}
        </section>
      </div>
    </>
  );
}

function OrderRow({
  order,
  outlet,
  selected,
  onSelect,
}: {
  order: CustomerOrder;
  outlet: Outlet | undefined;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className={`w-full rounded-card p-3 text-left ring-1 transition ${
          selected
            ? 'bg-brand/10 ring-brand'
            : 'bg-card ring-ink/10 hover:ring-brand/50'
        }`}
      >
        <span className="flex flex-wrap items-center gap-2">
          <Mono className="font-semibold text-link">{order.orderNumber}</Mono>
          <BrandChip brand={order.brand} />
          <TempChip temp={order.temperatureRequirement} />
          {order.afterCutoff && (
            <span className="text-xs font-medium text-warning">
              ! After cutoff
            </span>
          )}
          <span className="ml-auto">
            <OrderStatusBadge status={order.status} />
          </span>
        </span>
        <span className="mt-1 block text-sm text-ink">
          <Mono>{order.outletId}</Mono>
          {outlet && (
            <>
              {' '}
              · {outlet.name}{' '}
              <span className="text-ink-muted">· {outlet.district}</span>
            </>
          )}
        </span>
        <span className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-ink-muted">
          <span>
            <Mono className="text-ink">{formatKg(order.totalWeightKg)}</Mono> ·{' '}
            <Mono className="text-ink">{formatM3(order.totalVolumeM3)}</Mono>
          </span>
          {outlet && (
            <span>
              Window{' '}
              <ClockTimeText
                value={outlet.windowOpenTime}
                className="text-ink"
              />
              –
              <ClockTimeText
                value={outlet.windowCloseTime}
                className="text-ink"
              />
            </span>
          )}
          {outlet?.parkingConstraint === 'VAN_ONLY' && (
            <span>Van-only access</span>
          )}
          {outlet?.mallWindow && (
            <span>
              Mall {outlet.mallWindow.open}–{outlet.mallWindow.close}
            </span>
          )}
          <span>For {formatDay(order.requestedDeliveryDate)}</span>
        </span>
      </button>
    </li>
  );
}
