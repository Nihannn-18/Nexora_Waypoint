import {
  CONSTRAINT_CATALOG,
  timeBudgetForBrand,
  type Brand,
  type ConfirmAllocationRequest,
  type ConstraintCode,
  type CustomerOrder,
  type DeferralReasonType,
  type Outlet,
  type PlanningProposal,
  type TripNumber,
  type Vehicle,
} from '@waypoint/shared-types';

/**
 * Turns a planning job's proposals into what the dispatcher reviews: trips
 * grouped by vehicle and trip, and the orders the engine proposes to defer.
 *
 * Nothing here decides feasibility — the engine already did, and confirmation
 * re-checks on the server. This only groups, orders and totals the server's
 * numbers for display (weights and volumes are the server-computed order
 * totals; trip minutes come from the engine's official formula).
 */

export interface StopView {
  readonly orderId: string;
  readonly seq: number;
  readonly order: CustomerOrder | undefined;
  readonly outlet: Outlet | undefined;
}

export interface TripView {
  readonly key: string;
  readonly vehicleId: string;
  readonly tripNo: TripNumber;
  readonly vehicle: Vehicle | undefined;
  readonly brand: Brand | undefined;
  readonly district: string | undefined;
  readonly stops: readonly StopView[];
  readonly weightKg: number;
  readonly volumeM3: number;
  readonly tripMinutes: number;
  /** Minutes the vehicle spends that day in this trip's budget pool. */
  readonly vehicleDayMinutes: number;
  /** 270 for Fresh, 480 shared by Style and Tech. */
  readonly budgetMinutes: number | undefined;
}

export interface DeferredView {
  readonly orderId: string;
  readonly constraintCode: ConstraintCode | undefined;
  /** The engine's plain-language reason. */
  readonly explanation: string;
  readonly order: CustomerOrder | undefined;
  readonly outlet: Outlet | undefined;
}

export interface ConstraintTally {
  readonly code: ConstraintCode | undefined;
  readonly count: number;
}

export interface PlanView {
  readonly trips: readonly TripView[];
  readonly deferred: readonly DeferredView[];
  readonly ordersTotal: number;
  readonly vehiclesUsed: number;
  /** Deferral counts by binding rule, most frequent first. */
  readonly deferralsByConstraint: readonly ConstraintTally[];
  /**
   * Orders whose status moved on since planning (allocated or deferred by a
   * confirmation, say). A confirm that includes them is refused by the API.
   */
  readonly changedSincePlanning: readonly string[];
}

