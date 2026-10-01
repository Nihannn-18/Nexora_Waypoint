/**
 * The official trip-time formula, from the Challenge Booklet.
 *
 *   outbound_min  = district_travel.depot_to_district_freeflow_min      (once)
 *   inter_stop_min= district_travel.inter_stop_freeflow_min × max(n-1, 0)
 *   handling_min  = Σ service_allowance(brand, outlet.dock_type)
 *   trip_minutes  = outbound + inter_stop + handling
 *
 * The return journey to the depot is NOT counted.
 *
 * ── Why this exists in TypeScript ──────────────────────────────────────────
 * The Go `TripTimeCalculator` is the authoritative implementation; the backend
 * revalidates every plan regardless of what the client computed. This mirror is
 * for the dispatcher's live meters on D-03, so dragging an order between lanes
 * updates the minute counter without a round-trip. It must stay numerically
 * identical to the Go version — the worked example from the brief is pinned as
 * a test in both languages, and that test is the contract.
 *
 * Do not replace this with a routing API. The brief requires the supplied
 * district_travel / service_allowance reference data and this exact formula.
 */

import type { Brand } from './domain';
import {
  FRESH_TIME_BUDGET_MIN,
  STYLE_TECH_TIME_BUDGET_MIN,
  timeBudgetForBrand,
} from './constraints';

export interface TripTimeInput {
  /** `depot_to_district_freeflow_min` for this depot → district pair. */
  readonly outboundFreeflowMin: number;
  /** `inter_stop_freeflow_min` for this district. */
  readonly interStopFreeflowMin: number;
  /** One `service_allowance_min` per order, looked up by brand + dock type. */
  readonly serviceAllowancesMin: readonly number[];
}

export interface TripTimeBreakdown {
  readonly orderCount: number;
  readonly outboundMin: number;
  readonly interStopMin: number;
  readonly handlingMin: number;
  readonly totalTripMin: number;
}

/**
 * An empty trip takes zero minutes — a vehicle with no orders was never
 * dispatched, so it must not consume any of its daily budget.
 */
export function calculateTripTime(input: TripTimeInput): TripTimeBreakdown {
  const orderCount = input.serviceAllowancesMin.length;

  if (orderCount === 0) {
    return {
      orderCount: 0,
      outboundMin: 0,
      interStopMin: 0,
      handlingMin: 0,
      totalTripMin: 0,
    };
  }

  const outboundMin = input.outboundFreeflowMin;
  const interStopMin = input.interStopFreeflowMin * Math.max(orderCount - 1, 0);
  const handlingMin = input.serviceAllowancesMin.reduce(
    (sum, min) => sum + min,
    0,
  );

  return {
    orderCount,
    outboundMin,
    interStopMin,
    handlingMin,
    totalTripMin: outboundMin + interStopMin + handlingMin,
  };
}

/* -------------------------------------------------------------------------- */
/* Daily budget accounting                                                    */
/* -------------------------------------------------------------------------- */

export interface VehicleDayBudget {
  /** Minutes already committed to Fresh trips for this vehicle today. */
  readonly freshMinutesUsed: number;
  /** Minutes already committed to Style and Tech trips, which share a pool. */
  readonly styleTechMinutesUsed: number;
}

export interface BudgetCheck {
  readonly fits: boolean;
  readonly budgetMin: number;
  readonly usedMin: number;
  readonly remainingMin: number;
  /** Minutes over budget; 0 when the trip fits. */
  readonly overByMin: number;
}

/**
 * Whether adding `tripMinutes` of `brand` work keeps the vehicle inside its
 * daily budget. Fresh draws on its own 270; Style and Tech share 480.
 */
export function checkTimeBudget(
  brand: Brand,
  tripMinutes: number,
  used: VehicleDayBudget,
): BudgetCheck {
  const budgetMin = timeBudgetForBrand(brand);
  const usedMin =
    brand === 'FRESH' ? used.freshMinutesUsed : used.styleTechMinutesUsed;
  const projected = usedMin + tripMinutes;

  return {
    // Exactly at budget is allowed; one minute over is not.
    fits: projected <= budgetMin,
    budgetMin,
    usedMin,
    remainingMin: Math.max(budgetMin - usedMin, 0),
    overByMin: Math.max(projected - budgetMin, 0),
  };
}

export function remainingFreshMinutes(used: VehicleDayBudget): number {
  return Math.max(FRESH_TIME_BUDGET_MIN - used.freshMinutesUsed, 0);
}

export function remainingStyleTechMinutes(used: VehicleDayBudget): number {
  return Math.max(STYLE_TECH_TIME_BUDGET_MIN - used.styleTechMinutesUsed, 0);
}

/* -------------------------------------------------------------------------- */
/* Delivery window arithmetic                                                 */
/* -------------------------------------------------------------------------- */

/** Minutes since midnight for a 24-hour `HH:mm`. */
export function clockToMinutes(clock: string): number {
  const [h, m] = clock.split(':');
  const hours = Number(h);
  const minutes = Number(m);
  if (!Number.isFinite(hours) || !Number.isFinite(minutes)) {
    throw new Error(`Invalid clock time: "${clock}" (expected HH:mm)`);
  }
  return hours * 60 + minutes;
}

export function minutesToClock(total: number): string {
  const wrapped = ((total % 1440) + 1440) % 1440;
  const h = Math.floor(wrapped / 60);
  const m = wrapped % 60;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

export interface WindowAssessment {
  readonly arrival: string;
  /** When handling actually begins — an early vehicle waits for the window. */
  readonly serviceStart: string;
  readonly waitingMin: number;
  /** Lateness is arrival *after the window closes*, per the official definition. */
  readonly late: boolean;
  readonly lateByMin: number;
}

/**
 * Early arrival waits until the window opens; lateness is arrival after it
 * closes. This matches the Datathon Task 1 label definition exactly, so the
 * operational app and the model agree on what "late" means.
 */
export function assessWindow(
  arrivalClock: string,
  windowOpen: string,
  windowClose: string,
): WindowAssessment {
  const arrival = clockToMinutes(arrivalClock);
  const open = clockToMinutes(windowOpen);
  const close = clockToMinutes(windowClose);

  const waitingMin = arrival < open ? open - arrival : 0;
  const serviceStart = Math.max(arrival, open);
  const lateByMin = arrival > close ? arrival - close : 0;

  return {
    arrival: arrivalClock,
    serviceStart: minutesToClock(serviceStart),
    waitingMin,
    late: lateByMin > 0,
    lateByMin,
  };
}

/* -------------------------------------------------------------------------- */
/* Fuel                                                                       */
/* -------------------------------------------------------------------------- */

export function estimateFuelLitres(distanceKm: number, kmPerL: number): number {
  if (kmPerL <= 0) {
    throw new Error('kmPerL must be greater than zero');
  }
  return distanceKm / kmPerL;
}

export function isWithinWeeklyFuelQuota(
  weeklyUsedL: number,
  projectedL: number,
  quotaL: number,
): boolean {
  return weeklyUsedL + projectedL <= quotaL;
}
