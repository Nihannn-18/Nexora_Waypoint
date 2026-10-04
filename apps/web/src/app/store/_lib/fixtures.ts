/**
 * Test-only builders for the store-manager specs. Kept out of the app bundle:
 * nothing under `src/app` imports this except `*.spec.tsx` files.
 */
import type {
  CustomerOrder,
  Item,
  MetaResponse,
  Outlet,
} from '@waypoint/shared-types';

export const demoMeta: MetaResponse = {
  now: '2026-09-25T15:40:00+05:30',
  demoMode: true,
  timezone: 'Asia/Colombo',
};

export const ownOutlet: Outlet = {
  outletId: 'OUT014',
  name: 'Outlet OUT014',
  brand: 'FRESH',
  district: 'Colombo',
  depotId: 'depot-peli',
  dockType: 'REAR_DOCK',
  parkingConstraint: 'NORMAL',
  mallWindow: null,
  windowOpenTime: '05:00',
  windowCloseTime: '08:00',
};

export function makeOrder(over: Partial<CustomerOrder> = {}): CustomerOrder {
  return {
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
    lines: [
      {
        orderItemId: 'oi1',
        itemId: 'it-ambient',
        sku: 'DEMO-FRESH-AMBIENT',
        name: 'Demo ambient',
        quantity: 25,
        unitWeightKgSnapshot: 4.94,
        unitVolumeM3Snapshot: 0.0568,
        totalWeightKg: 123.5,
        totalVolumeM3: 1.42,
      },
    ],
    ...over,
  };
}

export function makeItem(over: Partial<Item> = {}): Item {
  return {
    itemId: 'it-ambient',
    sku: 'DEMO-FRESH-AMBIENT',
    name: 'Demo ambient',
    brand: 'FRESH',
    unitWeightKg: 1,
    unitVolumeM3: 0.01,
    temperatureRequirement: 'AMBIENT',
    ...over,
  };
}