export function buildPlanView(
  proposals: readonly PlanningProposal[],
  orders: ReadonlyMap<string, CustomerOrder>,
  outlets: ReadonlyMap<string, Outlet>,
  vehicles: ReadonlyMap<string, Vehicle>,
): PlanView {
  const groups = new Map<string, PlanningProposal[]>();
  const deferred: DeferredView[] = [];

  for (const p of proposals) {
    const order = orders.get(p.orderId);
    const outlet = order ? outlets.get(order.outletId) : undefined;
    if (p.decision === 'DEFER' || !p.vehicleId || !p.tripNo) {
      deferred.push({
        orderId: p.orderId,
        constraintCode: p.constraintCode,
        explanation: p.explanation,
        order,
        outlet,
      });
      continue;
    }
    const key = `${p.vehicleId}#${p.tripNo}`;
    const group = groups.get(key) ?? [];
    group.push(p);
    groups.set(key, group);
  }

  const draftTrips = [...groups.entries()].map(([key, rows]) => {
    const first = rows[0] as PlanningProposal;
    const stops = rows
      .map((p): StopView => {
        const order = orders.get(p.orderId);
        return {
          orderId: p.orderId,
          // seq is omitted on the wire when it is 0 (the first stop).
          seq: p.seq ?? 0,
          order,
          outlet: order ? outlets.get(order.outletId) : undefined,
        };
      })
      .sort((a, b) => a.seq - b.seq);
    const leadOrder = stops.find((s) => s.order)?.order;
    const leadOutlet = stops.find((s) => s.outlet)?.outlet;
    return {
      key,
      vehicleId: first.vehicleId as string,
      tripNo: first.tripNo as TripNumber,
      vehicle: vehicles.get(first.vehicleId as string),
      brand: leadOrder?.brand,
      district: leadOutlet?.district,
      stops,
      weightKg: sum(stops.map((s) => s.order?.totalWeightKg ?? 0)),
      volumeM3: sum(stops.map((s) => s.order?.totalVolumeM3 ?? 0)),
      tripMinutes: first.tripMinutes ?? 0,
    };
  });

  // Each vehicle has one Fresh budget and one shared Style + Tech budget per
  // day, so a trip's budget meter shows the vehicle's whole day in that pool.
  const poolMinutes = new Map<string, number>();
  const poolKey = (vehicleId: string, brand: Brand | undefined) =>
    `${vehicleId}|${brand === 'FRESH' ? 'FRESH' : 'STYLE_TECH'}`;
  for (const t of draftTrips) {
    const k = poolKey(t.vehicleId, t.brand);
    poolMinutes.set(k, (poolMinutes.get(k) ?? 0) + t.tripMinutes);
  }

  const trips: TripView[] = draftTrips
    .map((t) => ({
      ...t,
      vehicleDayMinutes:
        poolMinutes.get(poolKey(t.vehicleId, t.brand)) ?? t.tripMinutes,
      budgetMinutes: t.brand ? timeBudgetForBrand(t.brand) : undefined,
    }))
    .sort((a, b) =>
      a.vehicleId === b.vehicleId
        ? a.tripNo - b.tripNo
        : a.vehicleId.localeCompare(b.vehicleId),
    );

  const tally = new Map<ConstraintCode | undefined, number>();
  for (const d of deferred) {
    tally.set(d.constraintCode, (tally.get(d.constraintCode) ?? 0) + 1);
  }
  const deferralsByConstraint = [...tally.entries()]
    .map(([code, count]) => ({ code, count }))
    .sort((a, b) => b.count - a.count);

  const changedSincePlanning = proposals
    .filter((p) => {
      const status = orders.get(p.orderId)?.status;
      return status !== undefined && status !== 'CONFIRMED';
    })
    .map((p) => p.orderId);

  return {
    trips,
    deferred,
    ordersTotal: proposals.length,
    vehiclesUsed: new Set(trips.map((t) => t.vehicleId)).size,
    deferralsByConstraint,
    changedSincePlanning,
  };
}

function sum(values: readonly number[]): number {
  return values.reduce((a, b) => a + b, 0);
}

/* -------------------------------------------------------------------------- */
/* Deferral reasons and the confirm request                                    */
/* -------------------------------------------------------------------------- */

export interface DeferralDecision {
  readonly reasonType: DeferralReasonType;
  readonly reasonText: string;
}

/** Orders whose deferral still has no reason; confirm stays disabled until empty. */
export function ordersMissingReason(
  deferred: readonly DeferredView[],
  decisions: ReadonlyMap<string, DeferralDecision>,
): string[] {
  return deferred
    .filter((d) => !decisions.get(d.orderId)?.reasonText.trim())
    .map((d) => d.orderId);
}

/** The engine's reason, as the default the dispatcher can accept or rewrite. */
export function suggestedDecision(d: DeferredView): DeferralDecision {
  return {
    reasonType: 'CONSTRAINT',
    reasonText:
      d.explanation.trim() ||
      (d.constraintCode
        ? CONSTRAINT_CATALOG[d.constraintCode].explanation
        : ''),
  };
}

/**
 * Builds POST /allocations/confirm from the reviewed plan. Stops travel in the
 * engine's sequence (the loader loads in reverse of it); every deferred order
 * carries the dispatcher's reason. Throws if a reason is missing, so a bad
 * request can never be sent.
 */
export function buildConfirmRequest(
  jobId: string,
  view: PlanView,
  decisions: ReadonlyMap<string, DeferralDecision>,
): ConfirmAllocationRequest {
  const missing = ordersMissingReason(view.deferred, decisions);
  if (missing.length > 0) {
    throw new Error(`${missing.length} deferral(s) have no reason`);
  }
  return {
    jobId,
    routes: view.trips.map((t) => ({
      vehicleId: t.vehicleId,
      tripNo: t.tripNo,
      orderIds: t.stops.map((s) => s.orderId),
    })),
    deferrals: view.deferred.map((d) => {
      const decision = decisions.get(d.orderId) as DeferralDecision;
      return {
        orderId: d.orderId,
        reasonType: decision.reasonType,
        ...(d.constraintCode ? { constraintCode: d.constraintCode } : {}),
        reasonText: decision.reasonText.trim(),
      };
    }),
  };
}
