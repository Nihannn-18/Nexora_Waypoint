/**
 * Core domain vocabulary for the Waypoint Delivery Planning System.
 *
 * These names are the contract between the Go API and the Next.js client.
 * The Go side mirrors them as string constants — if you rename anything here,
 * rename it there in the same commit. See docs/data-model.md.
 *
 * Wire format rule: every enum travels as the UPPER_SNAKE string, never as an
 * ordinal, so a mismatch fails loudly instead of silently shifting meaning.
 */

/* -------------------------------------------------------------------------- */
/* Roles                                                                      */
/* -------------------------------------------------------------------------- */

/** The four roles a judge must be able to complete the workflow across. */
export const ROLES = [
  'DISPATCHER',
  'LOADER',
  'DRIVER',
  'STORE_MANAGER',
] as const;
export type Role = (typeof ROLES)[number];

/* -------------------------------------------------------------------------- */
/* Network, brands, vehicles                                                  */
/* -------------------------------------------------------------------------- */

/** The three brands sharing one fleet. */
export const BRANDS = ['FRESH', 'STYLE', 'TECH'] as const;
export type Brand = (typeof BRANDS)[number];

/** The two depots. Vehicles serve only outlets of their home depot. */
export const DEPOT_CODES = ['PELIYAGODA', 'KANDY'] as const;
export type DepotCode = (typeof DEPOT_CODES)[number];

export const VEHICLE_TYPES = ['TRUCK', 'VAN'] as const;
export type VehicleType = (typeof VEHICLE_TYPES)[number];

/**
 * Vehicle temperature capability. A REEFER may also carry ambient goods;
 * an AMBIENT vehicle may never carry chilled or frozen.
 */
export const VEHICLE_TEMP_CLASSES = ['AMBIENT', 'REEFER'] as const;
export type VehicleTempClass = (typeof VEHICLE_TEMP_CLASSES)[number];

/** Temperature an order requires. CHILLED and FROZEN both demand a reefer. */
export const TEMP_REQUIREMENTS = ['AMBIENT', 'CHILLED', 'FROZEN'] as const;
export type TempRequirement = (typeof TEMP_REQUIREMENTS)[number];

/** Outlet unloading access. Drives the service allowance lookup. */
export const DOCK_TYPES = ['REAR_DOCK', 'STREET', 'MALL_BAY'] as const;
export type DockType = (typeof DOCK_TYPES)[number];

/**
 * `parking_constraint` in outlets.csv. One column, three values — not two
 * independent flags:
 *   NORMAL    any vehicle may serve the outlet
 *   VAN_ONLY  trucks cannot reach it; only a van may be allocated
 *   MALL_DOCK access is limited to the mall's fixed window (`mallWindow`)
 */
export const PARKING_CONSTRAINTS = ['NORMAL', 'VAN_ONLY', 'MALL_DOCK'] as const;
export type ParkingConstraint = (typeof PARKING_CONSTRAINTS)[number];

export const VEHICLE_STATUSES = [
  'AVAILABLE',
  'IN_WORKSHOP',
  'UNAVAILABLE',
  'BROKEN_DOWN',
] as const;
export type VehicleStatus = (typeof VEHICLE_STATUSES)[number];

/** A vehicle may run at most two trips per day. */
export const TRIP_NUMBERS = [1, 2] as const;
export type TripNumber = (typeof TRIP_NUMBERS)[number];

/* -------------------------------------------------------------------------- */
/* Order lifecycle                                                            */
/* -------------------------------------------------------------------------- */

/**
 * PLACED → CONFIRMED → ALLOCATED → LOADED → IN_TRANSIT → DELIVERED → RECEIVED
 *                           └──────────────→ DEFERRED → next planning run
 *
 * IN_TRANSIT can also produce FAILED. DELAYED is a delivery *outcome*, not a
 * terminal order state — a delayed stop is still expected to be delivered.
 */
