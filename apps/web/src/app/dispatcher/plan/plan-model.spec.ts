import type {
  CustomerOrder,
  Outlet,
  PlanningProposal,
  Vehicle,
} from '@waypoint/shared-types';
import {
  buildConfirmRequest,
  buildPlanView,
  ordersMissingReason,
  suggestedDecision,
  type DeferralDecision,
} from './plan-model';

const order = (
  id: string,
  over: Partial<CustomerOrder> = {},
): CustomerOrder => ({
  orderId: id,
  orderNumber: `ORD-${id}`,
  outletId: `OUT-${id}`,
  brand: 'FRESH',
  orderDate: '2026-09-25',
  requestedDeliveryDate: '2026-09-26',
  totalUnits: 10,
  totalWeightKg: 100,
  totalVolumeM3: 1,
  temperatureRequirement: 'CHILLED',
  status: 'CONFIRMED',
  afterCutoff: false,
  ...over,
});

const outlet = (id: string, district = 'Colombo'): Outlet => ({
  outletId: id,
  name: `Outlet ${id}`,
  brand: 'FRESH',
  district,
  depotId: 'd1',
  dockType: 'REAR_DOCK',
  parkingConstraint: 'NORMAL',
  mallWindow: null,
  windowOpenTime: '05:00',
  windowCloseTime: '08:00',
});

const vehicle: Vehicle = {
  vehicleId: 'VEH014',
  type: 'TRUCK',
  tempClass: 'REEFER',
  weightCapKg: 2500,
  volumeCapM3: 18,
  fuelType: 'diesel',
  kmPerL: 6,
  weeklyFuelQuotaL: 400,
  depotId: 'd1',
  status: 'AVAILABLE',
};

// Wire shape: a zero seq is omitted, so the first stop has no `seq`.
const proposals: PlanningProposal[] = [
  {
    orderId: 'b',
    decision: 'SERVE',
    vehicleId: 'VEH014',
    tripNo: 1,
    seq: 1,
    tripMinutes: 101,
    explanation: '',
  },
  {
    orderId: 'a',
    decision: 'SERVE',
    vehicleId: 'VEH014',
    tripNo: 1,
    tripMinutes: 101,
    explanation: '',
  },
  {
    orderId: 'c',
    decision: 'SERVE',
    vehicleId: 'VEH014',
    tripNo: 2,
    tripMinutes: 150,
    explanation: '',
  },
  {
    orderId: 'x',
    decision: 'DEFER',
    constraintCode: 'REEFER_REQUIRED',
    explanation: 'No reefer trip left.',
  },
  {
    orderId: 'y',
    decision: 'DEFER',
    constraintCode: 'REEFER_REQUIRED',
    explanation: 'No reefer trip left.',
  },
  {
    orderId: 'z',
    decision: 'DEFER',
    explanation: 'Did not fit the daily budgets.',
  },
];

function view(statuses: Record<string, CustomerOrder['status']> = {}) {
  const ids = ['a', 'b', 'c', 'x', 'y', 'z'];
  const orders = new Map(
    ids.map((id) => [id, order(id, { status: statuses[id] ?? 'CONFIRMED' })]),
  );
  const outlets = new Map(ids.map((id) => [`OUT-${id}`, outlet(`OUT-${id}`)]));
  return buildPlanView(
    proposals,
    orders,
    outlets,
    new Map([['VEH014', vehicle]]),
  );
}

describe('buildPlanView', () => {
  it('groups served orders into vehicle trips in stop order', () => {
    const v = view();
    expect(v.trips.map((t) => t.key)).toEqual(['VEH014#1', 'VEH014#2']);
    // The stop with no seq on the wire is stop 0, so it leads.
    expect(v.trips[0]?.stops.map((s) => s.orderId)).toEqual(['a', 'b']);
    expect(v.trips[0]?.district).toBe('Colombo');
    expect(v.trips[0]?.weightKg).toBe(200);
  });

  it('meters a trip against the vehicle’s whole day in its budget pool', () => {
    const v = view();
    // Both trips are Fresh: 101 + 150 minutes of one 270-minute budget.
    expect(v.trips[0]?.vehicleDayMinutes).toBe(251);
    expect(v.trips[0]?.budgetMinutes).toBe(270);
  });

  it('lists deferrals with their binding rule, most frequent rule first', () => {
    const v = view();
    expect(v.deferred).toHaveLength(3);
    expect(v.deferralsByConstraint[0]).toEqual({
      code: 'REEFER_REQUIRED',
      count: 2,
    });
    expect(v.vehiclesUsed).toBe(1);
    expect(v.ordersTotal).toBe(6);
  });

  it('flags orders that moved on since planning, so a stale plan is not confirmed', () => {
    expect(view().changedSincePlanning).toEqual([]);
    expect(view({ a: 'ALLOCATED' }).changedSincePlanning).toEqual(['a']);
  });
});

describe('deferral reasons and the confirm request', () => {
  it('requires a non-blank reason for every deferral', () => {
    const v = view();
    const decisions = new Map<string, DeferralDecision>([
      ['x', { reasonType: 'CONSTRAINT', reasonText: 'No reefer left' }],
      ['y', { reasonType: 'CONSTRAINT', reasonText: '   ' }],
    ]);
    expect(ordersMissingReason(v.deferred, decisions)).toEqual(['y', 'z']);
    expect(() => buildConfirmRequest('JOB-1', v, decisions)).toThrow(
      /no reason/,
    );
  });

  it('offers the engine’s own explanation as the suggested reason', () => {
    const v = view();
    const x = v.deferred.find((d) => d.orderId === 'x');
    expect(x && suggestedDecision(x)).toEqual({
      reasonType: 'CONSTRAINT',
      reasonText: 'No reefer trip left.',
    });
  });

  it('sends every route in stop order and every deferral with its reason', () => {
    const v = view();
    const decisions = new Map(
      v.deferred.map((d) => [d.orderId, suggestedDecision(d)]),
    );
    const req = buildConfirmRequest('JOB-1', v, decisions);
    expect(req.jobId).toBe('JOB-1');
    expect(req.routes).toEqual([
      { vehicleId: 'VEH014', tripNo: 1, orderIds: ['a', 'b'] },
      { vehicleId: 'VEH014', tripNo: 2, orderIds: ['c'] },
    ]);
    expect(req.deferrals).toContainEqual({
      orderId: 'x',
      reasonType: 'CONSTRAINT',
      constraintCode: 'REEFER_REQUIRED',
      reasonText: 'No reefer trip left.',
    });
    // No invented constraint code when the engine named none.
    expect(req.deferrals.find((d) => d.orderId === 'z')).not.toHaveProperty(
      'constraintCode',
    );
  });
});
