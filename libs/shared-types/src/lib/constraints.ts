/**
 * The feasibility rule catalogue.
 *
 * Every hard rule in the brief has exactly one code here. The Go
 * ConstraintValidationService is the *only* authoritative implementation —
 * this file exists so the client can label a violation, explain it to a human,
 * and trace it back to the rule panel the judges saw in the Day 5 design.
 *
 * `designRuleId` is the E-0x identifier printed in the Designathon D-03 rule
 * panel. Keep it: the Hackathon is scored partly on fidelity to that design,
 * and a judge comparing the two should see the same identifiers.
 */

import type { Brand } from './domain';

/* -------------------------------------------------------------------------- */
/* Official numeric limits — do not inline these anywhere else                 */
/* -------------------------------------------------------------------------- */

/** Per-vehicle, per-day minute budget for Fresh trips. */
export const FRESH_TIME_BUDGET_MIN = 270;

/** Per-vehicle, per-day minute budget for Style and Tech *combined*. */
export const STYLE_TECH_TIME_BUDGET_MIN = 480;

/** A vehicle may run at most this many trips per day. */
export const MAX_TRIPS_PER_VEHICLE_PER_DAY = 2;

/** Next-day orders close at 16:00 Asia/Colombo. */
export const ORDER_CUTOFF_HOUR = 16;
export const ORDER_CUTOFF_LABEL = '16:00';

/** The system operates in a single timezone; all wall-clock times are local. */
export const OPERATING_TIMEZONE = 'Asia/Colombo';

/** Which budget a brand draws from. Style and Tech share one pool. */
export function timeBudgetForBrand(brand: Brand): number {
  return brand === 'FRESH' ? FRESH_TIME_BUDGET_MIN : STYLE_TECH_TIME_BUDGET_MIN;
}

/* -------------------------------------------------------------------------- */
/* Constraint codes                                                           */
/* -------------------------------------------------------------------------- */

export const CONSTRAINT_CODES = [
  'VEHICLE_UNAVAILABLE',
  'DEPOT_MISMATCH',
  'BRAND_DISTRICT_MIX',
  'REEFER_REQUIRED',
  'VAN_ONLY_ACCESS',
  'ORDER_SPLIT_FORBIDDEN',
  'TRIP_NUMBER_INVALID',
  'TRIP_LIMIT_EXCEEDED',
  'WEIGHT_CAPACITY_EXCEEDED',
  'VOLUME_CAPACITY_EXCEEDED',
  'FRESH_TIME_BUDGET',
  'STYLE_TECH_TIME_BUDGET',
  'DELIVERY_WINDOW_MISSED',
  'MALL_WINDOW_MISSED',
  'FUEL_QUOTA_EXCEEDED',
  'DUPLICATE_ASSIGNMENT',
  'NON_OPERATING_DAY',
] as const;

export type ConstraintCode = (typeof CONSTRAINT_CODES)[number];

/** The E-0x rule groups shown in the dispatcher's D-03 rule panel. */
export type DesignRuleId =
  | 'E-01'
  | 'E-02'
  | 'E-03'
  | 'E-04'
  | 'E-05'
  | 'E-06'
  | 'E-07';

export interface ConstraintDefinition {
  readonly code: ConstraintCode;
  /** The Designathon rule-panel group this check belongs to. */
  readonly designRuleId: DesignRuleId;
  /** Short label for the rule panel and violation chips. */
  readonly label: string;
  /**
   * Operator-facing explanation. This is what a dispatcher reads when deciding
   * whether a deferral was unavoidable, so it names the binding quantity rather
   * than restating the rule.
   */
  readonly explanation: string;
}

/**
 * Hard constraints only. Every one of these blocks an allocation; none is
 * advisory. A violated constraint is never overridden in the UI — the backend
 * revalidates regardless of what the client allowed.
 */
export const CONSTRAINT_CATALOG: Readonly<
  Record<ConstraintCode, ConstraintDefinition>
