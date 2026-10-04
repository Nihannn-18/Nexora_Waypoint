'use client';

import type {
  CustomerOrder,
  Item,
  Outlet,
  ReceiptView,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { useApiQuery, type ApiQuery } from '../../../lib/use-api-query';

/**
 * Store-manager data access. Every query goes through the shared API client,
 * which attaches the Go bearer session; the server derives the caller's outlet
 * from that session, so none of these requests name an outlet or depot of their
 * own. That is the point: scope is the server's decision, not the screen's.
 */

/**
 * The caller's own outlet. GET /outlets is pinned server-side to the store
 * manager's `app_user.outlet_id`, so the array holds at most the one outlet
 * their identity may access — a query parameter cannot widen it.
 */
export function useStoreOutlet(): ApiQuery<readonly Outlet[]> {
  return useApiQuery('store-own-outlet', () => api.getOutlets());
}

/** The API clock and demo mode; countdowns derive from it, never the browser. */
export function useApiClock() {
  return useApiQuery('store-meta', () => api.meta(), { pollMs: 60_000 });
}

/**
 * Every order for the caller's outlet, newest first. Deliberately no
 * `outletId`: the server pins the result. 200 is the API's page cap, which is
 * more than one outlet accumulates over the demo window.
 */
export function useMyOrders(): ApiQuery<{
  readonly orders: readonly CustomerOrder[];
  readonly total: number;
}> {
  return useApiQuery('store-my-orders', () => api.listOrders({ limit: 200 }));
}

/** One order, restricted server-side to the caller's outlet. */
export function useMyOrder(orderId: string | null): ApiQuery<CustomerOrder> {
  return useApiQuery(orderId ? `store-order-${orderId}` : null, () =>
    api.getOrder(orderId as string),
  );
}

/** The catalogue for the outlet's brand, used to build an order. */
export function useOutletItems(
  brand: string | null,
): ApiQuery<readonly Item[]> {
  return useApiQuery(brand ? `store-items-${brand}` : null, () =>
    api.listItems({ brand: brand as string }),
  );
}

/**
 * S-06: one order's GRN view — expected lines with the loader's flags, the
 * driver's POD and the receipt once recorded. Scoped server-side to the
 * caller's outlet.
 */
export function useOrderReceipt(orderId: string | null): ApiQuery<ReceiptView> {
  return useApiQuery(orderId ? `store-receipt-${orderId}` : null, () =>
    api.getReceipt(orderId as string),
  );
}
