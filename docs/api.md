# REST API

Base path `/api/v1`.

**Authentication.** The Go API owns authentication with opaque, database-backed sessions.
`POST /api/v1/auth/login` verifies the email and Argon2id password hash against `app_user` and
returns a random session token (returned once; only its SHA-256 hash is stored in `session`).
The client sends it as `Authorization: Bearer <token>` on every subsequent call. The Go API
hashes the presented token, looks up a live, unexpired session, loads role and depot/outlet
scope from `app_user`, and enforces RBAC and per-depot/outlet/route scope. A missing, malformed,
unknown or expired token is `401`; an authenticated user with no active `app_user` is `403`.
No client-supplied identity header (`X-User-ID`, `X-Role`, …) is ever trusted.

Handlers stay thin: no planning arithmetic in a handler. Every calculation happens in
`internal/planning` or the constraint validator, which are unit-tested without a server.

**Scope is enforced server-side on every request.** A store manager's queries are restricted
to their outlet, and a loader's and driver's to their depot and routes, derived from the user
record — never from a client-supplied parameter, and never from a token claim alone. The
dispatcher plans **both** depots from Peliyagoda, so dispatcher queries take a `depotId`
filter rather than being pinned to one depot.

---

## Error shape

One shape for every failure, so the client never has to special-case an HTML error page.

```json
{
  "message": "Allocation is not feasible",
  "code": "CONSTRAINT_VIOLATION",
  "constraintResults": [{ "code": "REEFER_REQUIRED", "passed": false, "detail": "VEH014 is a dry-box vehicle" }]
}
```

| Status | Meaning                                                                                                             |
| ------ | ------------------------------------------------------------------------------------------------------------------- |
| `400`  | Malformed body, unknown field (`BAD_REQUEST`), or failed field validation (`VALIDATION_FAILED`, with `fieldErrors`) |
| `401`  | Missing or invalid session (`UNAUTHENTICATED`)                                                                      |
| `403`  | Authenticated but not permitted (`FORBIDDEN`) — wrong role, or out of depot/outlet/route scope                      |
| `404`  | Not found, or found but out of scope                                                                                |
| `409`  | Optimistic-lock conflict — `routeVersion` is stale                                                                  |
| `422`  | Request is well-formed but infeasible; `constraintResults` says which rule blocked it                               |

`401` and `403` are produced by the reusable auth middleware in
`apps/api/internal/auth` and carry the machine codes `UNAUTHENTICATED` and `FORBIDDEN`. The
messages are deliberately generic — no token, session or scope value is ever echoed.

A `VALIDATION_FAILED` body lists every invalid field at once so forms can mark each in place:
`{ "message": "...", "code": "VALIDATION_FAILED", "fieldErrors": [{ "field": "weightKg", "message": "weightKg must be greater than zero" }] }`.
An unexpected server fault is `500` with code `INTERNAL_ERROR`; the cause is logged, never returned.

`constraintResults` returns **every** rule verdict, passed and failed, not only the failures.
The dispatcher's rule panel shows the full picture rather than just the first objection.

---

## Shared

