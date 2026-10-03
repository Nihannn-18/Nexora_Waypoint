/**
 * Typed bindings for the Waypoint REST API (`/api/v1`).
 *
 * One method per endpoint in docs/api.md, grouped by the role that uses it.
 * Nothing here contains business logic — feasibility lives in the Go
 * ConstraintValidationService, which is the single authority.
 */

import type {
  Brand,
  ClockTime,
  ConfirmAllocationRequest,
  CreateOrderRequest,
  CreateReceiptRequest,
  CustomerOrder,
  DeferralLogEntry,
  DeferralRequest,
  DeliveryEventRequest,
  DemandForecastPoint,
  DemandForecastQuery,
  DockType,
  IsoDate,
  IsoDateTime,
  LiveRouteState,
  LoadItemRecord,
  LoginRequest,
  LoginResponse,
  MetaResponse,
  Outlet,
  ParkingConstraint,
  PlanningJob,
  PlanningResults,
  RecordShortfallRequest,
  ReorderLegsRequest,
  Receipt,
  Route,
  RouteLeg,
  SuggestPlanRequest,
  SyncEventsRequest,
  SyncResult,
  TempRequirement,
  TripNumber,
  ValidateAllocationRequest,
  ValidationResponse,
  Vehicle,
} from '@waypoint/shared-types';

import { HttpClient, type HttpClientOptions } from './http';

export interface SyncEventOutcome {
  readonly clientEventId: string;
  readonly status: SyncResult;
  readonly serverEventId?: string;
  /** Short, non-sensitive explanation on REJECTED / CONFLICT. */
  readonly reason?: string;
}

/** A route's header as a driver sees it. `status` is the DB route status. */
export interface DriverRouteSummary {
  readonly routeId: string;
  readonly routeDate: IsoDate;
  readonly vehicleId: string;
  readonly tripNo: TripNumber;
  readonly brand: Brand;
  readonly district: string;
  readonly status: string;
}

/** One stop on GET /driver/routes. Absent `plannedArrival` = none stored. */
export interface DriverRouteStop {
  readonly legId: string;
  readonly seq: number;
  readonly outletId: string;
  readonly outletName: string;
  readonly windowOpen: ClockTime;
  readonly windowClose: ClockTime;
  readonly plannedArrival?: IsoDateTime;
  readonly status: string;
}

export interface DriverRoute extends DriverRouteSummary {
  readonly stops: readonly DriverRouteStop[];
}

/** The driver-visible stop (GET /legs/{id}): route, outlet window, orders. */
export interface LegContext {
  readonly legId: string;
  readonly routeId: string;
  readonly depotId: string;
  readonly routeDate: IsoDate;
  readonly toOutletId: string;
  readonly status: string;
  readonly orderIds: readonly string[];
  readonly seq: number;
  readonly plannedArrival?: IsoDateTime;
  readonly route: DriverRouteSummary;
  readonly outlet: {
    readonly outletId: string;
    readonly name: string;
    readonly district: string;
    readonly dockType: DockType;
    readonly parkingConstraint: ParkingConstraint;
    readonly windowOpen: ClockTime;
    readonly windowClose: ClockTime;
    readonly mallWindowOpen?: ClockTime;
    readonly mallWindowClose?: ClockTime;
  };
  readonly orders: readonly {
    readonly orderId: string;
    readonly orderNumber: string;
    readonly tempRequirement: TempRequirement;
    readonly totalUnits: number;
    readonly totalWeightKg: number;
    readonly totalVolumeM3: number;
    readonly lines: readonly {
      readonly orderItemId: string;
      readonly sku: string;
      readonly name: string;
      readonly quantity: number;
    }[];
  }[];
}

/** Where to PUT the bytes of a POD image; `fileRef` goes on the event. */
export interface MediaUpload {
  readonly fileRef: string;
  readonly uploadMode: 'inline' | 'presigned';
  readonly uploadUrl: string;
  readonly headers?: Readonly<Record<string, string>>;
}

export class WaypointClient {
  private readonly http: HttpClient;

  constructor(options: HttpClientOptions) {
    this.http = new HttpClient(options);
  }

  /* --- Shared ------------------------------------------------------------ */

  login(body: LoginRequest): Promise<LoginResponse> {
    return this.http.post<LoginResponse>('/auth/login', body, {
      anonymous: true,
    });
  }

  me(): Promise<LoginResponse['user']> {
    return this.http.get<LoginResponse['user']>('/me');
  }

  /** The API clock and demo mode. Public: needed before sign-in for countdowns. */
  meta(): Promise<MetaResponse> {
    return this.http.get<MetaResponse>('/meta', { anonymous: true });
  }

  /* --- Store Manager ----------------------------------------------------- */

  createOrder(body: CreateOrderRequest): Promise<CustomerOrder> {
    return this.http.post<CustomerOrder>('/orders', body);
  }

  confirmOrder(orderId: string): Promise<CustomerOrder> {
    return this.http.post<CustomerOrder>(`/orders/${orderId}/confirm`);
  }

  getOrder(orderId: string): Promise<CustomerOrder> {
    return this.http.get<CustomerOrder>(`/orders/${orderId}`);
  }

  getOrderEta(
    orderId: string,
  ): Promise<{ eta: string | null; status: string }> {
    return this.http.get(`/orders/${orderId}/eta`);
  }

  createReceipt(orderId: string, body: CreateReceiptRequest): Promise<Receipt> {
    return this.http.post<Receipt>(`/orders/${orderId}/receipt`, body);
  }

