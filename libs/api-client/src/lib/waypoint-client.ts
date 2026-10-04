/**
 * Typed bindings for the Waypoint REST API (`/api/v1`).
 *
 * One method per endpoint in docs/api.md, grouped by the role that uses it.
 * Nothing here contains business logic — feasibility lives in the Go
 * ConstraintValidationService, which is the single authority.
 */

import type {
  AppNotification,
  AssignDriverRequest,
  AssignLoaderRequest,
  AssignManagerRequest,
  AuditQuery,
  AuditRecord,
  Brand,
  ClockTime,
  CloseQueueRequest,
  CloseQueueResponse,
  ConfirmAllocationRequest,
  ConfirmAllocationResponse,
  CreateOrderRequest,
  CreateReceiptRequest,
  CreateUserRequest,
  CustomerOrder,
  DeferralLogEntry,
  DeferralRequest,
  DeliveryEventRequest,
  DemandForecastPoint,
  DemoClockRequest,
  DemoClockResponse,
  DemoResetResponse,
  DemandForecastQuery,
  Depot,
  DockType,
  Driver,
  ForgotPasswordRequest,
  ForgotPasswordResponse,
  IsoDate,
  IsoDateTime,
  Item,
  ListOrdersQuery,
  ListUsersQuery,
  LiveRouteState,
  LoadItemRecord,
  Loader,
  LoginRequest,
  LoginResponse,
  ManagedUser,
  MetaResponse,
  OrderPage,
  OrderQueueQuery,
  Outlet,
  OutletManager,
  OutletWriteRequest,
  ParkingConstraint,
  PlanningJob,
  PlanningResults,
  QueueResponse,
  RecordShortfallRequest,
  ReorderLegsRequest,
  Receipt,
  ResetPasswordRequest,
  ResetPasswordResponse,
  Route,
  RouteLeg,
  StoreManagerOption,
  SuggestPlanRequest,
  SuggestPlanResponse,
  SyncEventsRequest,
  SyncResult,
  TempRequirement,
  TripNumber,
  UpdateUserRequest,
  ValidateAllocationRequest,
  ValidationResponse,
  Vehicle,
  VehicleAssignment,
  VehicleWriteRequest,
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

/** One row of GET /loading/routes — a route the loader's depot must load. */
export interface LoaderRouteSummary {
  readonly routeId: string;
  readonly vehicleId: string;
  readonly depotId: string;
  readonly routeDate: IsoDate;
  readonly tripNo: TripNumber;
  readonly brand: Brand;
  readonly district: string;
  readonly status: string;
  /** Drops on the route, and order lines to count across them. */
  readonly stops: number;
  readonly lines: number;
  /** Lines whose loaded + damaged + missing already equals the ordered qty. */
  readonly linesComplete: number;
  readonly shortfallQty: number;
  readonly routeReady: boolean;
}

/**
 * One order line of the picking list. `seq` is the planned stop order; the
 * loader screen renders it reversed, because the last drop is loaded first and
 * so sits nearest the door.
 */
export interface LoadingLine {
  readonly orderItemId: string;
  readonly orderId: string;
  readonly itemId: string;
  readonly sku: string;
  readonly name: string;
  readonly orderedQty: number;
  readonly loadedQty: number;
  readonly damagedQty: number;
  readonly missingQty: number;
  readonly shortfallQty: number;
  readonly photoRef?: string;
  readonly recordedBy?: string;
  readonly recordedAt?: IsoDateTime;
  readonly seq: number;
  readonly outletId: string;
  readonly outletName: string;
  readonly orderNumber: string;
  readonly dockType: DockType;
  readonly tempRequirement: TempRequirement;
  readonly weightKg: number;
  readonly volumeM3: number;
}

/** GET /routes/{id}/loading — the picking list and the truck being loaded. */
export interface RouteLoading {
  readonly routeId: string;
  readonly vehicleId: string;
  readonly depotId: string;
  readonly routeDate: IsoDate;
  readonly tripNo: TripNumber;
  readonly brand: Brand;
  readonly district: string;
  readonly status: string;
  readonly routeReady: boolean;
  readonly lines: readonly LoadingLine[];
  readonly vehicleType: string;
  readonly vehicleTemp: string;
  readonly weightCapKg: number;
  readonly volumeCapM3: number;
  /** Only the loaded portion of each line — a shortfall lightens the truck. */
  readonly loadedWeightKg: number;
  readonly loadedVolumeM3: number;
  readonly stops: number;
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

  /** Revoke the current session. The presented bearer token names it. */
  logout(): Promise<void> {
    return this.http.post<void>('/auth/logout');
  }

  me(): Promise<LoginResponse['user']> {
    return this.http.get<LoginResponse['user']>('/me');
  }

  /**
   * Request a password-reset link. Always resolves with a generic message
   * whether or not the email exists, so no enumeration is possible. Public.
   */
  forgotPassword(
    body: ForgotPasswordRequest,
  ): Promise<ForgotPasswordResponse> {
    return this.http.post<ForgotPasswordResponse>(
      '/auth/forgot-password',
      body,
      { anonymous: true },
    );
  }

  /**
   * Consume a reset token and set a new password. An invalid, expired or
   * already-used token is a 400 with a `token` field error. Public.
   */
  resetPassword(
    body: ResetPasswordRequest,
  ): Promise<ResetPasswordResponse> {
    return this.http.post<ResetPasswordResponse>(
      '/auth/reset-password',
      body,
      { anonymous: true },
    );
  }

  /** The API clock and demo mode. Public: needed before sign-in for countdowns. */
  meta(): Promise<MetaResponse> {
    return this.http.get<MetaResponse>('/meta', { anonymous: true });
  }

  /* --- Demo mode (dispatcher; 404 unless the API runs with DEMO_MODE) ---- */

  /** Jump the API clock to a walkthrough stage. */
  setDemoClock(body: DemoClockRequest): Promise<DemoClockResponse> {
    return this.http.post<DemoClockResponse>('/demo/clock', body);
  }

  /** Clear operational data, re-seed the demo day and rewind the clock. */
  resetDemo(): Promise<DemoResetResponse> {
    return this.http.post<DemoResetResponse>('/demo/reset');
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

  /** The closed-queue page of CONFIRMED orders for a date and depot. */
  getOrderQueue(query: OrderQueueQuery): Promise<QueueResponse> {
    return this.http.get<QueueResponse>('/orders/queue', {
      query: { ...query },
    });
  }

  /**
   * Freeze the planning queue. `brands` narrows the close to the brands the
   * dispatcher confirmed; omit it to close every brand. Idempotent: closing an
   * already-closed brand writes nothing and is not an error.
   */
  closeQueue(body: CloseQueueRequest): Promise<CloseQueueResponse> {
    return this.http.post<CloseQueueResponse>('/orders/close', body);
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

  /**
   * The depot's confirmed routes. Scope comes from the signed-in loader
   * server-side, so this never takes a depot: a loader sees their own dock and
   * nothing else. Omit `date` to let the server resolve the depot's active
   * run from the routes themselves.
   */
  getLoaderRoutes(date?: IsoDate): Promise<readonly LoaderRouteSummary[]> {
    return this.http
      .get<{ routes: readonly LoaderRouteSummary[] }>('/loading/routes', {
        query: { date },
      })
      .then((r) => r.routes);
  }

  /** The picking list for one route, with its current load state. */
  getRouteLoading(routeId: string): Promise<RouteLoading> {
    return this.http.get<RouteLoading>(`/routes/${routeId}/loading`);
  }

  /**
   * Mints a server-side `shortfall/<orderItemId>/…` key. The key is scoped to
   * the order line, which is why the line is named here and never by the
   * caller: the server refuses a photo ref that belongs to another line.
   */
  createShortfallUpload(
    orderItemId: string,
    contentType: string,
  ): Promise<MediaUpload> {
    return this.http.post<MediaUpload>('/media/uploads', {
      purpose: 'SHORTFALL',
      orderItemId,
      contentType,
    });
  }

  /** Records load counts. Returns the whole route's state after recording. */
  recordShortfall(
    routeId: string,
    body: RecordShortfallRequest,
  ): Promise<RouteLoading> {
    return this.http.post<RouteLoading>(`/routes/${routeId}/shortfalls`, body);
  }

  /* --- Driver ---------------------------------------------------------- */

  /**
   * The depot's driveable routes. Omit `date` to let the server resolve the
   * depot's active run, so the cockpit opens on the planned run rather than a
   * date taken from the phone.
   */
  getDriverRoutes(date?: IsoDate): Promise<readonly DriverRoute[]> {
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

  /** One outlet by id. A store manager may read only their own (404 otherwise). */
  getOutlet(outletId: string): Promise<Outlet> {
    return this.http.get<Outlet>(`/outlets/${outletId}`);
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

  /** One vehicle by id. */
  getVehicle(vehicleId: string): Promise<Vehicle> {
    return this.http.get<Vehicle>(`/vehicles/${vehicleId}`);
  }

  /* --- Dispatcher master data (create/update) -------------------------- */

  /** Create a vehicle. The id is generated server-side. Dispatcher only. */
  createVehicle(body: VehicleWriteRequest): Promise<Vehicle> {
    return this.http.post<Vehicle>('/vehicles', body);
  }

  /** Update a vehicle's mutable fields. The id is immutable. Dispatcher only. */
  updateVehicle(
    vehicleId: string,
    body: VehicleWriteRequest,
  ): Promise<Vehicle> {
    return this.http.patch<Vehicle>(`/vehicles/${vehicleId}`, body);
  }

  /** Create an outlet. The id is generated server-side. Dispatcher only. */
  createOutlet(body: OutletWriteRequest): Promise<Outlet> {
    return this.http.post<Outlet>('/outlets', body);
  }

  /** Update an outlet's mutable fields. The id is immutable. Dispatcher only. */
  updateOutlet(outletId: string, body: OutletWriteRequest): Promise<Outlet> {
    return this.http.patch<Outlet>(`/outlets/${outletId}`, body);
  }

  /* --- Dispatcher operational assignments ------------------------------ */

  /** Active drivers for the assignment picker. Dispatcher only. */
  async listDrivers(query?: { depotId?: string }): Promise<readonly Driver[]> {
    const body = await this.http.get<{ drivers: readonly Driver[] }>(
      '/drivers',
      { query },
    );
    return body.drivers;
  }

  /** Active store managers for the assignment picker. Dispatcher only. */
  async listStoreManagers(): Promise<readonly StoreManagerOption[]> {
    const body = await this.http.get<{
      storeManagers: readonly StoreManagerOption[];
    }>('/store-managers');
    return body.storeManagers;
  }

  /** Active loaders with their current depot. Dispatcher only. */
  async listLoaders(): Promise<readonly Loader[]> {
    const body = await this.http.get<{ loaders: readonly Loader[] }>(
      '/loaders',
    );
    return body.loaders;
  }

  /** The day's driver-vehicle assignments. Dispatcher only. */
  async listVehicleAssignments(
    date?: IsoDate,
  ): Promise<readonly VehicleAssignment[]> {
    const body = await this.http.get<{
      assignments: readonly VehicleAssignment[];
    }>('/driver-vehicle-assignments', { query: { date } });
    return body.assignments;
  }

  /** The driver assigned to a vehicle on a date, or null. Dispatcher only. */
  async getVehicleAssignment(
    vehicleId: string,
    date?: IsoDate,
  ): Promise<VehicleAssignment | null> {
    const body = await this.http.get<{ assignment: VehicleAssignment | null }>(
      `/vehicles/${vehicleId}/assignment`,
      { query: { date } },
    );
    return body.assignment;
  }

  /** Assign or change a vehicle's driver for a date. Dispatcher only. */
  assignDriver(
    vehicleId: string,
    body: AssignDriverRequest,
  ): Promise<VehicleAssignment> {
    return this.http.put<VehicleAssignment>(
      `/vehicles/${vehicleId}/assignment`,
      body,
    );
  }

  /** Remove a vehicle's driver for a date. Dispatcher only. */
  unassignDriver(
    vehicleId: string,
    date: IsoDate,
  ): Promise<VehicleAssignment> {
    return this.http.delete<VehicleAssignment>(
      `/vehicles/${vehicleId}/assignment`,
      { query: { date } },
    );
  }

  /** The store manager responsible for an outlet, or null. Dispatcher only. */
  async getOutletManager(outletId: string): Promise<OutletManager | null> {
    const body = await this.http.get<{ manager: OutletManager | null }>(
      `/outlets/${outletId}/manager`,
    );
    return body.manager;
  }

  /** Assign or change an outlet's store manager. Dispatcher only. */
  assignManager(
    outletId: string,
    body: AssignManagerRequest,
  ): Promise<OutletManager> {
    return this.http.put<OutletManager>(`/outlets/${outletId}/manager`, body);
  }

  /** Remove an outlet's store manager. Dispatcher only. */
  unassignManager(outletId: string): Promise<OutletManager> {
    return this.http.delete<OutletManager>(`/outlets/${outletId}/manager`);
  }

  /** A loader with their current depot, or null. Dispatcher only. */
  async getLoaderDepot(loaderId: string): Promise<Loader | null> {
    const body = await this.http.get<{ loader: Loader | null }>(
      `/loaders/${loaderId}/depot`,
    );
    return body.loader;
  }

  /** Assign or change a loader's depot. Dispatcher only. */
  assignLoader(
    loaderId: string,
    body: AssignLoaderRequest,
  ): Promise<Loader> {
    return this.http.put<Loader>(`/loaders/${loaderId}/depot`, body);
  }

  /** Remove a loader's depot. Dispatcher only. */
  unassignLoader(loaderId: string): Promise<Loader> {
    return this.http.delete<Loader>(`/loaders/${loaderId}/depot`);
  }

  /** The caller's own driver assignment on a date, or null. Driver only. */
  async getOwnDriverAssignment(
    date?: IsoDate,
  ): Promise<VehicleAssignment | null> {
    const body = await this.http.get<{ assignment: VehicleAssignment | null }>(
      '/driver/assignment',
      { query: { date } },
    );
    return body.assignment;
  }

  /** The caller's own loader depot assignment, or null. Loader only. */
  async getOwnLoaderAssignment(): Promise<Loader | null> {
    const body = await this.http.get<{ loader: Loader | null }>(
      '/loader/assignment',
    );
    return body.loader;
  }

  /* --- Dispatcher account management ----------------------------------- */

  /**
   * Operational accounts (DRIVER, LOADER, STORE_MANAGER), optionally filtered
   * by role and active state. Dispatcher only. Never returns credentials.
   */
  async listUsers(query: ListUsersQuery = {}): Promise<readonly ManagedUser[]> {
    const body = await this.http.get<{ users: readonly ManagedUser[] }>(
      '/users',
      { query: { role: query.role, active: query.active } },
    );
    return body.users;
  }

  /** One account by id. Dispatcher only. */
  getUser(userId: string): Promise<ManagedUser> {
    return this.http.get<ManagedUser>(`/users/${userId}`);
  }

  /**
   * Create an operational account. The role is validated server-side; a
   * DISPATCHER role is refused. Dispatcher only.
   */
  createUser(body: CreateUserRequest): Promise<ManagedUser> {
    return this.http.post<ManagedUser>('/users', body);
  }

  /** Edit an account's permitted profile/assignment fields. Dispatcher only. */
  updateUser(userId: string, body: UpdateUserRequest): Promise<ManagedUser> {
    return this.http.patch<ManagedUser>(`/users/${userId}`, body);
  }

  /** Deactivate an account and revoke its sessions. Dispatcher only. */
  deactivateUser(userId: string): Promise<ManagedUser> {
    return this.http.post<ManagedUser>(`/users/${userId}/deactivate`);
  }

  /** Reactivate a deactivated account. Dispatcher only. */
  activateUser(userId: string): Promise<ManagedUser> {
    return this.http.post<ManagedUser>(`/users/${userId}/activate`);
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
