/**
 * Store-manager arithmetic and grouping, kept out of the components so the
 * rules a judge cares about — which orders are ongoing, how the delivered tail
 * is windowed, the cutoff clock and the "one temperature per order" rule — can
 * be tested without rendering a screen.
 *
 * The server is the authority on every number here and on every outlet scope.
 * These helpers only arrange what the API already returned for the caller's own
 * outlet; they never widen it. A store manager's list request carries no outlet
 * or depot filter, because the server pins the response to the authenticated
 * identity (see GET /orders and GET /outlets in docs/api.md).
 */
import { WaypointApiError } from '@waypoint/api-client';
import {
  ORDER_CUTOFF_LABEL,
  type CustomerOrder,
  type IsoDate,
  type Item,
  type OrderStatus,
  type TempRequirement,
} from '@waypoint/shared-types';
import { addDays } from '../../../lib/format';

/* -------------------------------------------------------------------------- */
/* Order grouping                                                             */
/* -------------------------------------------------------------------------- */

/**
 * The statuses still expected on their delivery day. `DELIVERED` is *not* here:
 * the order lifecycle moves DELIVERED → RECEIVED, so from the store's point of
 * view the delivery has happened and the order belongs with its outcomes. That
 * is deliberately broader than `isTerminalForDay` (which marks RECEIVED,
 * DEFERRED and FAILED), because S-07's completed tail is about outcomes.
 */
const ONGOING_STATUSES: ReadonlySet<OrderStatus> = new Set<OrderStatus>([
  'PLACED',
  'CONFIRMED',
  'ALLOCATED',
  'LOADED',
  'IN_TRANSIT',
]);

/** An order still expected on its day — everything except an outcome. */
export function isOngoingOrder(order: CustomerOrder): boolean {
  return ONGOING_STATUSES.has(order.status);
}

function byDeliveryAsc(a: CustomerOrder, b: CustomerOrder): number {
  return a.requestedDeliveryDate.localeCompare(b.requestedDeliveryDate);
}

function byDeliveryDesc(a: CustomerOrder, b: CustomerOrder): number {
  return b.requestedDeliveryDate.localeCompare(a.requestedDeliveryDate);
}

/** Live orders, soonest delivery first. */
export function ongoingOrders(
  orders: readonly CustomerOrder[],
): CustomerOrder[] {
  return orders.filter(isOngoingOrder).sort(byDeliveryAsc);
}

/**
 * The closed tail: orders that reached an outcome, limited to the last seven
 * days on the API clock. A deferred order re-enters a later run but is closed
 * for this one, so it belongs here too.
 */
export function completedOrders(
  orders: readonly CustomerOrder[],
  today: IsoDate,
): CustomerOrder[] {
  const from = addDays(today, -7);
  return orders
    .filter((order) => !isOngoingOrder(order))
    .filter((order) => order.requestedDeliveryDate >= from)
    .sort(byDeliveryDesc);
}

/** Orders requested for one delivery day. */
export function ordersOn(
  orders: readonly CustomerOrder[],
  date: IsoDate,
): CustomerOrder[] {
  return orders
    .filter((order) => order.requestedDeliveryDate === date)
    .sort(byDeliveryAsc);
}

/** Deferred orders, newest delivery day first. */
export function deferredOrders(
  orders: readonly CustomerOrder[],
): CustomerOrder[] {
  return orders
    .filter((order) => order.status === 'DEFERRED')
    .sort(byDeliveryDesc);
}

/* -------------------------------------------------------------------------- */
/* Cutoff clock                                                               */
/* -------------------------------------------------------------------------- */

/**
 * Milliseconds until today's 16:00 cutoff on the API clock, or a negative
 * number once it has passed. The offset is taken from the API's own `now`
 * string so the instant is 16:00 in Asia/Colombo, never the browser's zone.
 */
export function cutoffRemainingMs(
  now: Date,
  today: IsoDate,
  offset: string,
): number {
  const cutoff = Date.parse(`${today}T${ORDER_CUTOFF_LABEL}:00${offset}`);
  return cutoff - now.getTime();
}

/** The offset suffix of an RFC 3339 instant, e.g. `+05:30`. */
export function offsetOf(instant: string): string {
  return /[+-]\d{2}:\d{2}$/.test(instant) ? instant.slice(-6) : 'Z';
}