  /* --- Dispatcher: queue ------------------------------------------------- */

  getOrderQueue(
    date: IsoDate,
    depotId?: string,
  ): Promise<readonly CustomerOrder[]> {
    return this.http.get<readonly CustomerOrder[]>('/orders/queue', {
      query: { date, depotId },
    });
  }

  closeQueue(date: IsoDate, depotId?: string): Promise<{ closed: number }> {
    return this.http.post('/orders/close', { date, depotId });
  }

  /* --- Dispatcher: planning --------------------------------------------- */

  /** Returns 202 immediately; poll `getPlanningJob` until COMPLETED. */
  suggestPlan(body: SuggestPlanRequest): Promise<PlanningJob> {
    return this.http.post<PlanningJob>('/allocations/suggest', body);
  }

  getPlanningJob(jobId: string): Promise<PlanningJob> {
    return this.http.get<PlanningJob>(`/planning-jobs/${jobId}`);
  }

  getPlanningResults(jobId: string): Promise<PlanningResults> {
    return this.http.get<PlanningResults>(`/planning-jobs/${jobId}/results`);
  }

  validateAllocation(
    body: ValidateAllocationRequest,
  ): Promise<ValidationResponse> {
    return this.http.post<ValidationResponse>('/allocations/validate', body);
  }

  /** Persists the plan transactionally after a full server-side revalidation. */
  confirmAllocation(body: ConfirmAllocationRequest): Promise<{
    routeIds: readonly string[];
    deferralIds: readonly string[];
  }> {
    return this.http.post('/allocations/confirm', body);
  }

  /* --- Dispatcher: routes ----------------------------------------------- */

  getRoute(routeId: string): Promise<Route> {
    return this.http.get<Route>(`/routes/${routeId}`);
  }

  getRouteLegs(routeId: string): Promise<readonly RouteLeg[]> {
    return this.http.get<readonly RouteLeg[]>(`/routes/${routeId}/legs`);
  }

  /** Rejected with 409 when `routeVersion` is stale. */
  reorderLegs(routeId: string, body: ReorderLegsRequest): Promise<Route> {
    return this.http.patch<Route>(`/routes/${routeId}/legs/reorder`, body);
  }

  dispatchRoute(routeId: string): Promise<Route> {
    return this.http.post<Route>(`/routes/${routeId}/dispatch`);
  }

  getLiveRoutes(date?: IsoDate): Promise<readonly LiveRouteState[]> {
    return this.http.get<readonly LiveRouteState[]>('/routes/live', {
      query: { date },
    });
  }

  /* --- Dispatcher: deferrals ------------------------------------------- */

  getDeferrals(query?: {
    date?: IsoDate;
    outletId?: string;
  }): Promise<readonly DeferralLogEntry[]> {
    return this.http.get<readonly DeferralLogEntry[]>('/deferrals', { query });
  }

  deferOrder(
    orderId: string,
    body: Omit<DeferralRequest, 'orderId'>,
  ): Promise<DeferralLogEntry> {
    return this.http.post<DeferralLogEntry>(`/deferrals/${orderId}`, body);
  }

  /* --- Dispatcher: forecast -------------------------------------------- */

  getDemandForecast(query: DemandForecastQuery): Promise<{
    series: readonly DemandForecastPoint[];
  }> {
    return this.http.get('/forecast/demand', { query: { ...query } });
  }

  /* --- Loader ---------------------------------------------------------- */

  recordShortfall(
    routeId: string,
    body: RecordShortfallRequest,
  ): Promise<{ routeReady: boolean; updatedItems: readonly LoadItemRecord[] }> {
    return this.http.post(`/routes/${routeId}/shortfalls`, body);
  }

  /* --- Driver ---------------------------------------------------------- */

  /** Depot-scoped: every live route of the driver's depot on `date`. */
  getDriverRoutes(date: IsoDate): Promise<readonly DriverRoute[]> {
    return this.http.get<readonly DriverRoute[]>('/driver/routes', {
      query: { date },
    });
  }

  getLeg(legId: string): Promise<LegContext> {
    return this.http.get<LegContext>(`/legs/${legId}`);
  }

  recordDeliveryEvent(
    legId: string,
    body: DeliveryEventRequest,
  ): Promise<SyncEventOutcome> {
    return this.http.post<SyncEventOutcome>(`/legs/${legId}/events`, body);
  }

  /** Mints a server-side `pod/<legId>/…` key; never accepts a client key. */
  createPodUpload(legId: string, contentType: string): Promise<MediaUpload> {
    return this.http.post<MediaUpload>('/media/uploads', {
      purpose: 'POD',
      legId,
      contentType,
    });
  }

  /** Batch upload for events captured offline. Each is resolved independently. */
  syncEvents(
    body: SyncEventsRequest,
  ): Promise<{ results: readonly SyncEventOutcome[] }> {
    return this.http.post('/sync/events', body);
  }

  getSyncStatus(): Promise<{ synced: number; conflicts: number }> {
    return this.http.get('/sync/status');
  }

  /* --- Reference data -------------------------------------------------- */

  getOutlets(query?: {
    depotId?: string;
    brand?: string;
  }): Promise<readonly Outlet[]> {
    return this.http.get<readonly Outlet[]>('/outlets', { query });
  }

  getVehicles(query?: {
    depotId?: string;
    date?: IsoDate;
  }): Promise<readonly Vehicle[]> {
    return this.http.get<readonly Vehicle[]>('/vehicles', { query });
  }
}