| Method | Endpoint       | Role                      | Purpose                                                                                                                                      |
| ------ | -------------- | ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST` | `/auth/login`  | all                       | Verify email + password; return an opaque session token and the user's role/scope                                                            |
| `POST` | `/auth/logout` | all authenticated         | Revoke the caller's current session (the presented bearer token)                                                                             |
| `GET`  | `/me`          | all authenticated         | Current user with depot or outlet scope                                                                                                      |
| `GET`  | `/depots`      | dispatcher                | The depots with their internal `depotId` (the id planning, routes and audit filter by)                                                       |
| `GET`  | `/outlets`     | dispatcher, store manager | Access, window, brand, district. `?depotId=` narrows to one depot; a store manager is pinned to their own outlet                             |
| `GET`  | `/vehicles`    | dispatcher                | Availability on `?date=` (default: today on the API clock), capacity, temperature, depot, fuel. `?depotId=` narrows                          |
| `GET`  | `/items`       | all authenticated         | Catalogue SKUs: dimensions and temperature requirement. Read-only                                                                            |
| `GET`  | `/items/{id}`  | all authenticated         | One SKU by `itemId`, or by `?sku=`                                                                                                           |
| `GET`  | `/healthz`     | —                         | Liveness. Does **not** touch the database: a database blip must not make the orchestrator kill a healthy API                                 |
| `GET`  | `/readyz`      | —                         | Readiness. Pings PostgreSQL and checks RabbitMQ is reachable; `503` and `dependencies: {database, queue}` (`up`/`down`) name the failing one |

### `GET /items`

Read-only catalogue. Returns every SKU the order and capacity rules are checked against.
Optional query parameters: `brand` (FRESH|STYLE|TECH), `temperature` (AMBIENT|CHILLED|FROZEN)
and `search` (case-insensitive match on SKU or name). Results are ordered by SKU.

```json
{
  "items": [
    {
      "itemId": "a3f1…",
      "sku": "FRESH-0001",
      "name": "Red lentils 1kg",
      "brand": "FRESH",
      "unitWeightKg": 1.0,
      "unitVolumeM3": 0.0012,
      "temperatureRequirement": "AMBIENT"
    }
  ]
}
```

An unknown `brand` or `temperature` value is a `400 VALIDATION_FAILED`. There is no write
endpoint: the catalogue is reference data seeded from the authoritative source, not edited
through the API.

### Network reference: `GET /depots`, `GET /outlets`, `GET /vehicles`

**Implemented** (`internal/catalog/network.go`). `GET /outlets` is available to
dispatchers and store managers; `GET /depots` and `GET /vehicles` are dispatcher-only.
Read-only reference data; no feasibility rule is evaluated here. Each response is an
envelope:

A store manager is pinned to the outlet on their `app_user` record: the handler ignores any
client-supplied `depotId` or outlet query parameter and returns only the caller's own outlet,
so scope can never be widened through the request. A dispatcher reads the whole network,
optionally narrowed by `?depotId=`.

```json
{ "depots": [{ "depotId": "2bcc…", "code": "PELIYAGODA", "name": "Peliyagoda" }] }
```

```json
{
  "outlets": [
    {
      "outletId": "OUT015",
      "name": "Outlet OUT015",
      "brand": "STYLE",
      "district": "Colombo",
      "depotId": "2bcc…",
      "dockType": "MALL_BAY",
      "parkingConstraint": "MALL_DOCK",
      "mallWindow": { "open": "09:00", "close": "11:00" },
      "windowOpenTime": "09:00",
      "windowCloseTime": "11:00"
    }
  ]
}
```

```json
{
  "vehicles": [
    {
      "vehicleId": "VEH001",
      "type": "TRUCK",
      "tempClass": "REEFER",
      "weightCapKg": 5510,
      "volumeCapM3": 26.4,
      "fuelType": "diesel",
      "kmPerL": 4.7,
      "weeklyFuelQuotaL": 340,
      "depotId": "2bcc…",
      "status": "IN_WORKSHOP"
    }
  ]
}
```

`mallWindow` is `null` outside malls. A vehicle's `status` is its availability on `date`; a
vehicle with no availability row for that day is `AVAILABLE` (the planning loader's
convention), and a stored `BREAKDOWN` is sent as the wire enum `BROKEN_DOWN`. A malformed
`date` is `400`; a `depotId` that matches nothing returns an empty list.

---

## Demo mode

The seeded delivery day is in the past and the judge walkthrough spans Friday 16:00 to Saturday
morning, so the API runs on an injected clock rather than the wall clock. Enabled with
`DEMO_MODE=true`; the clock starts at `DEMO_CLOCK_START` and ticks from there. `DEMO_MODE`
is off unless set (`.env.example` sets it on for the walkthrough); `DEMO_CLOCK_START` is
RFC 3339 with offset and defaults to `2026-09-25T15:40:00+05:30`. With demo mode off, `now` is
the wall clock in the business timezone.

`GET /meta` → `200`, public:

```json
{ "now": "2026-09-25T15:40:00+05:30", "demoMode": true, "timezone": "Asia/Colombo" }
```

| Method | Endpoint      | Role       | Purpose                                                                                                                                                                   |
| ------ | ------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/meta`       | —          | Returns `now` (API clock, ISO-8601 with offset), `demoMode`, `timezone`. The web app derives every countdown and "today" from `now`, never from the browser clock         |
| `POST` | `/demo/clock` | dispatcher | `{ "stage": "BEFORE_CUTOFF" \| "AFTER_CUTOFF" \| "LOADING" \| "ON_ROUTE" }` — jump the clock to Fri 15:40, Fri 16:05, Sat 03:30 or Sat 05:00. `404` when demo mode is off |
| `POST` | `/demo/reset` | dispatcher | Re-seed the demo day and reset the clock. `404` when demo mode is off                                                                                                     |

---

## Store manager