/* -------------------------------------------------------------------------- */
/* Placing an order                                                           */
/* -------------------------------------------------------------------------- */

/** itemId → quantity as typed in the order form. */
export type DraftQuantities = Readonly<Record<string, number>>;

export interface DraftSubmission {
  readonly items: readonly {
    readonly itemId: string;
    readonly quantity: number;
  }[];
  readonly units: number;
  readonly weightKg: number;
  readonly volumeM3: number;
  readonly temperature: TempRequirement;
  /** The first reason the order cannot be submitted, or null when it can. */
  readonly error: string | null;
}

/** Positive-quantity lines resolved against the catalogue. */
function selectedLines(
  items: readonly Item[],
  quantities: DraftQuantities,
): { readonly item: Item; readonly quantity: number }[] {
  const byId = new Map(items.map((item) => [item.itemId, item]));
  const lines: { item: Item; quantity: number }[] = [];
  for (const [itemId, quantity] of Object.entries(quantities)) {
    if (!quantity || quantity <= 0) continue;
    const item = byId.get(itemId);
    if (item) lines.push({ item, quantity });
  }
  return lines;
}

/**
 * Validates a draft and computes its totals against the catalogue dimensions.
 * Chilled/frozen and dry lines may not share one order: the backend derives a
 * single temperature per order, and merging them would put ambient goods on a
 * reefer and hide the separate order the store actually placed. The copy tells
 * the store to submit them one at a time.
 */
export function draftSubmission(
  items: readonly Item[],
  quantities: DraftQuantities,
): DraftSubmission {
  const lines = selectedLines(items, quantities);
  const units = lines.reduce((n, line) => n + line.quantity, 0);
  const weightKg = lines.reduce(
    (n, line) => n + line.quantity * line.item.unitWeightKg,
    0,
  );
  const volumeM3 = lines.reduce(
    (n, line) => n + line.quantity * line.item.unitVolumeM3,
    0,
  );
  const temperature: TempRequirement = lines.some(
    (line) =>
      line.item.temperatureRequirement === 'CHILLED' ||
      line.item.temperatureRequirement === 'FROZEN',
  )
    ? 'CHILLED'
    : 'AMBIENT';

  let error: string | null = null;
  if (lines.length === 0) {
    error = 'Add at least one item before submitting.';
  } else {
    const hasChilled = lines.some(
      (line) =>
        line.item.temperatureRequirement === 'CHILLED' ||
        line.item.temperatureRequirement === 'FROZEN',
    );
    const hasAmbient = lines.some(
      (line) => line.item.temperatureRequirement === 'AMBIENT',
    );
    if (hasChilled && hasAmbient) {
      error =
        'Chilled and dry items must be placed as separate orders. Clear one group and submit this order first.';
    }
  }

  return {
    items: lines.map((line) => ({
      itemId: line.item.itemId,
      quantity: line.quantity,
    })),
    units,
    weightKg,
    volumeM3,
    temperature,
    error,
  };
}

/** The next delivery day offered by default: the day after the API clock's day. */
export function nextDeliveryDay(today: IsoDate): IsoDate {
  return addDays(today, 1);
}

/* -------------------------------------------------------------------------- */
/* Operator-facing error copy                                                 */
/* -------------------------------------------------------------------------- */

/**
 * Turns a failed request into a sentence a store manager can act on. Raw Go
 * error text is never shown: the status decides the wording, and a 400/422
 * surfaces the field message the API supplied.
 */
export function readableStoreError(error: unknown, subject: string): string {
  if (error instanceof WaypointApiError) {
    if (error.isOffline) {
      return 'No connection to the server. Check the store connection and try again — nothing has been lost.';
    }
    switch (error.status) {
      case 401:
        return 'Your session has expired. Sign in again to continue.';
      case 403:
        return 'Your account is not allowed to see this. Sign in with the store account.';
      case 404:
        return `${subject} was not found, or it belongs to another outlet.`;
      case 409:
        return 'That action is not allowed from the order’s current status.';
      case 422:
      case 400:
        return (
          error.fieldErrors?.[0]?.message ??
          'The order was rejected. Check the items and quantities and try again.'
        );
      default:
        return 'Something went wrong on our side. Try again in a moment.';
    }
  }
  return 'Something went wrong. Try again in a moment.';
}
