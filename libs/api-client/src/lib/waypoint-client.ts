/**
 * Typed bindings for the Waypoint REST API (`/api/v1`).
 *
 * One method per endpoint in docs/api.md, grouped by the role that uses it.
 * Nothing here contains business logic — feasibility lives in the Go
 * ConstraintValidationService, which is the single authority.
 */

import type {
  AppNotification,
  AuditQuery,
  AuditRecord,
  ConfirmAllocationRequest,
  ConfirmAllocationResponse,
  CreateOrderRequest,
  CreateReceiptRequest,
  CustomerOrder,
  DeferralLogEntry,
  DeferralRequest,
  DeliveryEvent,
  DeliveryEventRequest,
  DemandForecastPoint,
  DemandForecastQuery,
  Depot,
  IsoDate,
  Item,
  ListOrdersQuery,
  LiveRouteState,
  LoadItemRecord,
  LoginRequest,
  LoginResponse,
  MetaResponse,
  OrderPage,
  Outlet,
  PlanningJob,
  PlanningResults,
  RecordShortfallRequest,
  ReorderLegsRequest,
  Receipt,
  Route,
  RouteLeg,
  SuggestPlanRequest,
  SuggestPlanResponse,
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

  /**
   * One page of orders, filtered server-side. A store manager is pinned to
   * their outlet by the server; a dispatcher sees every outlet.
   */
  listOrders(query: ListOrdersQuery = {}): Promise<OrderPage> {
    return this.http.get<OrderPage>('/orders', { query: { ...query } });
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
  suggestPlan(body: SuggestPlanRequest): Promise<SuggestPlanResponse> {
    return this.http.post<SuggestPlanResponse>('/allocations/suggest', body);
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

  /**
   * Persists the plan in one transaction after revalidating it against current
   * state. A retry of an already-confirmed plan is a 409, never a duplicate.
   */
  confirmAllocation(
    body: ConfirmAllocationRequest,
  ): Promise<ConfirmAllocationResponse> {
    return this.http.post<ConfirmAllocationResponse>(
      '/allocations/confirm',
      body,
    );
  }

  /* --- Dispatcher: routes ----------------------------------------------- */

  /** Confirmed routes for a date, with their legs, ordered by vehicle and trip. */
  async listRoutes(query: {
    date: IsoDate;
    depotId?: string;
  }): Promise<readonly Route[]> {
    const body = await this.http.get<{ routes: readonly Route[] }>('/routes', {
      query,
    });
    return body.routes;
  }

  getRoute(routeId: string): Promise<Route> {
    return this.http.get<Route>(`/routes/${routeId}`);
  }

  async getRouteLegs(routeId: string): Promise<readonly RouteLeg[]> {
    const body = await this.http.get<{ legs: readonly RouteLeg[] }>(
      `/routes/${routeId}/legs`,
    );
    return body.legs;
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

  /** The full deferral history, newest first; `outletId` narrows server-side. */
  async getDeferrals(query?: {
    outletId?: string;
  }): Promise<readonly DeferralLogEntry[]> {
    const body = await this.http.get<{
      deferrals: readonly DeferralLogEntry[];
    }>('/deferrals', { query });
    return body.deferrals;
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

  /* --- Audit and notifications ---------------------------------------- */

  /** Dispatcher-only, newest first. Page with `limit` (≤ 200) and `offset`. */
  async listAudit(query: AuditQuery = {}): Promise<readonly AuditRecord[]> {
    const body = await this.http.get<{ records: readonly AuditRecord[] }>(
      '/audit',
      { query: { ...query } },
    );
    return body.records;
  }

  /** The caller's own notifications, newest first. */
  async listNotifications(query?: {
    limit?: number;
    offset?: number;
  }): Promise<readonly AppNotification[]> {
    const body = await this.http.get<{
      notifications: readonly AppNotification[];
    }>('/notifications', { query });
    return body.notifications;
  }

  async getUnreadNotificationCount(): Promise<number> {
    const body = await this.http.get<{ unread: number }>(
      '/notifications/unread-count',
    );
    return body.unread;
  }

  /** Idempotent: marking an already-read notification is not an error. */
  async markNotificationRead(id: string): Promise<void> {
    await this.http.post(`/notifications/${id}/read`);
  }

  /** Returns how many notifications changed from unread to read. */
  async markAllNotificationsRead(): Promise<number> {
    const body = await this.http.post<{ markedRead: number }>(
      '/notifications/read-all',
    );
    return body.markedRead;
  }

  /* --- Reference data -------------------------------------------------- */

  /** The depots, with the internal `depotId` planning and routes filter by. */
  async getDepots(): Promise<readonly Depot[]> {
    const body = await this.http.get<{ depots: readonly Depot[] }>('/depots');
    return body.depots;
  }

  async getOutlets(query?: { depotId?: string }): Promise<readonly Outlet[]> {
    const body = await this.http.get<{ outlets: readonly Outlet[] }>(
      '/outlets',
      { query },
    );
    return body.outlets;
  }

  /** Vehicles with their availability on `date` (default: today on the API clock). */
  async getVehicles(query?: {
    depotId?: string;
    date?: IsoDate;
  }): Promise<readonly Vehicle[]> {
    const body = await this.http.get<{ vehicles: readonly Vehicle[] }>(
      '/vehicles',
      { query },
    );
    return body.vehicles;
  }

  /** The SKU catalogue, used to name order lines. */
  async listItems(query?: {
    brand?: string;
    search?: string;
  }): Promise<readonly Item[]> {
    const body = await this.http.get<{ items: readonly Item[] }>('/items', {
      query,
    });
    return body.items;
  }
}