export const ORDER_STATUSES = [
  'PLACED',
  'CONFIRMED',
  'ALLOCATED',
  'LOADED',
  'IN_TRANSIT',
  'DELIVERED',
  'FAILED',
  'RECEIVED',
  'DEFERRED',
] as const;
export type OrderStatus = (typeof ORDER_STATUSES)[number];

/**
 * Legal forward transitions. The Go API is authoritative, but the client uses
 * this to grey out impossible actions before the user taps them.
 */
export const ORDER_STATUS_TRANSITIONS: Readonly<
  Record<OrderStatus, readonly OrderStatus[]>
> = {
  PLACED: ['CONFIRMED', 'DEFERRED'],
  CONFIRMED: ['ALLOCATED', 'DEFERRED'],
  ALLOCATED: ['LOADED', 'DEFERRED'],
  LOADED: ['IN_TRANSIT', 'DEFERRED'],
  IN_TRANSIT: ['DELIVERED', 'FAILED'],
  DELIVERED: ['RECEIVED'],
  FAILED: ['DEFERRED'],
  RECEIVED: [],
  DEFERRED: ['CONFIRMED'], // re-enters a later planning run
} as const;

export function canTransition(from: OrderStatus, to: OrderStatus): boolean {
  return ORDER_STATUS_TRANSITIONS[from].includes(to);
}

/** Whether an order has left the planning queue for good on its planned day. */
export function isTerminalForDay(status: OrderStatus): boolean {
  return status === 'RECEIVED' || status === 'DEFERRED' || status === 'FAILED';
}

/* -------------------------------------------------------------------------- */
/* Routes, legs, deliveries                                                   */
/* -------------------------------------------------------------------------- */

export const ROUTE_STATUSES = [
  'DRAFT',
  'CONFIRMED',
  'DISPATCHED',
  'LOADING',
  'LOADED',
  'IN_TRANSIT',
  'COMPLETED',
  'CANCELLED',
] as const;
export type RouteStatus = (typeof ROUTE_STATUSES)[number];

export const LEG_STATUSES = [
  'PENDING',
  'EN_ROUTE',
  'ARRIVED',
  'DELIVERED',
  'FAILED',
  'SKIPPED',
] as const;
export type LegStatus = (typeof LEG_STATUSES)[number];

/** What the Driver records at a stop. */
export const DELIVERY_OUTCOMES = ['DELIVERED', 'FAILED', 'DELAYED'] as const;
export type DeliveryOutcome = (typeof DELIVERY_OUTCOMES)[number];

export const POD_TYPES = ['PHOTO', 'SIGNATURE', 'NONE'] as const;
export type PodType = (typeof POD_TYPES)[number];

/** Result of reconciling one offline-captured event. Never silently overwrite. */
export const SYNC_RESULTS = [
  'ACCEPTED',
  'DUPLICATE',
  'REJECTED',
  'CONFLICT',
] as const;
export type SyncResult = (typeof SYNC_RESULTS)[number];

/* -------------------------------------------------------------------------- */
/* Receipts and shortfalls                                                    */
/* -------------------------------------------------------------------------- */

export const RECEIPT_STATUSES = [
  'RECEIVED',
  'RECEIVED_WITH_ISSUE',
  'REJECTED',
] as const;
export type ReceiptStatus = (typeof RECEIPT_STATUSES)[number];

export const ISSUE_TYPES = ['DAMAGED', 'SHORT', 'WRONG_ITEM', 'LATE'] as const;
export type IssueType = (typeof ISSUE_TYPES)[number];

/* -------------------------------------------------------------------------- */
/* Planning jobs                                                              */
/* -------------------------------------------------------------------------- */

export const PLANNING_JOB_STATUSES = [
  'QUEUED',
  'RUNNING',
  'COMPLETED',
  'FAILED',
] as const;
export type PlanningJobStatus = (typeof PLANNING_JOB_STATUSES)[number];

/** A suggestion row either proposes service or proposes deferral. */
export const PLANNING_DECISIONS = ['SERVE', 'DEFER'] as const;
export type PlanningDecision = (typeof PLANNING_DECISIONS)[number];
