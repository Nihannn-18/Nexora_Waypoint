# Architecture

Required Hackathon deliverable: `docs/` must contain an architecture diagram, a data model
and an AI tool disclosure.

---

## Components

```
┌────────────────────────────────────────────────────────────────────┐
│                    Next.js 16 · App Router · PWA                   │
│                                                                    │
│  /dispatcher      /loader        /driver         /store            │
│  1440 desktop     1024 tablet    402 phone       desktop/phone     │
│  D-01…D-09        L-01…L-T3      R-01…R-04       S-01…S-07         │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ @waypoint/ui          tokens, StatusBadge, Mono, Offline     │  │
│  │ @waypoint/api-client  typed REST, offline-aware errors       │  │
│  │ @waypoint/shared-types  the contract: codes, DTOs, formula   │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  Driver only: IndexedDB outbox ──► POST /sync/events               │
└───────────────────────────────┬────────────────────────────────────┘
                                │ HTTPS · opaque bearer session · /api/v1
┌───────────────────────────────┴────────────────────────────────────┐
│                          Go 1.24 REST API                          │
│                                                                    │
│  httpx         router, strict JSON decoding, one error shape       │
│  auth*         opaque bearer sessions, Argon2id, RBAC, scope       │
│  useradmin     Dispatcher account mgmt + self-service reset        │
│  catalog       outlets, vehicles, calendar, travel, allowance      │
│  assignment    driver→vehicle, loader→depot, manager→outlet        │
│  orders        lifecycle, 16:00 cutoff, queue close, totals        │
│  planning      ▸ ConstraintValidator — the only feasibility rule   │
│                ▸ TripTimeCalculator — the official formula         │
│                ▸ PlanningEngine + job runner, fuel, windows        │
│  routes        confirm, legs, ETA, deferral log, optimistic lock   │
│  loading       reverse-order lists, line-level shortfall flags     │
│  delivery      events, POD, idempotent offline reconciliation      │
│  receipts      store GRN, DELIVERED → RECEIVED                     │
│  notify        notifications to users and outlets                  │
│  media         photo/signature uploads, scope-checked              │
│  audit         immutable operational history                       │
│  clock, demo   injected time, demo stages and reset                │
│  store, seed   pgx pool, goose migrations, embedded CSV seed       │
└────────┬──────────────────────────────────────────┬────────────────┘
         │                                          │ in-process call
┌────────┴─────────────────┐          ┌─────────────┴────────────────┐
│       PostgreSQL 17      │          │    Planning job (inline)     │
│                          │          │                              │
│  authoritative facts     │◄─────────│  loads orders + reference    │
│  planning_result (draft) │          │  runs PlanningEngine         │
│  allocation (final)      │          │  writes planning_result      │
│  audit_log (immutable)   │          │  QUEUED→RUNNING→COMPLETED    │
└──────────────────────────┘          └──────────────────────────────┘
```

---

## Authentication

The Go API owns authentication with opaque, database-backed sessions. `POST /api/v1/auth/login`
verifies the submitted password against `app_user.password_hash` (Argon2id, constant-time) and
mints a random 256-bit session token; only its SHA-256 hash is stored in `session`. The web
client keeps the raw token and sends it as `Authorization: Bearer <token>` on every request.

`internal/auth` extracts the bearer token and `internal/authstore` hashes it, looks up a live,
unexpired session and loads role and depot/outlet scope from `app_user`. The same RBAC middleware
every handler uses then enforces scope. Missing/invalid/expired → `401`; authenticated but no
active `app_user` → `403`. Client-supplied identity headers are never trusted. `POST
/api/v1/auth/logout` deletes the caller's session row. Media endpoints resolve the same identity
and enforce role, purpose and depot/outlet scope before any byte moves.

The four demo accounts are seeded with a hashed password (`DEMO_SEED_PASSWORD`, default
`waypoint2026`).

