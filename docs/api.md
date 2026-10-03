# REST API

Base path `/api/v1`. JWT bearer token on everything except `POST /auth/login`.

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

| Status | Meaning                                                                               |
| ------ | ------------------------------------------------------------------------------------- |
| `400`  | Malformed body, unknown field, or a failed field validation                           |
| `401`  | Missing, expired or invalid token                                                     |
| `403`  | Authenticated but out of scope — another depot's route, another outlet's order        |
| `404`  | Not found, or found but out of scope                                                  |
| `409`  | Optimistic-lock conflict — `routeVersion` is stale                                    |
| `422`  | Request is well-formed but infeasible; `constraintResults` says which rule blocked it |

`constraintResults` returns **every** rule verdict, passed and failed, not only the failures.
The dispatcher's rule panel shows the full picture rather than just the first objection.

---

## Shared

| Method | Endpoint      | Role                      | Purpose                                                                                                      |
| ------ | ------------- | ------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `POST` | `/auth/login` | all                       | `{email, password}` → JWT, user, role, scope                                                                 |
| `GET`  | `/me`         | all                       | Current user with depot or outlet scope                                                                      |
| `GET`  | `/outlets`    | dispatcher, store manager | Access, window, brand, district                                                                              |
| `GET`  | `/vehicles`   | dispatcher                | Availability, capacity, temperature, depot, fuel                                                             |
| `GET`  | `/healthz`    | —                         | Liveness. Does **not** touch the database: a database blip must not make the orchestrator kill a healthy API |
| `GET`  | `/readyz`     | —                         | Readiness. Pings PostgreSQL and RabbitMQ; `503` names the failing dependency                                 |

---

## Demo mode

The seeded delivery day is in the past and the judge walkthrough spans Friday 16:00 to Saturday
morning, so the API runs on an injected clock rather than the wall clock. Enabled with
`DEMO_MODE=true`; the clock starts at `DEMO_CLOCK_START` and ticks from there.

| Method | Endpoint      | Role       | Purpose                                                                                                                                                                   |
| ------ | ------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/meta`       | —          | Returns `now` (API clock, ISO-8601 with offset), `demoMode`, `timezone`. The web app derives every countdown and "today" from `now`, never from the browser clock         |
| `POST` | `/demo/clock` | dispatcher | `{ "stage": "BEFORE_CUTOFF" \| "AFTER_CUTOFF" \| "LOADING" \| "ON_ROUTE" }` — jump the clock to Fri 15:40, Fri 16:05, Sat 03:30 or Sat 05:00. `404` when demo mode is off |
| `POST` | `/demo/reset` | dispatcher | Re-seed the demo day and reset the clock. `404` when demo mode is off                                                                                                     |

---

## Store manager

| Method | Endpoint               | Purpose                            | Server must                                                                                                                                                                           |
| ------ | ---------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
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
  "status": "PLACED",
  "totalUnits": 25,
  "totalWeightKg": 123.5,
  "totalVolumeM3": 1.42,
  "temperatureRequirement": "AMBIENT",
  "afterCutoff": false
}
```

Totals are never accepted from the client. They are computed from the lines and the
catalogue, because they are what the capacity rules are checked against.

---

## Dispatcher — queue

| Method | Endpoint              | Purpose                              | Server must                                                              |
| ------ | --------------------- | ------------------------------------ | ------------------------------------------------------------------------ |
| `POST` | `/orders/close`       | Freeze the planning queue for a date | Hold post-cutoff orders for the next run rather than rejecting them      |
| `GET`  | `/orders/queue?date=` | Closed, confirmed orders             | Return current authoritative totals and the outlet fields the rules need |

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

### `POST /allocations/suggest` → `202`

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

| Method | Endpoint                  | Purpose                         | Server must                                                                                                                                    |
| ------ | ------------------------- | ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/routes/{id}/legs`       | The picking list                | Return stops with their order lines. The **client renders in reverse stop order** — last delivery loads first, nearest the door                |
| `POST` | `/routes/{id}/shortfalls` | Record missing or damaged items | Read ordered quantities **from the database**, not the request; require `loaded + damaged + missing = ordered` per line; notify the dispatcher |

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

## Driver

| Method | Endpoint            | Purpose                                  | Server must                                                                                                           |
| ------ | ------------------- | ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `POST` | `/legs/{id}/events` | Record an outcome with proof of delivery | Check driver authorisation and leg ownership; enforce idempotency on `clientEventId`; update actual arrival and delay |
| `POST` | `/sync/events`      | Batch upload of offline events           | Process each event **independently**; return accepted, duplicate, rejected or conflict without ever double-applying   |
| `GET`  | `/sync/status`      | Pending, synced and failed counts        | —                                                                                                                     |

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

---

## Client bindings

Every endpoint above has a typed method on `WaypointClient`
(`libs/api-client/src/lib/waypoint-client.ts`). Adding an endpoint means three edits in one
commit: the Go handler, the client method, and a row in this file.
