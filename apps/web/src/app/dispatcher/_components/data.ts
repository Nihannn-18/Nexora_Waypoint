import type {
  CustomerOrder,
  Item,
  ListOrdersQuery,
  Outlet,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';

/** The server's largest page. */
const PAGE = 200;

/**
 * Every order matching a filter, fetched page by page. Used where a screen
 * genuinely needs the whole set (a day's plan at one depot is ~200 orders);
 * the Orders screen pages instead.
 */
export async function fetchAllOrders(
  query: Omit<ListOrdersQuery, 'limit' | 'offset'>,
): Promise<CustomerOrder[]> {
  const all: CustomerOrder[] = [];
  for (let offset = 0; ; ) {
    const page = await api.listOrders({ ...query, limit: PAGE, offset });
    all.push(...page.orders);
    offset += page.orders.length;
    if (page.orders.length === 0 || offset >= page.total) return all;
  }
}

/**
 * Reference data (outlets, catalogue) is seeded and does not change during a
 * session, so it is fetched once per key. A failed fetch is not cached.
 */
const referenceCache = new Map<string, Promise<unknown>>();

function cachedReference<T>(key: string, load: () => Promise<T>): Promise<T> {
  let hit = referenceCache.get(key) as Promise<T> | undefined;
  if (!hit) {
    hit = load().catch((error: unknown) => {
      referenceCache.delete(key);
      throw error;
    });
    referenceCache.set(key, hit);
  }
  return hit;
}

export const loadOutlets = (depotId?: string): Promise<readonly Outlet[]> =>
  cachedReference(`outlets:${depotId ?? '*'}`, () =>
    api.getOutlets({ depotId }),
  );

export const loadItems = (): Promise<readonly Item[]> =>
  cachedReference('items', () => api.listItems());

export function indexBy<T, K extends keyof T>(
  list: readonly T[],
  key: K,
): Map<T[K], T> {
  return new Map(list.map((row) => [row[key], row]));
}