> = {
  WEIGHT_CAPACITY_EXCEEDED: {
    code: 'WEIGHT_CAPACITY_EXCEEDED',
    designRuleId: 'E-01',
    label: 'Weight capacity',
    explanation: 'Trip weight would exceed the vehicle weight limit.',
  },
  VOLUME_CAPACITY_EXCEEDED: {
    code: 'VOLUME_CAPACITY_EXCEEDED',
    designRuleId: 'E-01',
    label: 'Volume capacity',
    explanation: 'Trip volume would exceed the vehicle volume limit.',
  },
  REEFER_REQUIRED: {
    code: 'REEFER_REQUIRED',
    designRuleId: 'E-02',
    label: 'Refrigeration',
    explanation: 'Chilled or frozen goods require a refrigerated vehicle.',
  },
  VAN_ONLY_ACCESS: {
    code: 'VAN_ONLY_ACCESS',
    designRuleId: 'E-03',
    label: 'Van-only access',
    explanation: 'This outlet cannot be served by a truck.',
  },
  DEPOT_MISMATCH: {
    code: 'DEPOT_MISMATCH',
    designRuleId: 'E-04',
    label: 'Home depot',
    explanation: 'A vehicle may serve only outlets assigned to its own depot.',
  },
  TRIP_LIMIT_EXCEEDED: {
    code: 'TRIP_LIMIT_EXCEEDED',
    designRuleId: 'E-05',
    label: 'Trips per day',
    explanation: 'A vehicle may run at most two trips per day.',
  },
  TRIP_NUMBER_INVALID: {
    code: 'TRIP_NUMBER_INVALID',
    designRuleId: 'E-05',
    label: 'Trip number',
    explanation: 'Trip number must be 1 or 2.',
  },
  FRESH_TIME_BUDGET: {
    code: 'FRESH_TIME_BUDGET',
    designRuleId: 'E-05',
    label: 'Fresh time budget',
    explanation: `No feasible Fresh trip remains within the ${FRESH_TIME_BUDGET_MIN}-minute vehicle budget.`,
  },
  STYLE_TECH_TIME_BUDGET: {
    code: 'STYLE_TECH_TIME_BUDGET',
    designRuleId: 'E-05',
    label: 'Style/Tech time budget',
    explanation: `Style and Tech trips share a ${STYLE_TECH_TIME_BUDGET_MIN}-minute vehicle budget, which is exhausted.`,
  },
  DELIVERY_WINDOW_MISSED: {
    code: 'DELIVERY_WINDOW_MISSED',
    designRuleId: 'E-06',
    label: 'Delivery window',
    explanation:
      'Planned arrival falls after the outlet delivery window closes.',
  },
  MALL_WINDOW_MISSED: {
    code: 'MALL_WINDOW_MISSED',
    designRuleId: 'E-06',
    label: 'Mall access window',
    explanation:
      'Planned arrival falls outside the mall’s fixed access window.',
  },
  FUEL_QUOTA_EXCEEDED: {
    code: 'FUEL_QUOTA_EXCEEDED',
    designRuleId: 'E-07',
    label: 'Weekly fuel quota',
    explanation: 'Projected weekly fuel use would exceed this vehicle’s quota.',
  },
  BRAND_DISTRICT_MIX: {
    code: 'BRAND_DISTRICT_MIX',
    designRuleId: 'E-05',
    label: 'Trip composition',
    explanation:
      'All orders on one vehicle and trip must share a brand and a district.',
  },
  VEHICLE_UNAVAILABLE: {
    code: 'VEHICLE_UNAVAILABLE',
    designRuleId: 'E-05',
    label: 'Vehicle availability',
    explanation:
      'Vehicle is in the workshop or otherwise unavailable on this date.',
  },
  ORDER_SPLIT_FORBIDDEN: {
    code: 'ORDER_SPLIT_FORBIDDEN',
    designRuleId: 'E-01',
    label: 'Whole order',
    explanation: 'An order cannot be split across vehicles or trips.',
  },
  DUPLICATE_ASSIGNMENT: {
    code: 'DUPLICATE_ASSIGNMENT',
    designRuleId: 'E-01',
    label: 'Duplicate assignment',
    explanation: 'This order is already assigned to another vehicle or trip.',
  },
  NON_OPERATING_DAY: {
    code: 'NON_OPERATING_DAY',
    designRuleId: 'E-05',
    label: 'Operating day',
    explanation: 'The planning date is not an operating day.',
  },
} as const;

/** Rule-panel groups, in the order the D-03 panel lists them. */
export const DESIGN_RULE_GROUPS: Readonly<Record<DesignRuleId, string>> = {
  'E-01': 'Capacity — weight and volume',
  'E-02': 'Refrigeration',
  'E-03': 'Outlet access',
  'E-04': 'Home depot',
  'E-05': 'Trips and time budgets',
  'E-06': 'Delivery windows',
  'E-07': 'Weekly fuel quota',
} as const;

/* -------------------------------------------------------------------------- */
/* Deferral reasons                                                           */
/* -------------------------------------------------------------------------- */

/**
 * Why an order was not served. A constraint-driven deferral carries the
 * blocking ConstraintCode; a dispatcher's judgement call carries POLICY_* and
 * requires free-text, because the brief insists deferrals be explained rather
 * than silently recorded.
 */
export const DEFERRAL_REASON_TYPES = [
  'CONSTRAINT', // a hard rule left no feasible slot
  'POLICY', // dispatcher prioritised another outlet
  'OPERATIONAL', // breakdown, shortfall, depot problem
] as const;
export type DeferralReasonType = (typeof DEFERRAL_REASON_TYPES)[number];

/** A deferral must always be attributable and timestamped. */
export interface DeferralReason {
  readonly reasonType: DeferralReasonType;
  /** Present when reasonType is CONSTRAINT. */
  readonly constraintCode?: ConstraintCode;
  /** Human-readable text shown to the store manager in S-05. */
  readonly reasonText: string;
  readonly deferredToDate?: string; // ISO date
}

/* -------------------------------------------------------------------------- */
/* Prioritisation policy                                                      */
/* -------------------------------------------------------------------------- */

/**
 * The team's documented fairness ordering, applied *after* feasibility.
 * Earlier entries outrank later ones. Documented rather than silently encoded,
 * as the brief requires — docs/prioritisation-policy.md is the written version.
 */
export const PRIORITISATION_POLICY = [
  {
    rank: 1,
    key: 'NOT_DEFERRED_YESTERDAY',
    rule: 'Never defer an outlet that was deferred yesterday.',
  },
  {
    rank: 2,
    key: 'STARVATION_GUARD',
    rule: 'Protect outlets unserved for 7 or more days.',
  },
  {
    rank: 3,
    key: 'CHILLED_FIRST',
    rule: 'Chilled and frozen load before ambient — reefer capacity is the scarce resource.',
  },
  {
    rank: 4,
    key: 'WINDOW_TIGHTNESS',
    rule: 'Narrow delivery windows outrank wide ones; they are harder to place tomorrow.',
  },
  {
    rank: 5,
    key: 'FILL_EFFICIENCY',
    rule: 'Among equals, take the largest order that still fits.',
  },
  {
    rank: 6,
    key: 'FLEXIBLE_BRANDS_MOVE',
    rule: 'Style and Tech with flexible windows shift before Fresh, which must arrive before stores open.',
  },
] as const;

export type PrioritisationRuleKey =
  (typeof PRIORITISATION_POLICY)[number]['key'];