`internal/useradmin` sits on top of the same credential model. A Dispatcher creates operational
accounts (DRIVER, LOADER, STORE_MANAGER) with an Argon2id-hashed initial password, edits their
profile/assignment fields, and deactivates them (revoking their sessions) — never deleting them.
The public forgot/reset endpoints mint a short-lived, single-use token whose SHA-256 hash alone
is stored in `password_reset_token`; a reset sets the new hash, consumes the token and revokes
the user's sessions in one transaction. There is no second identity system and no password is
ever returned.

---

## The five rules this architecture is built around

**PostgreSQL stores facts, not conclusions.** Remaining capacity and remaining minutes are
derived from a route and its current orders on every read. Persisting them would mean a
route edit could leave a stale number that looks authoritative, and the dispatcher would
plan against a lie.

**One authority on feasibility.** Capacity, refrigeration, access, depot, trip count, time
budgets, windows, fuel and duplicate assignment are implemented once, in the Go constraint
validator. The dispatcher's board greys out impossible moves, but that is convenience only —
`POST /allocations/confirm` revalidates the entire plan against current database state inside
the transaction that persists it. A plan the client believed was valid is still rejected if
the facts moved underneath it.

**Suggestions are never allocations.** A planning run writes `planning_result` rows and
nothing else. Only dispatcher confirmation creates `route`, `route_leg` and `allocation`
rows. This is what makes assisted planning safe: a suggestion can be wrong, discarded or
ignored without any cleanup.

**Every decision is attributable.** Deferrals, load shortfalls, route edits and delivery
outcomes are timestamped and tied to a person. The system's purpose is not only to plan but
to answer _why was this outlet skipped_ — and to make it visible when an outlet is skipped
twice in a row.

**Offline is a normal state.** The Driver's surface works from cache. Events are captured
with a `client_event_id` generated before the network call and with the device's own clock;
they upload in capture order and keep their original timestamps. Duplicate uploads are
resolved by a unique constraint on `client_event_id`, so a flaky connection cannot
double-record a delivery, and a later plan change never overwrites what the driver observed.

---

## Request flows

### Placing an order

```
Store manager → POST /orders
  ↓ validate outlet authorisation, item existence, brand consistency, cutoff
  ↓ snapshot each SKU's weight and volume at order time
  ↓ totals = Σ quantity × snapshot;  temperature = chilled if ANY line is
  ↓ generate order number
PLACED → POST /orders/{id}/confirm → CONFIRMED
```

SKU dimensions are snapshotted so a later catalogue correction cannot retroactively change a
historical order's weight — and therefore cannot retroactively invalidate a plan that was
feasible when it was made.

A Fresh outlet may hold **two orders for one delivery day**, one chilled and one dry. They
are never merged: they have different vehicle requirements.

### Planning a day

```
16:00  POST /orders/close          queue frozen; later orders held for the next run
       POST /allocations/suggest   → 202 { jobId, QUEUED }
                                     │ runs in-process (see note below)
                                     ▼
                                   planning.Service
                                     ├ group by depot + brand + district
                                     ├ candidate vehicles: home depot, available,
                                     │   capacity, temperature, access
                                     ├ build trip 1 and trip 2
                                     ├ validate every hard rule
                                     ├ trip time, windows, ETA, fuel
                                     ├ rank by the documented policy
                                     └ write planning_result (SERVE | DEFER)
       GET  /planning-jobs/{id}         poll until COMPLETED
       GET  /planning-jobs/{id}/results board + "why this plan"
            dispatcher reviews and edits
       POST /allocations/validate       per-move, per-rule results
       POST /allocations/confirm        ← revalidate-all, then persist, in one txn
                                          route + route_leg + allocation + deferral_log
                                          → notify loader and stores
```