| Method | Endpoint               | Purpose                            | Server must                                                                                                                                                                           |
| ------ | ---------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/orders`              | Filtered, paged order list         | Apply every filter in SQL and count the same filtered set; pin a store manager to their own outlet                                                                                    |
| `POST` | `/orders`              | Create an order with its lines     | Authorise the outlet; check items exist and brands match; enforce the 16:00 cutoff; snapshot SKU dimensions; compute units, weight, volume and temperature; generate the order number |
| `POST` | `/orders/{id}/confirm` | Confirm before cutoff              | Compare against 16:00 Asia/Colombo and the operating calendar                                                                                                                         |
| `GET`  | `/orders/{id}`         | Order, lines, allocation, ETA      | Restrict to the caller's outlet                                                                                                                                                       |
| `GET`  | `/orders/{id}/eta`     | Expected arrival                   | Compute from current route state and remaining stops, including waiting for a window that has not opened                                                                              |
| `POST` | `/orders/{id}/receipt` | Confirm receipt or report an issue | Validate received quantities against delivered; record damage and shortage; transition the order                                                                                      |

### `POST /orders`

```json
{
  "outletId": "OUT001",
  "requestedDeliveryDate": "2026-09-26",
  "items": [
    { "itemId": "ITEM001", "quantity": 20 },
    { "itemId": "ITEM014", "quantity": 5 }
  ],
  "notes": "Optional"
}
```

```json
{
  "orderId": "a3f1…",
  "orderNumber": "ORD-2026-000153",
  "outletId": "OUT001",
  "brand": "FRESH",
  "orderDate": "2026-09-25",
  "requestedDeliveryDate": "2026-09-26",
  "status": "PLACED",
  "totalUnits": 25,
  "totalWeightKg": 123.5,
  "totalVolumeM3": 1.42,
  "temperatureRequirement": "AMBIENT",
  "afterCutoff": false,
  "lines": [
    {
      "orderItemId": "…",
      "itemId": "ITEM001",
      "quantity": 20,
      "unitWeightKgSnapshot": 1.0,
      "unitVolumeM3Snapshot": 0.0012,
      "totalWeightKg": 20.0,
      "totalVolumeM3": 0.024
    }
  ],
  "deferral": {
    "reasonText": "Refrigerated capacity ran out for this district.",
    "constraintCode": "FRESH_TIME_BUDGET",
    "decidedAt": "2026-09-25T16:20:00+05:30",
    "deferredToDate": "2026-09-28"
  }
}
```

`deferral` is present only when the order has a `deferral_log` row; it is the latest
decision, so a store manager reading their own order sees why it was deferred (and the run
it moved to) without dispatcher-only access. It is loaded on the scoped read, so it never
widens what a caller may see.

Totals are never accepted from the client. They are computed from the lines and the
catalogue, because they are what the capacity rules are checked against. Each line snapshots
the SKU's unit weight and volume, so a later catalogue edit cannot change a historical order's
totals.

### `GET /orders`

Dispatcher and store manager (loaders and drivers reach orders through their routes: `403`).
Query parameters, all optional and all applied server-side: `deliveryDate` (`YYYY-MM-DD`),
`status`, `brand`, `depotId`, `district`, `outletId`, `search` (case-insensitive match on the
order number or outlet id), `limit` (1–200, default 50) and `offset`. Newest first.

```json
{ "orders": [{ "orderId": "…", "orderNumber": "S1-001", "…": "same shape as GET /orders/{id}" }], "total": 85, "limit": 50, "offset": 0 }
```

`total` counts the whole filtered set with the caller's scope applied, so a page can say
"showing 1–50 of 85" truthfully. An unknown `status` or `brand`, a malformed `deliveryDate`,
or an out-of-range `limit`/`offset` is `400 VALIDATION_FAILED` naming the field. A store
manager is pinned to their outlet; asking for another outlet is `404`.

**Implemented:** `GET /orders`, `POST /orders`, `GET /orders/{id}` and
`POST /orders/{id}/confirm` are mounted. A store manager may only order for and read their own outlet; a dispatcher may act
across outlets. `POST /orders/{id}/confirm` moves `PLACED`/`DEFERRED` to `CONFIRMED` and returns
`409 CONFLICT` if the order is already past that point. `GET /orders/{id}/eta` and
`POST /orders/{id}/receipt` are **not** implemented — they need route state and delivery
outcomes owned by later agents.

---

## Dispatcher — queue

| Method | Endpoint              | Purpose                              | Server must                                                              |
| ------ | --------------------- | ------------------------------------ | ------------------------------------------------------------------------ |
| `POST` | `/orders/close`       | Freeze the planning queue for a date | Hold post-cutoff orders for the next run rather than rejecting them      |
| `GET`  | `/orders/queue?date=` | Closed, confirmed orders             | Return current authoritative totals and the outlet fields the rules need |

### `POST /orders/close`

**Implemented**, dispatcher-only (`internal/orders/handler.go`). Freezes the planning queue for a
delivery date and depot. Closing is **per brand** so one brand can be frozen while another keeps
taking orders; `brands` omitted closes all three. It is idempotent: closing an already-closed
brand writes nothing and is not an error, and the response reports which brands were already
closed. The actor comes from the authenticated session, never the body.

```json
{ "date": "2026-09-26", "depotId": "2bcc…", "brands": ["FRESH", "STYLE"] }
```

```json
{
  "date": "2026-09-26",
  "depotId": "2bcc…",
  "brands": ["FRESH", "STYLE"],
  "closed": 2,
  "alreadyClosed": [],
  "queue": { "orders": [], "total": 0, "limit": 200, "offset": 0 }
}
```

A missing `date` or `depotId`, or an unknown `brand`, is `400 VALIDATION_FAILED`. A non-dispatcher
caller is `400` (the route requires the dispatcher role). The closure is recorded in
`order_queue_close` (PK `queue_date, depot_id, brand`); `GET /orders/queue` reads it back as
`closedBrands`, so the board can go read-only and disable Close.

### `GET /orders/queue`

**Implemented**, dispatcher-only. Returns the CONFIRMED orders for a date and depot — the frozen
queue — plus the brands whose queue is already closed. Query parameters: `date` (required),
`depotId`, `brand`, `district`.

```json
{
  "orders": [ { "orderId": "…", "status": "CONFIRMED", "…": "same shape as GET /orders/{id}" } ],
  "total": 85,
  "limit": 200,
  "offset": 0,
  "closedBrands": ["FRESH"]
}
```

---

## Dispatcher — planning

| Method | Endpoint                         | Purpose                       | Server must                                                                                                                                                                             |
| ------ | -------------------------------- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST` | `/allocations/suggest`           | Start an assisted run         | Return `202` immediately with a job id; defer only inside the suggestion, never as a final allocation                                                                                   |
| `GET`  | `/planning-jobs/{jobId}`         | Job status                    | —                                                                                                                                                                                       |
| `GET`  | `/planning-jobs/{jobId}/results` | The suggested plan            | Include per-rule results and an explanation per row                                                                                                                                     |
| `POST` | `/allocations/validate`          | Test one candidate move       | Run the **complete** validator against current database state and return every rule result plus remaining resources                                                                     |
| `POST` | `/allocations/recalculate`       | Recompute a draft after edits | Recompute weight, volume, outbound, inter-stop, handling, total minutes, ETA, windows, fuel and daily budgets                                                                           |
| `POST` | `/allocations/confirm`           | Persist the final plan        | Reload authoritative state, **revalidate the whole plan inside the transaction**, persist routes, legs and allocations, update order states, write deferral logs, publish notifications |
| `GET`  | `/allocations/{date}`            | Confirmed allocation summary  | —                                                                                                                                                                                       |

