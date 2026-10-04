import { WaypointApiError } from '@waypoint/api-client';
import type { CustomerOrder, Item } from '@waypoint/shared-types';
import {
  completedOrders,
  cutoffRemainingMs,
  deferredOrders,
  draftSubmission,
  isOngoingOrder,
  nextDeliveryDay,
  offsetOf,
  ongoingOrders,
  ordersOn,
  readableStoreError,
} from './store';

const order = (over: Partial<CustomerOrder> = {}): CustomerOrder => ({
  orderId: 'o1',
  orderNumber: 'ORD-2026-000001',
  outletId: 'OUT014',
  brand: 'FRESH',
  orderDate: '2026-09-25',
  requestedDeliveryDate: '2026-09-26',
  totalUnits: 25,
  totalWeightKg: 123.5,
  totalVolumeM3: 1.42,
  temperatureRequirement: 'AMBIENT',
  status: 'PLACED',
  afterCutoff: false,
  ...over,
});

const item = (over: Partial<Item> = {}): Item => ({
  itemId: 'it1',
  sku: 'DEMO-FRESH-AMBIENT',
  name: 'Demo ambient',
  brand: 'FRESH',
  unitWeightKg: 1,
  unitVolumeM3: 0.01,
  temperatureRequirement: 'AMBIENT',
  ...over,
});

describe('store order grouping', () => {
  it('treats every non-terminal status as ongoing and the terminal ones as closed', () => {
    expect(isOngoingOrder(order({ status: 'PLACED' }))).toBe(true);
    expect(isOngoingOrder(order({ status: 'CONFIRMED' }))).toBe(true);
    expect(isOngoingOrder(order({ status: 'IN_TRANSIT' }))).toBe(true);
    expect(isOngoingOrder(order({ status: 'DELIVERED' }))).toBe(false);
    expect(isOngoingOrder(order({ status: 'FAILED' }))).toBe(false);
    // A deferred order is terminal for its original day.
    expect(isOngoingOrder(order({ status: 'DEFERRED' }))).toBe(false);
  });

  it('orders live orders soonest-first and leaves out the closed ones', () => {
    const list = ongoingOrders([
      order({ orderId: 'b', requestedDeliveryDate: '2026-09-28' }),
      order({ orderId: 'a', requestedDeliveryDate: '2026-09-26' }),
      order({ orderId: 'z', status: 'DELIVERED' }),
    ]);
    expect(list.map((o) => o.orderId)).toEqual(['a', 'b']);
  });

  it('windows the completed tail to the last seven days', () => {
    const today = '2026-09-25';
    const list = completedOrders(
      [
        order({
          orderId: 'recent',
          status: 'DELIVERED',
          requestedDeliveryDate: '2026-09-24',
        }),
        order({
          orderId: 'edge',
          status: 'RECEIVED',
          requestedDeliveryDate: '2026-09-18',
        }),
        order({
          orderId: 'today',
          status: 'DEFERRED',
          requestedDeliveryDate: '2026-09-25',
        }),
        order({
          orderId: 'old',
          status: 'FAILED',
          requestedDeliveryDate: '2026-09-17',
        }),
        order({
          orderId: 'live',
          status: 'PLACED',
          requestedDeliveryDate: '2026-09-26',
        }),
      ],
      today,
    );
    expect(list.map((o) => o.orderId)).toEqual(['today', 'recent', 'edge']);
  });

  it('filters one delivery day and finds deferred orders', () => {
    const list = [
      order({ orderId: 'a', requestedDeliveryDate: '2026-09-26' }),
      order({ orderId: 'b', requestedDeliveryDate: '2026-09-27' }),
      order({
        orderId: 'c',
        status: 'DEFERRED',
        requestedDeliveryDate: '2026-09-26',
      }),
    ];
    expect(ordersOn(list, '2026-09-26').map((o) => o.orderId)).toEqual([
      'a',
      'c',
    ]);
    expect(deferredOrders(list).map((o) => o.orderId)).toEqual(['c']);
  });

  it('defaults the next delivery day to the day after the API day', () => {
    expect(nextDeliveryDay('2026-09-25')).toBe('2026-09-26');
  });
});

describe('cutoff clock', () => {
  it('measures remaining time to 16:00 in the API offset', () => {
    const before = new Date('2026-09-25T15:40:00+05:30');
    const after = new Date('2026-09-25T16:05:00+05:30');
    expect(cutoffRemainingMs(before, '2026-09-25', '+05:30')).toBe(
      20 * 60 * 1000,
    );
    expect(cutoffRemainingMs(after, '2026-09-25', '+05:30')).toBeLessThan(0);
  });

  it('reads the offset suffix, defaulting to Z', () => {
    expect(offsetOf('2026-09-25T15:40:00+05:30')).toBe('+05:30');
    expect(offsetOf('2026-09-25T15:40:00Z')).toBe('Z');
  });
});

describe('order draft', () => {
  it('refuses an empty draft', () => {
    const draft = draftSubmission([item()], {});
    expect(draft.error).toMatch(/at least one item/i);
    expect(draft.items).toHaveLength(0);
  });

  it('computes totals from the catalogue dimensions', () => {
    const draft = draftSubmission(
      [item({ itemId: 'a', unitWeightKg: 2, unitVolumeM3: 0.5 })],
      {
        a: 3,
        ignored: 0,
      },
    );
    expect(draft.error).toBeNull();
    expect(draft).toMatchObject({
      units: 3,
      weightKg: 6,
      volumeM3: 1.5,
      temperature: 'AMBIENT',
    });
    expect(draft.items).toEqual([{ itemId: 'a', quantity: 3 }]);
  });

  it('derives chilled from any refrigerated line', () => {
    const draft = draftSubmission(
      [item({ itemId: 'cold', temperatureRequirement: 'CHILLED' })],
      { cold: 1 },
    );
    expect(draft.temperature).toBe('CHILLED');
  });

  // The design rule: a Fresh outlet places chilled and dry as separate orders.
  it('blocks merging chilled and dry lines into one order', () => {
    const draft = draftSubmission(
      [
        item({ itemId: 'cold', temperatureRequirement: 'CHILLED' }),
        item({ itemId: 'dry', temperatureRequirement: 'AMBIENT' }),
      ],
      { cold: 1, dry: 1 },
    );
    expect(draft.error).toMatch(/separate orders/i);
  });
});

describe('readableStoreError', () => {
  it('treats an offline request as harmless and retryable', () => {
    expect(
      readableStoreError(
        new WaypointApiError('x', { status: 0, isOffline: true }),
        'Your orders',
      ),
    ).toMatch(/nothing has been lost/i);
  });

  it('names the subject and hides another outlet', () => {
    expect(
      readableStoreError(
        new WaypointApiError('nope', { status: 404 }),
        'This order',
      ),
    ).toMatch(/This order was not found/);
  });

  it('surfaces the field message the API supplied', () => {
    expect(
      readableStoreError(
        new WaypointApiError('invalid', {
          status: 400,
          fieldErrors: [
            { field: 'items', message: 'an order needs at least one line' },
          ],
        }),
        'This order',
      ),
    ).toBe('an order needs at least one line');
  });
});