The job API (`202 {jobId}` → poll) is the contract. Today the engine runs inline inside
`planning.Service.Suggest`: it is pure and fast at this dataset size, and the job row is
already `COMPLETED` when the client first polls. RabbitMQ is in `docker-compose.yml` but no
AMQP consumer is wired yet; a worker can replace the inline call without changing the API.

### Driver offline and reconnection

```
offline:  capture outcome
          ├ clientEventId = uuid()      ← generated BEFORE any network attempt
          ├ occurredAt    = device clock
          └ store in IndexedDB outbox, created_offline = true

online:   POST /sync/events  [events in capture order]
          per event, independently:
            new clientEventId      → ACCEPTED
            seen clientEventId     → DUPLICATE   (no second write)
            leg changed under it   → CONFLICT    (surfaced to the dispatcher,
                                                  never silently overwritten)
            malformed              → REJECTED
```

Idempotency is a unique index on `delivery_event.client_event_id`. Retrying the whole batch
is always safe, which is what lets the client retry aggressively on a bad connection.

---

## Degradation

| Scenario                   | Design | Behaviour                                                                                                                                                                                                                             |
| -------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Demand exceeds the fleet   | DG-A   | Every unserved order is deferred with a binding constraint and a reason. Publishing is blocked until each has one; one reason can be applied in bulk to all orders blocked by the same rule. Affected stores are notified by default. |
| Driver loses signal        | DG-B   | The surface works from cache. The indicator is neutral grey, never red. Events queue locally and upload in order, keeping device timestamps.                                                                                          |
| Vehicle breaks down        | DG-C   | The dispatcher marks it unavailable. Only **unserved** stops are re-queued; a recommended replacement is shown with every constraint check visible, and deferral is the fallback.                                                     |
| Loader finds a shortfall   | —      | Recorded against the specific order line before departure, never as a trip-level note. The dispatcher is notified and route readiness can be blocked.                                                                                 |
| Plan changes while loading | —      | Optimistic locking on `route_version` plus a visible change indicator. A stale write is rejected, so the loader never works from a silently outdated list.                                                                            |
| Planning run fails         | —      | The job becomes `FAILED` with its error, and no partial plan is persisted. (Retry and dead-lettering arrive with the AMQP worker.)                                                                                                    |

---

## Technology choices

| Layer    | Choice                            | Why this one                                                                                                          |
| -------- | --------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Frontend | Next.js 16, React 19, TypeScript  | App Router suits four distinct role shells; strict TypeScript against a shared contract catches drift at compile time |
| Styling  | Tailwind v4                       | Day 5 tokens as CSS custom properties — each colour defined once                                                      |
| Offline  | Service worker + IndexedDB        | The only reliable way to keep a phone useful without signal                                                           |
| Backend  | Go 1.24                           | Static binary, fast start, small container; see the README for the departure from Spring Boot                         |
| Routing  | `net/http` ServeMux               | Go 1.22+ method-and-path patterns cover this API; no dependency to justify                                            |
| Database | PostgreSQL 17                     | Transactions, `JSONB` for constraint results, ISO week functions for the forecast                                     |
| Queue    | RabbitMQ (provisioned, not wired) | Named in the specification; the planning job runs in-process until the AMQP consumer lands                            |
| Monorepo | Nx                                | One install and one command surface across both languages, with caching and `nx affected`                             |

---

## Limits worth stating

- **Trip time uses the official free-flow formula**, not a routing API. The brief requires
  the supplied `district_travel` and `service_allowance` data and forbids proprietary
  API-based modelling for the Datathon. Live traffic is therefore not modelled.
- **Realtime is polling, at a 30-second interval** on the dispatcher's trip tracker, as the
  Day 5 design specifies. WebSockets would reduce latency but add a failure mode for a gain
  nobody in this workflow needs.
- **The forecast endpoint (`GET /forecast/demand`) is specified but not built** in this
  phase (first on the cut line). It is intentionally decoupled from the Datathon
  model. The two phases are judged separately, so the model stays modular rather than
  embedded.