**Implemented:** `POST /allocations/suggest`, `GET /planning-jobs/{jobId}`,
`GET /planning-jobs/{jobId}/results` (planning, dispatcher-only) and, from the Routes/Allocation
foundation, `POST /allocations/confirm`, `GET /routes`, `GET /routes/{id}`,
`GET /routes/{id}/legs`, `GET /deferrals` (dispatcher-only). Planning writes **proposals only**
(`planning_result`); confirmation is the only writer of `route`, `route_leg` and `allocation`.
`POST /allocations/validate` and `/recalculate` remain **not** implemented. See
[Confirmation](#confirmation) for the transaction, idempotency and allocation-invariant rules.

### Confirmation

`POST /api/v1/allocations/confirm` takes the dispatcher's chosen `routes` and `deferrals` for a
planning job. It validates the choice against current state, then writes routes, legs,
allocations, order-status transitions and deferral-log rows in **one transaction**. The orders in
the confirmation are row-locked (`SELECT ... FOR UPDATE`, deterministic order); an order already
actively allocated on the route date is rejected with `409 CONFLICT`. A retry of the same
confirmation conflicts rather than duplicating. Every order that planning proposed to `SERVE`
must be either allocated or deferred, or the request is `400 VALIDATION_FAILED`; every deferral
must carry a reason. `route`/`route_leg`/`allocation` uniqueness constraints and the order row
lock are the protections — no partial `(order_id, route_date)` index exists or is added.

### `POST /allocations/suggest` → `202`

`depotId` is the depot's internal UUID (the stable identifier; `depot.code` is a display/matching
key, not the id). The suggestion runs in-process and completes before the response, but the job
is still created and polled through the job endpoints, so the asynchronous contract holds and a
future worker can replace the inline run without an API change.

```json
{ "planningDate": "2026-09-26", "depotId": "DEP-PELIYAGODA" }
```

```json
{ "jobId": "PLAN-0001", "status": "QUEUED" }
```

### `POST /allocations/validate` → `200`

```json
{ "orderId": "ORD-S1-014", "vehicleId": "VEH014", "tripNo": 1 }
```

