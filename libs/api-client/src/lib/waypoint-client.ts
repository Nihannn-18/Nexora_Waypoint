/**
 * Typed bindings for the Waypoint REST API (`/api/v1`).
 *
 * One method per endpoint in docs/api.md, grouped by the role that uses it.
 * Nothing here contains business logic — feasibility lives in the Go
 * ConstraintValidationService, which is the single authority.
 */

import type {
  ConfirmAllocationRequest,
  CreateOrderRequest,
  CreateReceiptRequest,
  CustomerOrder,
  DeferralLogEntry,
  DeferralRequest,
  DeliveryEvent,
  DeliveryEventRequest,
  DemandForecastPoint,
  DemandForecastQuery,
  IsoDate,
  LiveRouteState,
  LoadItemRecord,
  LoginRequest,
  LoginResponse,
  MetaResponse,
  Outlet,
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
  ValidateAllocationRequest,
  ValidationResponse,
  Vehicle,
} from '@waypoint/shared-types';

import { HttpClient, type HttpClientOptions } from './http';

export interface SyncEventOutcome {
  readonly clientEventId: string;
  readonly status: SyncResult;
  readonly serverEventId?: string;
  readonly message?: string;
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

  recordDeliveryEvent(
    legId: string,
    body: DeliveryEventRequest,
  ): Promise<DeliveryEvent> {
    return this.http.post<DeliveryEvent>(`/legs/${legId}/events`, body);
  }

  /** Batch upload for events captured offline. Each is resolved independently. */
  syncEvents(
    body: SyncEventsRequest,
  ): Promise<{ results: readonly SyncEventOutcome[] }> {
    return this.http.post('/sync/events', body);
  }

  getSyncStatus(): Promise<{
    pending: number;
    synced: number;
    failed: number;
  }> {
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