```json
{
  "valid": true,
  "violations": [],
  "results": [
    { "code": "WEIGHT_CAPACITY_EXCEEDED", "passed": true },
    { "code": "REEFER_REQUIRED", "passed": true },
    { "code": "FRESH_TIME_BUDGET", "passed": true }
  ],
  "remainingWeightKg": 820.0,
  "remainingVolumeM3": 4.2,
  "remainingFreshMinutes": 112,
  "remainingStyleTechMinutes": 480,
  "eta": "07:34"
}
```

`200` with `valid: false` for a move the dispatcher is _probing_; `422` when a move is
actually attempted. The board probes continuously as the dispatcher drags, so a probe is
not an error.

### `POST /allocations/confirm`

```json
{
  "jobId": "PLAN-0001",
  "routes": [{ "vehicleId": "VEH014", "tripNo": 1, "orderIds": ["ORD-S1-014", "ORD-S1-021"] }],
  "deferrals": [
    {
      "orderId": "ORD-S1-033",
      "reasonType": "CONSTRAINT",
      "constraintCode": "FRESH_TIME_BUDGET",
      "reasonText": "No feasible Fresh trip remains within the 270-minute vehicle budget.",
      "deferredToDate": "2026-09-28"
    }
  ]
}
```

Rejected with `422` if any confirmed order is neither allocated nor deferred. Every unserved
order must carry a reason — this is the requirement the whole submission turns on.

---

## Dispatcher — routes and live view

| Method  | Endpoint                    | Purpose                     | Server must                                                                                               |
| ------- | --------------------------- | --------------------------- | --------------------------------------------------------------------------------------------------------- |
| `POST`  | `/routes`                   | Create a draft route        | Validate vehicle, date and trip uniqueness, availability, depot, brand and district, and initial capacity |
| `GET`   | `/routes/{id}`              | Route with stops and totals | —                                                                                                         |
| `GET`   | `/routes/{id}/legs`         | Stop-sequenced list         | Include order, lines, windows, handling, ETA and load state — the Loader and Driver both read this        |
| `PATCH` | `/routes/{id}/legs/reorder` | Reorder stops               | Recompute sequence, travel, handling, ETA, windows and budgets; reject a stale `routeVersion` with `409`  |
| `POST`  | `/routes/{id}/dispatch`     | Mark in transit             | Require a confirmed route, loading readiness and no blocking shortfall                                    |
| `GET`   | `/routes/live`              | Live progress               | Aggregate the latest driver events and legs into current state per route                                  |

---

## Dispatcher — deferrals and forecast

| Method | Endpoint               | Purpose                                                                 |
| ------ | ---------------------- | ----------------------------------------------------------------------- |
| `GET`  | `/deferrals`           | History: reason, date, order, outlet, who decided                       |
| `POST` | `/deferrals/{orderId}` | Manual or override deferral; writes an audit row and notifies the store |
| `GET`  | `/forecast/demand`     | Demand by depot × brand × ISO week                                      |

### `GET /forecast/demand?depotId=…&brand=Fresh&weekFrom=2026-W41&weekTo=2026-W50`

```json
{
  "series": [
    {
      "depotId": "DEP-PELIYAGODA",
      "brand": "FRESH",
      "isoWeek": "2026-W41",
      "predTotalVolumeM3": 412.7,
      "predChilledVolumeM3": 151.3
    }
  ]
}
```

Volume only, not converted to vehicle or driver counts. Only Fresh carries chilled demand;
Style and Tech are `0`. The Datathon model stays behind this endpoint and is not embedded in
the app — the two phases are judged separately.

---

## Loader

| Method | Endpoint                  | Purpose                         | Server must                                                                                                                                            |
| ------ | ------------------------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET`  | `/loading/routes?date=`   | The depot's routes to load      | Return the caller depot's `CONFIRMED` routes on `date`, with stop/line counts and progress. Depot comes from the session, never the query               |
| `GET`  | `/routes/{id}/loading`    | The picking list                | Return stops with their order lines and current load state. The **client renders in reverse stop order** — last delivery loads first, nearest the door |
| `POST` | `/routes/{id}/shortfalls` | Record missing or damaged items | Read ordered quantities **from the database**, not the request; require `loaded + damaged + missing = ordered` per line; notify the dispatcher         |

**Implemented:** both endpoints are loader-only (`LOADER` role, depot-scoped). The picking list
is served at `GET /routes/{id}/loading` rather than `/legs`, because `GET /routes/{id}/legs` is
the dispatcher route/stop view; the loader needs the order lines and load state alongside the
stops. A route must be `CONFIRMED`. The response includes `routeReady`, computed as "every line
reconciles". `notify the dispatcher` is a Delivery/Notifications agent concern and is **not** yet
implemented — the recorded shortfall is the trigger a later agent will publish from.

### `GET /api/v1/loading/routes?date=YYYY-MM-DD`

The loader's way in. Nothing else exposes a route to a loader — `GET /routes` is
dispatcher-only — so without this the picking list could only be reached by already knowing a
route id. `date` is required and validated; the depot is taken from the signed-in loader, and a
caller with no depot gets an empty list rather than every depot's work.

```json
{
  "routes": [
    {
      "routeId": "…", "vehicleId": "VEH014", "depotId": "…",
      "routeDate": "2026-09-26", "tripNo": 1, "brand": "FRESH",
      "district": "Colombo", "status": "CONFIRMED",
      "stops": 6, "lines": 18, "linesComplete": 12,
      "shortfallQty": 2, "routeReady": false
    }
  ]
}
```

`linesComplete` counts lines where `loaded + damaged + missing = ordered`, so the list shows
progress without the client re-deriving it. `routeReady` is the same rule the picking list
reports: every line reconciles, and a route with no lines is never ready.

### `GET /routes/{routeId}/loading` — response fields

Each line carries its stop as well as its item, because the loader works stop by stop and the
screen labels every row with its drop:

```json
{
  "routeId": "…", "vehicleId": "VEH014", "tripNo": 1, "status": "CONFIRMED",
  "routeReady": false, "stops": 6,
  "vehicleType": "TRUCK", "vehicleTemp": "REEFER",
  "weightCapKg": 6500, "volumeCapM3": 22,
  "loadedWeightKg": 4820, "loadedVolumeM3": 18.2,
  "lines": [
    {
      "orderItemId": "…", "sku": "WF-MLK-01", "name": "Fresh Milk 1L",
      "orderedQty": 16, "loadedQty": 14, "damagedQty": 1, "missingQty": 1,
      "shortfallQty": 2, "photoRef": "shortfall/<orderItemId>/…",
      "seq": 3, "outletId": "OUT014", "outletName": "…", "orderNumber": "S1-001",
      "dockType": "REAR_DOCK", "tempRequirement": "CHILLED",
      "weightKg": 32, "volumeM3": 0.8
    }
  ]
}
```

Lines come back in planned stop order (`seq` ascending); the **client reverses them**, because
the last drop is loaded first and must sit deepest in the vehicle. `loadedWeightKg` and
`loadedVolumeM3` count only the loaded portion of each line, so a shortfall lightens the truck;
with the vehicle's capacity they give the payload and volume meters their denominator.

### `POST /routes/{routeId}/shortfalls`

```json
{ "items": [{ "orderItemId": "OI1", "loadedQty": 18, "damagedQty": 0, "missingQty": 2 }] }
```

```json
{ "routeId": "R001", "routeReady": false, "dispatcherNotificationCreated": true }
```

A shortfall is always recorded against a specific order line, never as a trip-level note:
the dispatcher needs to know _which_ SKU is short to decide whether the stop can still go.

---

## Media (proof photos)

Proof-of-delivery and shortfall photos are the only uploaded media. There is **no generic
file-upload endpoint**: every object key is generated server-side from a `purpose` and a
business owner, and the caller is authorised against that owner before any key is minted or
any byte is read.

The backend is chosen by configuration (`MEDIA_STORAGE`):

- **`local`** (Docker Compose default) — files are written under `MEDIA_ROOT`; upload and
  download both go through the API's own routes below, and the "upload URL" is same-origin.
- **`s3`** — files live in a **private** S3 bucket. `POST /media/uploads` returns a presigned
  `PUT` URL; `GET /media/{key}` returns a presigned `GET` URL. The bucket is never public.

Object keys have the shape `<purpose>/<ownerId>/<uuid>` (`pod/LEG1/…`, `shortfall/OI1/…`),
so a key is self-describing and can never be repurposed; a client-supplied key is never
trusted.

| Method | Endpoint         | Role                   | Purpose                                             | Server must                                                                                |
| ------ | ---------------- | ---------------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `POST` | `/media/uploads` | loader, driver         | Request an upload slot for a shortfall or POD photo | Authorise the caller against the owner; generate the key server-side; return an upload URL |
| `PUT`  | `/media/{key}`   | loader, driver         | Upload bytes to a server-minted key (local backend) | Re-authorise; reject traversal keys and non-images; cap the body size                      |
| `GET`  | `/media/{key}`   | dispatcher, store, etc | Read an authorised object (local backend)           | Re-authorise against the key's purpose/owner; stream with the stored content type          |

### `POST /media/uploads`

```json
{ "purpose": "POD", "legId": "LEG1", "contentType": "image/jpeg" }
```

For a shortfall photo, send `orderItemId` instead of `legId`:

```json
{ "purpose": "SHORTFALL", "orderItemId": "OI1", "contentType": "image/jpeg" }
```

```json
{ "fileRef": "pod/LEG1/9f2c…", "uploadMode": "inline", "uploadUrl": "/api/v1/media/pod/LEG1/9f2c…" }
```

With `MEDIA_STORAGE=s3` the response carries a presigned URL and the headers to send:

```json
{
  "fileRef": "pod/LEG1/9f2c…",
  "uploadMode": "presigned",
  "uploadUrl": "https://waypoint-media.s3.ap-southeast-1.amazonaws.com/pod/LEG1/9f2c…?X-Amz-…",
  "headers": { "Content-Type": "image/jpeg" }
}
```

`contentType` must be one of `image/jpeg`, `image/png`, `image/webp`, `image/heic`.
`purpose` must be `SHORTFALL` or `POD`. Anything else is `400`.

### `PUT /media/{key}` (local backend)

Upload the image bytes with the matching `Content-Type`. Returns `{ "fileRef": "<key>" }`.
Bodies over 8 MiB are rejected with `413`. The object is written to a temp file and renamed,
so a partial upload never appears under the real key.

### `GET /media/{key}` (local backend)

Streams the object with its recorded content type. Out-of-scope reads return `404` (found but
not yours) or `403` per the standard error rules; a missing object is `404`.

**Association.** A POD photo's `fileRef` is stored on the delivery event (`delivery_event.pod_photo`).
A shortfall photo's `fileRef` is stored on the order line (`load_item.photo_ref`). The `POST
/legs/{id}/events` and `POST /routes/{id}/shortfalls` payloads carry the `fileRef` returned
here; there is no separate "attach media" step.

> Authentication for these endpoints uses the same Go opaque bearer session as the rest of
> the API. Uploads are restricted to the producing role and its depot scope: `SHORTFALL`
> uploads require `LOADER` and an order line in the caller's depot, `POD` uploads require
> `DRIVER` and a leg in the caller's depot. Reads are allowed for the dispatcher (both
> depots), the producing role within its depot, and a store manager for an object belonging
> to its outlet. A wrong-purpose, cross-scope or unknown-owner request is `403`.

---

## Driver

| Method | Endpoint            | Purpose                                  | Server must                                                                                                           |
| ------ | ------------------- | ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `POST` | `/legs/{id}/events` | Record an outcome with proof of delivery | Check driver authorisation and leg ownership; enforce idempotency on `clientEventId`; update actual arrival and delay |
| `POST` | `/sync/events`      | Batch upload of offline events           | Process each event **independently**; return accepted, duplicate, rejected or conflict without ever double-applying   |
| `GET`  | `/sync/status`      | Pending, synced and failed counts        | —                                                                                                                     |
| `GET`  | `/driver/routes?date=` | The depot's routes and stops on a date | Depot-scoped (no driver→vehicle link exists in the schema); `DRAFT`/`CANCELLED` routes excluded; `date` is `YYYY-MM-DD` |
| `GET`  | `/legs/{id}`        | One stop: route, outlet window, orders and lines | Depot-scoped; another depot's leg is `404`, never its data                                                       |

### `POST /legs/{legId}/events`

```json
{
  "clientEventId": "EV-LOCAL-001",
  "outcome": "DELIVERED",
  "occurredAt": "2026-09-26T07:42:00+05:30",
  "createdOffline": true,
  "deliveredItems": [{ "orderItemId": "OI1", "quantity": 18 }],
  "proofOfDelivery": { "type": "PHOTO", "fileRef": "pod/EV-LOCAL-001.jpg" }
}
```

`clientEventId` is generated on the device **before** any network attempt. That is what
makes a retry safe, and it is enforced by a unique index rather than by a check-then-insert,
which would race.

`occurredAt` is the device's clock at capture time and is preserved through sync. An event
recorded offline at 07:42 and uploaded at 11:15 is recorded as having happened at 07:42.

A `FAILED` outcome **requires** `reasonCode`, one of `DELIVERY_FAILURE_REASONS`
(`OUTLET_CLOSED`, `ACCESS_BLOCKED`, `REFUSED_BY_STORE`, `GOODS_DAMAGED`, `OTHER`); it is stored on
`delivery_event.reason_code` and in the audit detail. Only a `FAILED` outcome may carry one. A
missing, unknown, or misapplied code is a `400` validation error (or `REJECTED` inside
`/sync/events`).

### `GET /driver/routes?date=2026-09-26`

```json
[
  {
    "routeId": "R1", "routeDate": "2026-09-26", "vehicleId": "VEH014", "tripNo": 1,
    "brand": "FRESH", "district": "Colombo", "status": "DISPATCHED",
    "stops": [
      { "legId": "LEG1", "seq": 0, "outletId": "OUT014", "outletName": "…",
        "windowOpen": "05:00", "windowClose": "08:00", "status": "PENDING" }
    ]
  }
]
```

### `GET /legs/{id}`

The fields above (`legId`, `routeId`, `depotId`, `routeDate`, `toOutletId`, `status`, `orderIds`)
plus `seq`, `route` (the summary above), `outlet` (`name`, `district`, `dockType`,
`parkingConstraint`, `windowOpen`, `windowClose`, optional `mallWindowOpen`/`mallWindowClose`) and
`orders` (`orderNumber`, `tempRequirement`, totals and `lines` of `sku`, `name`, `quantity`).
`plannedArrival` is `route_leg.planned_arrival` and is omitted when unset. No ETA, distance or
location is derived here.

`GET /driver/routes` is depot-scoped and the schema links no driver to a vehicle, so the API
cannot say which route is the caller's. The Driver PWA therefore remembers an explicit run choice
on the device and, when the depot has more than one route, asks rather than guessing — a driver is
never silently shown another vehicle's stops. A depot with exactly one live route is unambiguous
and selected automatically.

### `POST /sync/events`

```json
{ "events": [{ "legId": "LEG-1", "clientEventId": "EV1", "…": "…" }] }
```

```json
{
  "results": [
    { "clientEventId": "EV1", "status": "ACCEPTED", "serverEventId": "E1" },
    { "clientEventId": "EV2", "status": "DUPLICATE", "serverEventId": "E2" }
  ]
}
```

`CONFLICT` is returned when the leg changed underneath a queued event — reallocated after a
breakdown, say. The event is surfaced to the dispatcher rather than silently discarded or
silently applied; both of those lose information a human needs.

**Implemented (delivery foundation):** `POST /legs/{id}/events`, `POST /sync/events`,
`GET /sync/status` and `GET /legs/{id}` are mounted, driver-only and depot-scoped. `delivery_event`
is the authoritative record; idempotency is the unique `client_event_id`. `DELIVERED` requires a
receiver name and a POD artefact (photo `pod/<legId>/…` or signature); `FAILED`/`DELAYED` do not;
`FAILED` requires a `reasonCode`. An event updates the leg and order status in one transaction.
The media upload endpoints authenticate the same Go bearer session and enforce role/purpose/depot
scope, so a POD photo can be uploaded end-to-end (create upload → PUT bytes → attach `fileRef` to
the event). `GET /sync/status` reports server-synced counts and a `0` conflict count (there is no
separate conflict store). The Driver PWA reads `GET /driver/routes` and `GET /legs/{id}`, caches
both in IndexedDB, and uploads its outbox through `/sync/events`.

---

## Audit

| Method | Endpoint        | Role       | Purpose                                              |
| ------ | --------------- | ---------- | ---------------------------------------------------- |
| `GET`  | `/api/v1/audit` | dispatcher | Append-only operational audit trail; filter and page |

Query params: `actor`, `action`, `entityType`, `entityId`, `depotId`, `from`
(RFC 3339), `to` (RFC 3339), `limit` (≤200, default 50), `offset`. Dispatcher-only —
there is no admin role, and audit is not exposed to Loader/Driver/Store Manager. A
dispatcher sees both depots and narrows with `depotId`. Response:
`{ "records": [{ id, actor?, role?, action, entityType, entityId?, depotId?, outletId?, result?, detail?, occurredAt }] }`.
Records are written by the business transaction they describe (route/load/delivery), so a
rolled-back mutation leaves none. No secrets, tokens or request bodies are stored.

---

## Notifications

| Method | Endpoint                             | Role              | Purpose                                             |
| ------ | ------------------------------------ | ----------------- | --------------------------------------------------- |
| `GET`  | `/api/v1/notifications`              | any authenticated | List the caller's notifications (`limit`, `offset`) |
| `GET`  | `/api/v1/notifications/unread-count` | any authenticated | Unread count                                        |
| `POST` | `/api/v1/notifications/{id}/read`    | any authenticated | Mark one read (idempotent)                          |
| `POST` | `/api/v1/notifications/read-all`     | any authenticated | Mark all read                                       |

Every endpoint is scoped to the authenticated recipient: a user sees notifications addressed to
their user id, or (for an outlet-scoped account) to their outlet. A user cannot read or mark
another user's notification (`404`). Read state is persisted in `read_at`. Operational
notifications are raised by existing workflows: a loading shortfall and a FAILED/DELAYED
delivery each notify the depot's dispatchers; creation is idempotent on
`(user_id, type, reference)`, so a retried event does not duplicate. There is no external
provider (no email/SMS/push).

---

## Client bindings

Every endpoint above has a typed method on `WaypointClient`
(`libs/api-client/src/lib/waypoint-client.ts`). Adding an endpoint means three edits in one
commit: the Go handler, the client method, and a row in this file.
