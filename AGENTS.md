# AGENTS.md — Waypoint Delivery Planning System

**Team Nexora · Rootcode Tech-Triathlon 2026 · Hackathon**  
**Stack: Go 1.24 REST backend, Next.js 16/React 19/TypeScript frontend, PostgreSQL 17, RabbitMQ 4, Nx 23/npm monorepo.**  
**Deadline: Sunday 4 October 2026, 23:59 Asia/Colombo.**

> This document consolidates the uploaded official Challenge Booklet, final combined engineering specification, submitted Nexora Designathon, and existing `CLAUDE.md`. It is a development checklist and agent context, **not** a replacement for the source documents or the exact API/schema contracts in `docs/`. In conflicts, follow the official booklet for competition rules, the submitted Designathon for screen fidelity, and the agreed Go-specific repository implementation for technology; escalate unresolved contradictions. Do not invent functionality.

## 0. How to load context (read this first)

`CLAUDE.md` is the companion specification and is always in context via `@CLAUDE.md`. Read it fully before editing — this file and `CLAUDE.md` together are the authority.

Other repository contracts are **large**: load them **on demand only**, never preemptively. When a task touches their subject, use the Read tool then.

- `@CLAUDE.md` — companion spec: mission, screen-ID map, build order, rules of engagement. **Always loaded.**
- `docs/api.md` — every endpoint with role, purpose and payload. Read before touching an API route.
- `docs/data-model.md` — tables, columns, relations and database constraints. Read before touching a migration or query.
- `docs/prioritisation-policy.md` — the fairness ordering behind deferrals. Read before touching the planner.
- `docs/deployment.md` — Compose fallback, local vs S3 media, AWS (EC2/S3/Nginx/Neon). Read before touching deployment, storage or infra.
- `libs/shared-types/src/lib/` — enums, DTOs, constraint catalogue (the TS half of the mirrored contract).

Sections below marked _[see `CLAUDE.md`]_ are summarized here for checklist completeness; treat `CLAUDE.md` as the fuller wording where the two overlap. This file carries the authoritative checklists that `CLAUDE.md` does not: the offline/sync contract, non-functional gates, the full test matrix, the delivery package and the Datathon separation.

### 0.1 How to work in this repo (mechanics)

**Package manager — absolute rule: use `npm`.** Never `bun`, `yarn` or `pnpm`. This matches the `engines` field and every script in `package.json`. All commands run from the repository root.

| Package manager       | Command                |
| --------------------- | ---------------------- |
| Install the workspace | `npm ci`               |
| Add a dependency      | `npm install <pkg>`    |
| Add a dev dependency  | `npm install -D <pkg>` |
| Run a script          | `npm run <script>`     |

**Stack at a glance.** Go 1.24 REST backend · Next.js 16 (App Router) / React 19 / TypeScript 6 (strict, `noUncheckedIndexedAccess`) · Tailwind v4 (CSS-based config) · PostgreSQL 17 · RabbitMQ 4 · Nx 23 / npm monorepo · Node 22. Do not introduce a stack technology without escalating first.

**Commands you will actually use** (full list and rationale in `@CLAUDE.md` §3):

- `npx nx dev web` — web dev server → http://localhost:3000
- `npx nx serve api` — Go API → http://localhost:8080
- `docker compose up postgres rabbitmq` — just the database and queue
- `npm run verify` — lint + typecheck + test + build, both languages (must pass before a commit)
- `npx nx test <project>` — one project: `web`, `api`, `shared-types`, `api-client`, `ui`
- `docker compose up --build` — the full judge stack

**Repository layout.** Nx monorepo: `apps/web` (Next.js, four role workspaces), `apps/api` (Go), `libs/` (`shared-types`, `api-client`, `ui`), `docs/`, `infra/`, root `docker-compose.yml`. Authoritative tree and the current state of the code: `@CLAUDE.md` §3 — read it before assuming a module exists.

**Configuration files — what controls what:**

- `eslint.config.mjs` — ESLint (Nx flat config); **not** Biome.
- `.prettierrc` — Prettier; `singleQuote: true`. For Go, `nx fmt api` runs `gofmt`.
- `tsconfig.base.json` — strict TS; path aliases `@waypoint/shared-types`, `@waypoint/api-client`, `@waypoint/ui`.
- `nx.json` / `apps/*/project.json` — Nx targets; Go is wrapped with `nx:run-commands` (no third-party Go plugin).
- `.env.example` — environment template. Local dev: `cp .env.example .env`. Never commit `.env`. Authentication is Go-owned opaque sessions (`SESSION_TTL`); `DEMO_SEED_PASSWORD` sets the four seeded demo passwords. There is no JWT/signing secret. `JWT_SECRET` is no longer used.

**Conventions — the fuller wording lives in `CLAUDE.md`:** Go backend `@CLAUDE.md` §10 · Frontend `@CLAUDE.md` §11 · Testing `@CLAUDE.md` §12 · Git workflow `@CLAUDE.md` §13 · API conventions `@CLAUDE.md` §9. Design tokens and touch-target rules are in §11 and must be followed for screen fidelity.

**MCP servers available.** Neon (`neon`) for the linked Postgres branches; Figma (`figma`) for reading the Day 5 design — the Figma MCP reads frames/components directly when a task touches screen fidelity.

**Agent skills.** Procedural skill packs are installed project-level in `.agents/skills/` and tracked in `skills-lock.json`. The full list — installed, skipped and why — is in [`docs/skills.md`](docs/skills.md). Skills adapt to this document's architecture; they never change it.

## 1. Source-of-truth and boundaries

1. Official **Challenge Booklet**: binding competition requirements, constraints, deadlines, submission and Datathon restrictions.
2. Submitted **Nexora_Designathon**: the Day 5 experience that the Hackathon must implement, including screens, visual style and three degradation scenarios. Figma: https://www.figma.com/design/y56IO0Jr7dT381Rrq1xfj2/UI-Tech-Triathlon?node-id=0-1
3. **Waypoint FINAL System Specification**: consolidated requirements, suggested schema and API contracts; its **Spring Boot implementation is superseded by the agreed Go backend**, not by changes to business rules.
4. **CLAUDE.md**, `docs/api.md`, `docs/data-model.md`, `docs/prioritisation-policy.md`, `libs/shared-types`: repository-specific decisions and detailed contracts. Keep them synchronized with this guide. If they do not exist yet, create them from the agreed source specification, not guesses.
5. Code and tests: implementation state, never authority to silently weaken a requirement.

**Official vs chosen:** the competition allows automatic, assisted or validated manual allocation; **we chose assisted planning**. RabbitMQ, product-level `item`/`order_item`, Go-owned opaque sessions, the exact REST routes, polling, a demo clock and our fairness order are **team implementation decisions**, not official rules. Do not present them as competition mandates.

**System mission:** one responsive application covering **Store Manager → Dispatcher → Loader → Driver → Store Manager**, from ordering to validated planning, loading, delivery, receipt, notifications, forecasts and audit. Planning must explain which orders cannot be served and why.

## 2. Business context

- 120 outlets: **Fresh 80** (daily groceries, chilled/frozen; delivery before 08:00 opening), **Style 25** (weekly garments; volume-limited, mall access), **Tech 15** (as-needed heavy/fragile electronics).
- 60 vehicles: **12 refrigerated trucks + 40 dry-box trucks + 8 vans (4 refrigerated)**; 16 refrigerated vehicles in total. Two home depots: **Peliyagoda and Kandy**. Dispatchers plan both; a planning run is scoped to a depot.
- 12 districts. Outlet and vehicle home depot are fixed reference facts. A vehicle has an assigned driver; **driver availability is not an additional fleet-allocation constraint**.
- Operate Monday–Saturday, but use `calendar_day.is_operating`, not weekday assumptions. Calendar also marks holidays, paydays, festivals and monsoon conditions.
- **16:00** next-operating-day order cutoff. Orders arriving after cutoff are accepted for the **following operating run**, visibly warned, never silently lost/rejected. A Fresh outlet may have **separate chilled and ambient orders for the same date**. Do not merge them.
- All dates/times and demo operations: **Asia/Colombo**, 24-hour clock.

## 3. Unbreakable invariants

1. **One authoritative Go constraint validator** (`apps/api/internal/constraint`), reused by suggestion, candidate validation, route edits, breakdown recovery and final confirmation. UI validation is only advisory.
2. **Suggestions never become authoritative automatically**: planning jobs write `planning_result`; only final confirmation, revalidating current DB state **inside one transaction**, writes final `route`, `route_leg`, `allocation` and deferral decisions.
3. Every confirmed order in the closed queue is **either allocated or explicitly deferred**. Each deferral has type, text, binding constraint when applicable, actor, decision time and target date. Missing reasons block publishing.
4. **Hard rules cannot be overridden.** A dispatcher may override **soft prioritisation only** with a typed `POLICY` reason and audit record.
5. Remaining capacity/time/fuel are derived from actual current orders/routes, not trusted from clients or persisted as authoritative cached numbers.
6. Use the official district-travel/service-allowance trip formula. **No maps/routing API** may replace it. Never leak competition data to external services.
7. Go domain enums/constraints and `libs/shared-types` must agree. JSON camelCase; enum wire values UPPER_SNAKE. Change Go + TS + typed API client + `docs/api.md` together.
8. Inject a `Clock`; no direct `time.Now()` in business logic. Browser time must not determine the seeded demo day.
9. Offline driver events get a device-created `clientEventId` before network access. Persist locally first, upload in capture order, deduplicate on the server and preserve capture timestamps. Conflicts must remain visible, never silently discarded.
10. Preserve original reference IDs and data. A human teammate places supplied datasets in the designated repository paths; agents do not regenerate, commit restricted Datathon data, print private rows into prompts or upload them to outside services.
11. Do not add extra roles, settings, admin tools, dark mode, internationalisation, map APIs or unapproved scope. Follow the submitted design.
12. Keep `npm run verify` passing before commits.

## 4. Complete hard-constraint catalogue

Implement the **exact codes and group labels** in `libs/shared-types/src/lib/constraints.ts` and mirror in `apps/api/internal/domain/constraints.go`. Every validator call returns **all applicable passed and failed checks**, each with a machine code, E-ID and human-readable measured detail. **Equality at a limit passes; exceeding it fails.**

| Group | Code                       | Exact rule                                                                       |
| ----- | -------------------------- | -------------------------------------------------------------------------------- |
| E-01  | `WEIGHT_CAPACITY_EXCEEDED` | Sum of orders on each trip ≤ vehicle `weight_cap_kg`.                            |
| E-01  | `VOLUME_CAPACITY_EXCEEDED` | Sum of orders on each trip ≤ vehicle `volume_cap_m3`.                            |
| E-01  | `ORDER_SPLIT_FORBIDDEN`    | Whole order travels on exactly one vehicle and trip.                             |
| E-01  | `DUPLICATE_ASSIGNMENT`     | An order cannot be allocated to multiple routes/trips.                           |
| E-02  | `REEFER_REQUIRED`          | Chilled/frozen requires reefer; reefer can also carry ambient.                   |
| E-03  | `VAN_ONLY_ACCESS`          | `van_only` outlet must use a van.                                                |
| E-04  | `DEPOT_MISMATCH`           | Vehicle home depot must match the outlet's depot.                                |
| E-05  | `TRIP_LIMIT_EXCEEDED`      | At most **two total trips per vehicle per day**.                                 |
| E-05  | `TRIP_NUMBER_INVALID`      | Trip number is exactly **1 or 2**.                                               |
| E-05  | `FRESH_TIME_BUDGET`        | Vehicle's **sum of Fresh trip minutes ≤ 270/day**.                               |
| E-05  | `STYLE_TECH_TIME_BUDGET`   | Vehicle's **combined Style + Tech trip minutes ≤ 480/day**, shared pool.         |
| E-05  | `BRAND_DISTRICT_MIX`       | All orders on the same vehicle/trip have the **same brand AND district**.        |
| E-05  | `VEHICLE_UNAVAILABLE`      | No unavailable, `in_workshop` or broken-down vehicle on that date.               |
| E-05  | `NON_OPERATING_DAY`        | Dispatch date must be operating according to `calendar.csv`.                     |
| E-06  | `DELIVERY_WINDOW_MISSED`   | Planned arrival must be within outlet window; Fresh before opening as specified. |
| E-06  | `MALL_WINDOW_MISSED`       | Mall-dock deliveries must fit the mall's fixed access window.                    |
| E-07  | `FUEL_QUOTA_EXCEEDED`      | Existing plus projected vehicle fuel use for the ISO week ≤ weekly quota.        |

**Official trip-time formula:**

```text
outbound_min   = district_travel.depot_to_district_freeflow_min  # once per trip
inter_stop_min = district_travel.inter_stop_freeflow_min * max(order_count - 1, 0)
handling_min   = SUM(service_allowance_min(brand, outlet.dock_type))
trip_minutes   = outbound_min + inter_stop_min + handling_min
```

**Never include the return journey in official Task 2B trip minutes.** Pinned example: `37 + 9*(3-1) + 15+15+16 = 101 min`. One order means zero inter-stop journeys. Count each vehicle's trips against its brand-specific daily budgets; Style/Tech share one pool. Official Task 2B groups by `(vehicle_id, trip_id)` and keys individual orders by `order_ref`, **not** `outlet_id` (an outlet can have two orders).

**Windows:** if arrival is before `window_open`, wait and start service at opening; `arrival > window_close` is late (arrival exactly at close is not late). For a mall, respect the mall access interval as well. ETA is recalculated when stop order or live state changes. Show **arrival windows** to stores, not a false exact minute.

**Fuel:** `route_distance_km = depot_to_district_km + inter_stop_km * max(n-1,0)` for the agreed reference-based route approximation; `estimated_fuel_l = route_distance_km / km_per_l`; sum by vehicle/ISO week against its quota. Do not confuse the operational team's trip-2 departure/reload scheduling assumption with the official Task 2B duration formula.

## 5. Explicit team assumptions and prioritisation (soft, not official)

- Configurable: Fresh loading **03:30**, Fresh trip 1 departure **03:45**, Style/Tech trip 1 departure **09:00**. Trip 2 departure follows trip 1's last stop, estimated outbound-equivalent return time and **20 min reload**. These are operational scheduling assumptions; do not add the return to the **official Task 2B trip budget**.
- After hard feasibility, follow `docs/prioritisation-policy.md` in this exact order: **(1)** protect outlets deferred on the previous run, **(2)** protect outlets unserved ≥7 days, **(3)** chilled before ambient, **(4)** narrow windows first, **(5)** largest order that still fits among equals, **(6)** Style/Tech may move before Fresh. If the last preference conflicts with Fresh feasibility, hard Fresh constraints always win.
- Explain every suggested deferral with binding rule/code, E-ID, relevant shortage and human-readable consequence. Flag repeated skips (twice within 14 days). Never promise perfect optimisation or silently encode a different ranking.
- Treat `FROZEN` like chilled if present; the supplied S1 data contains chilled/ambient only. Do not add separate frozen UI.

## 6. Order and route lifecycle _[see `CLAUDE.md` §5]_

```text
PLACED → CONFIRMED → ALLOCATED → LOADED → IN_TRANSIT → DELIVERED → RECEIVED
          ↘ DEFERRED → eligible on a later run
IN_TRANSIT → FAILED → DEFERRED
```

`DELAYED` is a **delivery outcome**, not a separate order status. Enforce only the transitions defined in shared types. Store confirmation, dispatcher queue close, final allocation, loader readiness, dispatch, driver outcomes and GRN must be consistent across all four workspaces. Deferred orders must never appear in loader lists. Route = one vehicle trip; route leg = one stop; `seq` is zero-based internally; first leg starts at `DEPOT`, later legs at previous outlet. Route edits use `route_version` optimistic locking; never overwrite a stale edit.

## 7. Functional requirements — every submitted screen

**Screen fidelity is required.** Match the Designathon Figma and its rationale/copy, not merely the route names. Desktop target 1440px, loader tablet 1024px/phone 402px, driver phone 402px; judges test loader/driver at phone width.

### Shared authentication: G-01–G-03 — `/signin`

- Four role cards prefill the four demo accounts; proper login, loading state, visible accessible errors and role redirect. Go-owned opaque session and server-side scope enforcement; sign-out and expired-session handling. Four roles only.

### Dispatcher: D-01–D-09

- **D-01 `/dispatcher`:** actionable KPI dashboard: queue count, available vehicles, trips allocated, deferrals; each KPI links to action; decisions/alerts and 16:00 countdown.
- **D-02/D-02a/D-02b `/dispatcher/queue`:** unified confirmed queue across three brands, filter chilled Fresh/Style/Tech; order detail with lines, temp, weight, volume, windows, access. Explicit **Close queue**, per-brand confirmation, warning about unconfirmed orders; once closed, read-only with late orders shifted to next operating run; start planning.
- **D-03/D-03a `/dispatcher/plan`:** unallocated order pool; per-vehicle Trip 1/2 lanes; live weight/volume, Fresh or Style+Tech minutes, weekly fuel and windows; rule panel shows every E-ID and explanation. Auto-allocate starts a **proposal**, not a publish. Candidate moves/reordering use server validation and recalculate. Invalid move shows precise failing checks and a feasible alternative; accessible buttons may substitute for drag/drop if necessary.
- **DG-A1/DG-A2 overcapacity:** explain root binding resource and how engine chose, resource meters for reefers/Fresh time/van access/dry-box/fuel; show proposed deferrals with E-ID and explanation. Disable save until **every** deferral has reason; allow bulk reason by constraint; soft override requires justification. Notify affected stores checked by default.
- **D-04 deferral modal:** required reason picker and explanation, protected next run, optional bulk assignment; cannot defer without reason.
- **D-05 published state:** confirm publication once; create loading lists and store ETA/deferral notifications; block duplicate publishing.
- **D-06 `/dispatcher/tracker`:** active trip progress, completed/total stops, next stop, driver, ETA vs window, shortfalls, delay, breakdown and sync state; poll every **30s**. Offline driver status is neutral and not automatically 'late'.
- **DG-C1 breakdown drawer:** mark vehicle broken down; requeue **only unserved stops**; evaluate alternatives with capacity, depot, access, time, windows, trip count and fuel checks; explain rejected vehicles, allow explicit deferral fallback, notify driver/stores by default.
- **D-07/D-07a `/dispatcher/deferrals`:** searchable attributable history by outlet/date/constraint/decision maker/new date; highlight twice skipped in 14 days, clear empty state.
- **D-08 `/dispatcher/forecast`:** read-only weekly demand, chilled volume vs reefer capacity, payday/festival markers and recommendations; backed by `/forecast/demand` (seeded figures acceptable for Hackathon).
- **D-09 `/dispatcher/fleet`:** fleet, temperature/type, home depot, capacity and availability; reefer/dry-box/van filters.

### Loader: L-01–L-04, L-T1–L-T3

- **L-01 `/loader`:** own depot/bay trips ordered by departure, loading progress/status (Not started/Loading/Ready/Flagged), sensible pre-publication empty state.
- **L-02 `/loader/trips/[routeId]`:** loading list **reverse stop order** (last delivery loaded first), per-order checkoff, stop number, quantities, weight/volume, temp and dock type. **Mark trip loaded disabled** until each row is checked or flagged.
- **L-02a live change:** poll `routeVersion`, update changed rows **in place**, attribute changes and visually outline them; never reorder items under a loader's fingers while loading.
- **L-03/L-03a flag sheet:** record **order-line** item, missing/damaged reason, quantity and optional photo. Validate `loaded + damaged + missing = ordered` using database ordered quantity. Do not flag only the trip header.
- **L-04 flag sent:** confirm dispatcher, driver and store are notified.
- **L-T1–L-T3 tablet:** ≥1024px master-detail, trips left and list right, flag side panel keeping list visible. Deferred orders never shown.

### Driver: R-01–R-04

- **R-01 `/driver`:** phone cockpit with active run, current stop, arrival window, dock, planned/completed stops, loader shortfall/data-mismatch alert and POD entry.
- **R-02/R-02a `/driver/stops/[legId]/outcome`:** large **Delivered / Failed / Delayed** actions; Failed requires a reason chip; optional note/photo; POD requires receiver name and **signature or photo** for delivery. Every action works offline, local-save-first; primary targets ≥56px; use only while safely stopped.
- **DG-B1/DG-B2/DG-B3:** cached run sheet; neutral-grey 'No signal · N actions saved on this phone'; save outcome to IndexedDB **before** attempting upload; honest saved-locally message; next-stop action; chronological queued-event log with capture times; reconnect auto-flush and per-event reconciliation. Never use alarming red merely for lost connectivity.
- **R-04 `/driver/log`:** show pending, synced, conflicts and all-synced confirmation, how many events matched the plan and what the store now sees. Never claim synced while an event is pending/rejected.

### Store Manager: S-01–S-07

- **S-01 `/store`:** tomorrow's orders, today's delivery, cutoff countdown, deferrals; place-order/status/receipts shortcuts.
- **S-02/S-02a/S-02b `/store/order`:** SKU-based line items, chilled and ambient **separate orders**, running units/weight/volume and next-operating-day default. Inline plus summary errors; after-cutoff acceptance with clear next-run warning.
- **S-03 submitted:** immediate generated order number and next steps; submission state prevents duplicate creation.
- **S-04 `/store/orders/[orderId]`:** status, **arrival window**, stops away, driver and advance loader-shortfall warning.
- **S-05 deferred state:** new date, concise reason, who decided/when, **protected next run**, primary acknowledge action.
- **S-06/S-06b `/store/orders/[orderId]/receipt`:** GRN with expected vs received **per order line** and condition, prefilled shortfall, driver's POD, issues and confirmation/success state.
- **S-07 `/store/orders`:** ongoing live orders with detail overlay; completed orders for last seven days, outcomes and Reorder shortcut.

## 8. Backend architecture (Go replaces Spring Boot, not behaviour)

```text
Next.js PWA (four RBAC workspaces)
  ├─ typed /api/v1 REST client, opaque bearer session
  └─ Driver: Service Worker + Dexie/IndexedDB run-sheet cache and outbox
                  │ HTTPS
                  ▼
Go net/http API → domain services → pgx/PostgreSQL 17 (authoritative state + audit)
                  └─ planning job publisher → RabbitMQ 4 → Go planning worker
                                              └─ planning_result (proposals only)
```

### Authentication boundary (Go-owned sessions)

```
Next.js ── POST /api/v1/auth/login (email + password)
        └─> Go API (issue opaque session · verify bearer token · RBAC · depot/outlet scope)
                └─> PostgreSQL (app_user credentials+scope, session hashes)
```

**The Go API owns authentication, session and identity.** Go verifies the submitted password against `app_user.password_hash` (Argon2id), mints a random opaque session token, stores only its SHA-256 hash in `session`, and verifies the `Authorization: Bearer` token on every request (live, unexpired session for an active `app_user`). `POST /api/v1/auth/logout` deletes the caller's session. There is no second identity system and no cross-system mapping, and client-supplied identity headers are never trusted.

**Existing repo:** Nx 23/npm; Node 22; Next.js 16 App Router, React 19, Tailwind v4, strict TypeScript. Preserve existing workspace, CI, shared contracts, API client, design tokens, route shells, Go config/router/health/trip-time; verify actual files before assuming a module is finished.

**Go conventions:** `net/http` method-aware `ServeMux`, `r.PathValue`; `log/slog`; `pgx/v5` handwritten SQL **without ORM**; `goose/v3` numbered forward SQL migrations embedded and applied at startup; `rabbitmq/amqp091-go`. Authentication is owned by the Go API with opaque, database-backed sessions and Argon2id password hashes (`golang.org/x/crypto/argon2`); Go verifies the bearer token and enforces RBAC, scope and domain rules. Nx `project.json` wraps Go via `nx:run-commands` (no third-party Go Nx plugin). Handlers parse/auth/call services/write JSON; no SQL/arithmetic in handlers. Service methods take `context.Context` and injected clock; log errors once at HTTP boundary. Database structs stay private to persistence packages. Transactional writes for allocation, edits, state changes and audits. Avoid dependency cycles.

**Target packages** under `apps/api/internal/`:

| Package                           | Responsibilities                                                                                                            |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `config`, `clock`, `httpx`        | Env validation; real/demo clock; router, strict JSON, auth/error middleware.                                                |
| `store`, `seed`                   | Pool/transactions, embedded migrations, reference CSV and demo seed.                                                        |
| `domain`, `auth`                  | Mirrored enums/DTO vocabulary; verify the authenticated request (opaque Go session), RBAC and per-depot/outlet/route scope. |
| `catalog`, `orders`               | Items/outlets/vehicles/calendar/travel/service reference; orders, cutoff, confirmation, queue.                              |
| `constraint`, `planning`          | One pure validator; time/window/fuel calculation, fairness, engine, async job runner/results.                               |
| `routes`, `deferrals`             | Route/legs/reorder/dispatch/ETA/live state/breakdown; reasons/history/protected-next-run.                                   |
| `loading`, `delivery`, `receipts` | Load checklist/shortfalls; POD, offline event reconciliation; GRN/issues.                                                   |
| `notify`, `forecast`, `audit`     | In-app notifications; read-only demand series; immutable actor/time audit.                                                  |

**Planning job:** `POST /allocations/suggest` returns **202 `{jobId}`**. Worker state QUEUED → RUNNING → COMPLETED/FAILED, polling via `/planning-jobs/{id}`; RabbitMQ exchange `waypoint.planning`, queue `waypoint.planning.queue`, routing key `planning.generate`, retry transient errors then DLQ, idempotent processing. An in-process worker implementing the same interface is an explicitly permitted interim fallback; preserve the job API. Do not block the HTTP request for planning. Tracker uses **30s polling**, loader polls route versions; **no WebSocket scope**.

**Frontend:** App Router Server Components by default; use client components only for interactive/browser work. Next 16 async `params`/`searchParams` and `proxy.ts` (not old middleware convention). Tailwind v4 `@theme` in `global.css`, shared `@waypoint/ui`, `@waypoint/shared-types`, `@waypoint/api-client`. All calls through `apps/web/src/lib/api.ts` (no ad hoc raw fetch). Strict TS/no `any`; check errors with `WaypointApiError` (offline vs 422). Build role-specific UI in role folders and shared components in `libs/ui`.

## 9. PostgreSQL entities and required integrity

Use `docs/data-model.md` for exact types/relations/migrations. Implement **all** entities from the final specification:

| Table                                   | Minimum purpose/key data                                                                  |
| --------------------------------------- | ----------------------------------------------------------------------------------------- |
| `depot`                                 | ID/code/name/location/active.                                                             |
| `outlet`                                | ID, brand, district, depot, dock/parking constraints, outlet/mall windows.                |
| `app_user`                              | Identity + credential (Argon2id password_hash), role, depot/outlet scope, active flag.    |
| `session`                               | Opaque session tokens (SHA-256 hash only), expiry, FK to `app_user`.                      |
| `vehicle`                               | ID, type/temp, weight/volume, fuel type/efficiency/quota, home depot/status.              |
| `item`                                  | SKU, name/brand/category, unit weight/volume, temperature requirement.                    |
| `customer_order`                        | Number, outlet, brand, dates, totals, temp, status, cutoff, notes.                        |
| `order_item`                            | Order/SKU, qty and **snapshotted** unit weight/volume, line totals.                       |
| `calendar_day`                          | Date, ISO week, operating, holiday/payday/festival/monsoon.                               |
| `district_travel`, `service_allowance`  | Official time/distance and per-brand/dock handling references.                            |
| `traffic_speed`, `road_condition`       | Optional advanced/Datathon-compatible reference, **not** replacement for Task 2B formula. |
| `vehicle_daily_availability`            | Date-specific available/workshop/breakdown.                                               |
| `planning_job`, `planning_result`       | Async job status and **non-authoritative** proposal/constraint explanations.              |
| `route`, `route_leg`, `allocation`      | Confirmed trip, zero-based ordered stops, decision, route version.                        |
| `deferral_log`                          | Constraint/reason, actor/time, deferred date and historical protection.                   |
| `load_item`                             | Per order-line ordered/loaded/damaged/missing quantities.                                 |
| `delivery_event`, `order_item_delivery` | Idempotent offline driver events, POD and line-level delivered/short/damaged.             |
| `receipt`                               | GRN, line receipt/issue details, receiver and proof; implement line detail as needed.     |
| `vehicle_fuel_usage`                    | Per-vehicle ISO-week historical fuel facts.                                               |
| `notification`, `audit_log`             | Targeted alerts and append-only before/after actor/time trace.                            |

Enforce FK, CHECK, unique and transactional integrity. At minimum: `UNIQUE(vehicle_id,route_date,trip_no)`, `CHECK(trip_no IN (1,2))`, `UNIQUE(route_id,seq)`, `UNIQUE(client_event_id)` and no duplicate final order assignment. UUIDs for internal operational IDs; preserve supplied natural outlet (`OUT014`), vehicle (`VEH014`) and S1 `order_ref` IDs. Compute totals from server-side SKU snapshots; **never trust client-submitted totals**. No authoritative derived remaining-capacity/time/fuel columns.

## 10. Complete REST endpoint checklist

**Base `/api/v1`**; Go opaque bearer session except login; authorization and resource scope enforced server-side. JSON camelCase; reject unknown fields. Error `{message, code?, constraintResults?}`; 400 malformed, 401 unauthenticated, 403 wrong role, 404 missing/out-of-scope, 409 stale route version, 422 attempted infeasible confirmation. **Validation probe** returns `200 {valid:false,...}` rather than 422. The final specification lists the core endpoints below; `CLAUDE.md` adds demo-clock operations for reproducible judging.

| Method | Endpoint                         | Responsibility                                                                |
| ------ | -------------------------------- | ----------------------------------------------------------------------------- |
| POST   | `/auth/login`                    | Verify credentials; issue opaque session, role/scope.                         |
| POST   | `/auth/logout`                   | Revoke the caller's session.                                                  |
| GET    | `/me`                            | Current profile/scope.                                                        |
| GET    | `/outlets`                       | Outlet search/access/windows.                                                 |
| GET    | `/vehicles`                      | Fleet filters, availability, capabilities/fuel.                               |
| POST   | `/orders`                        | Create item-based order; generated number, server totals.                     |
| GET    | `/orders/{id}`                   | Scoped order/items/allocation/status.                                         |
| POST   | `/orders/{id}/confirm`           | Confirm order and apply cutoff policy.                                        |
| POST   | `/orders/close`                  | Dispatcher closes planning queue.                                             |
| GET    | `/orders/queue?date=`            | Closed queue, filter/group by depot/brand/district.                           |
| POST   | `/allocations/suggest`           | Create async planning proposal job (202).                                     |
| GET    | `/planning-jobs/{jobId}`         | Job status/progress/errors.                                                   |
| GET    | `/planning-jobs/{jobId}/results` | Proposed routes, metrics, deferrals/explanations.                             |
| POST   | `/allocations/validate`          | Full candidate constraint verdicts (200 even if invalid).                     |
| POST   | `/allocations/recalculate`       | Draft route metrics, ETA, windows, fuel and violations.                       |
| POST   | `/allocations/confirm`           | **Transactional revalidation** and final allocation/deferrals.                |
| GET    | `/allocations/{date}`            | Confirmed board.                                                              |
| POST   | `/routes`                        | Draft route/candidate validation.                                             |
| GET    | `/routes/{id}`                   | Scoped route and metrics.                                                     |
| PATCH  | `/routes/{id}/legs/reorder`      | Optimistic version check, recalculate.                                        |
| GET    | `/routes/{id}/legs`              | Sequenced stops/items/loading/ETA.                                            |
| POST   | `/routes/{id}/dispatch`          | Check readiness and dispatch.                                                 |
| POST   | `/routes/{id}/shortfalls`        | Order-line load discrepancies and notifications.                              |
| POST   | `/legs/{id}/events`              | Driver outcome, POD, client event ID.                                         |
| POST   | `/sync/events`                   | Batch offline events with **per-event** accepted/duplicate/rejected/conflict. |
| GET    | `/sync/status`                   | Driver sync status.                                                           |
| POST   | `/orders/{id}/receipt`           | GRN and line issues.                                                          |
| GET    | `/orders/{id}/eta`               | Store arrival window/route progress.                                          |
| GET    | `/deferrals`                     | Scoped deferral history.                                                      |
| POST   | `/deferrals/{orderId}`           | Manual/soft override deferral with reason.                                    |
| GET    | `/routes/live`                   | Dispatcher tracker.                                                           |
| GET    | `/forecast/demand`               | Depot/brand/week total + chilled forecast.                                    |
| GET    | `/meta`                          | Authoritative server/demo clock and demo mode (**repo-specific**).            |
| POST   | `/demo/clock`                    | Demo-stage clock jump, demo-mode dispatcher only (**repo-specific**).         |
| POST   | `/demo/reset`                    | Reset demo seed, restricted to demo mode (**repo-specific**).                 |

**API coverage gap to resolve explicitly:** D-06 breakdown action, loader's **Mark trip loaded** action, store's deferral **acknowledge**, notifications list/read, and store's ongoing/completed order list are required UI interactions but are **not explicitly enumerated as standalone endpoints** in the final specification's section 17. Before implementation, inspect `docs/api.md` and the current code for their actual contracts; if absent, design the minimum necessary Go endpoint + TS client + docs together and mark it as an implementation completion, **not** a new competition feature. Do not pretend these endpoints are already specified or implement no-op buttons.

## 11. Offline, synchronization and recovery contract

- Driver PWA caches the current route and stops when online. Dexie IndexedDB `routes` and `outbox` (event ID, leg ID, payload, occurredAt, status). Service worker supports offline app-shell where feasible; outcome capture must work offline even without background-sync browser support.
- Write every event to local outbox **before** UI success or network attempt; use `crypto.randomUUID()` and preserve device `occurredAt`. Flush in original capture order on foreground/reconnect, with background sync only as enhancement.
- Backend `client_event_id` unique; duplicate replays are safe. Reconcile **each event independently**: ACCEPTED, DUPLICATE, REJECTED or CONFLICT. Stale route version or changed leg must not overwrite authoritative data. Persist/surface unresolved conflicts to dispatcher. Do not discard pending data when a network request fails.
- Offline is a neutral-grey operating state; clearly distinguish locally saved, uploading, fully synced, failed and conflict states. Loader route-version changes should not silently erase checkmarks/flags; reconcile them with current authoritative route.
- Breakdown recovery touches **unserved** legs only; preserve delivered history, POD, existing receipts and audit.

## 12. Seed, demo clock and confidential datasets

**Reference CSVs and exact expected columns** (verify original headers on import):

- `outlets.csv`: `outlet_id`, `brand`, `district`, `depot`, `dock_type`, `parking_constraint`, `mall_window`, `window_open_time`, `window_close_time`.
- `vehicles.csv`: `vehicle_id`, `type`, `temp`, `weight_cap_kg`, `volume_cap_m3`, `fuel_type`, `km_per_l`, `weekly_fuel_quota_l`, `depot`.
- `calendar.csv`: `date`, `dow`, `dow_name`, `is_weekend`, `iso_year`, `iso_week`, `is_payday`, `festival`, `festival_ramp`, `is_holiday`, `monsoon`, `is_operating`.
- `district_travel.csv`: `district`, `depot`, `road_class`, `free_flow_kmh`, `depot_to_district_km`, `depot_to_district_freeflow_min`, `inter_stop_km`, `inter_stop_freeflow_min`.
- `service_allowance.csv`: `brand`, `dock_type`, `service_allowance_min`.
- `task2b_peak_day_scenarios.csv`: `scenario`, `order_ref`, `outlet_id`, `brand`, `district`, `depot`, `dock_type`, `parking_constraint`, `mall_window`, `window_open_time`, `window_close_time`, `temp_requirement`, `order_units`, `order_weight_kg`, `order_volume_m3`, `deferred_yesterday`, `days_since_last_served`.
- `task2b_peak_day_fleet.csv`: `scenario`, `vehicle_id`, `status`.

**Seed convention:** `apps/api/internal/seed/data/` embedded via `go:embed`; human teammate supplies the authorized reference files, not an agent. Startup runs migrations and **idempotent** seed. Dataset IDs/totals win over decorative prototype counts. Generate display-only outlet names/vehicle registrations if needed; source CSVs do not contain them. Do not modify official S1 aggregate totals; generate seed order lines that exactly reconcile to them.

**Demo scenario:** S1, planning Fri **25 Sep 2026**, delivery Sat **26 Sep 2026**, Peliyagoda. Use seeded `deferred_yesterday` and `days_since_last_served` for fairness. Prototype mentions 186 orders/54 available/6 workshop/22 deferred; **treat these as visual examples, not guaranteed dataset facts**. Use actual imported data and update the walkthrough accordingly.

**Four seeded accounts** (demo-only password `waypoint2026`):

| Role       | Login                     | Scope                  |
| ---------- | ------------------------- | ---------------------- |
| Dispatcher | `priyantha.w@waypoint.lk` | Both depots.           |
| Loader     | `nadeesha.p@waypoint.lk`  | Peliyagoda Bay 2.      |
| Driver     | `kasun.p@waypoint.lk`     | VEH014/assigned route. |
| Store      | `ishara.s@waypoint.lk`    | OUT014.                |

Never expose seeded credentials in a production-like deployment with real data. Use `DEMO_MODE` explicitly. Demo clock defaults to `2026-09-25T15:40:00+05:30` and progresses from injected time; dispatcher-only `/demo/clock` stages: BEFORE_CUTOFF (Fri 15:40), AFTER_CUTOFF (Fri 16:05), LOADING (Sat 03:30), ON_ROUTE (Sat 05:00). `/meta` supplies now/demoMode; `/demo/reset` restores the seed. Ensure all UI countdowns/dates use the API clock.

## 13. Non-functional requirements and quality gates

- **Responsiveness:** all four roles usable on their intended devices; loader/driver phone-sized tests are mandatory. Tablet master-detail for loader; no hover-only primary actions. Fast, clear feedback and disabled duplicate-submit actions.
- **Usability/accessibility:** 48px phone/tablet touch targets; driver primary 56px; ≥14px mobile text; labelled controls, keyboard/focus support, in-place plus summary validation, status **never by colour alone**, empty/loading/error/success states, plain-language reasons and next steps. Driver UI explicitly intended only while safely stopped.
- **Visual fidelity:** match submitted Figma screens, component hierarchy and style tokens. Use `bg-page`, `bg-card`, `text-ink`, `text-ink-muted`, `text-brand`, `text-link`, `bg-action`, `text-success`, `text-warning`, `text-error`, `bg-offline`; no hard-coded colours. Monospace/tabular IDs and times, 24h `HH:MM`, `kg`, `m³`, desktop 252px sidebar/60px top bar, phone app bar and 4-tab navigation, shared badges. Amber for degradation; neutral grey for offline.
- **Security:** Go-owned opaque session (authentication owned by the Go API); Go-enforced role and record-level depot/outlet/route scoping on **every** handler; strict JSON decoding, server-computed totals, safe file/photo handling, no secrets in source or logs. Return 404 for out-of-scope records where specified. Demo controls restricted to demo mode.
- **Reliability:** DB transactions and optimistic route versions; queue retries/DLQ; idempotent seed, planning jobs and offline events; conflict visibility and no data loss; safe restart/reconnect behaviour.
- **Performance/feedback:** planning is asynchronous with progress polling; live tracker 30s polling; loader updates on version changes; no blocking UI while jobs run. **No numeric latency/uptime SLA is specified in the supplied sources; do not invent one.**
- **Audit/explainability:** append-only actor/timestamp/before/after for deferrals, allocations, route edits, load shortfalls, delivery events and relevant recovery actions. Store explanations with constraint codes; no silent overrides.
- **Maintainability:** layered Go services; deterministic pure validator/planner; mirrored shared contracts; typed API client; forward-only migrations; documented assumptions and testable clock; no duplicate business logic.
- **Deployment:** fresh clone `cp .env.example .env && docker compose up --build` starts web, Go API, PostgreSQL, RabbitMQ/worker as implemented, migrations and seed. Root `docker compose up` must work. Public deployed URL must stay live through review.

## 13a. Deployment & infrastructure

Two run targets. Full procedure: [`docs/deployment.md`](docs/deployment.md).

|          | Docker Compose (fallback)                                | AWS (hosted)                                      |
| -------- | -------------------------------------------------------- | ------------------------------------------------- |
| Role     | **Guaranteed reproducible fallback** — a judge runs this | Hosted deployment target                          |
| Command  | `cp .env.example .env && docker compose up --build`      | Provisioned per `docs/deployment.md`              |
| Database | compose PostgreSQL 17                                    | Neon PostgreSQL                                   |
| Queue    | RabbitMQ 4                                               | RabbitMQ 4 (no SQS/SNS)                           |
| Media    | `LocalMediaStorage` (`MEDIA_ROOT`, named volume)         | `S3MediaStorage` (private bucket, presigned URLs) |
| Edge     | none                                                     | Nginx on EC2 + Elastic IP                         |

**MEDIA STORAGE — team decision, not a booklet requirement.**

- One `media.Storage` interface, two implementations: `LocalMediaStorage` and `S3MediaStorage`, chosen by `MEDIA_STORAGE` (`local` default, `s3` hosted). The domain never calls an AWS SDK directly.
- Object keys are always **server-generated** (`<purpose>/<ownerId>/<uuid>`); a client-supplied key is never trusted. The API authorises the caller and the record before minting a key or reading bytes.
- S3 is a **private** bucket reached only via short-lived presigned URLs. Credentials come from the AWS SDK chain (EC2 instance role in production); never from the repository or `.env`.
- POD photos associate to `delivery_event.pod_photo`; shortfall photos to `load_item.photo_ref`.

**Do not** make AWS a hard dependency: Compose must run with `MEDIA_STORAGE=local` and the compose database, needing no AWS credentials. Do not introduce SQS/SNS, and do not add Nginx to the base Compose stack. Do not create a production Compose file unless a concrete production-only difference exists; today the differences are environment variables, so none is needed.

> **Official vs team decision:** the Challenge Booklet defines the business requirements only. AWS EC2, S3, Nginx and the local-media fallback are **team implementation/deployment decisions**. They never alter a required workflow or constraint.

## 14. Mandatory test matrix

**Go unit/table-driven:** each of the **17 hard constraints** passes, fails and accepts its exact boundary where applicable. Specific tests: chilled on ambient rejected / reefer accepted; van-only on truck rejected; wrong depot rejected; weight/volume exactly at limit accepted and over rejected; Fresh 270 accepted/271 rejected; shared Style+Tech 480 accepted/481 rejected; third trip rejected; mixed brand/district rejected; split/duplicate order rejected; unavailable workshop rejected; non-operating day rejected; delivery/mall window boundary and early waiting; ISO-week fuel boundary; single-stop zero inter-stop; pinned 101-minute example.

**Service/integration:** post-16:00 rolls to next `calendar_day.is_operating`; S1 aggregate order lines reconcile exactly; confirmation rejects unserved order without reason; confirm rejects stale proposal if fleet/order changed; concurrent confirmation does not double-allocate; stale `routeVersion` → 409; load quantity invariant; order-line shortfall reaches all three roles; offline duplicate event persists once; each batch event gets own outcome; sync conflict remains actionable; breakdown only requeues unserved stops; GRN matches item quantities/POD; store/depot RBAC prevents cross-scope access; audit captures actors and decisions.

**Frontend/E2E:** all G/D/L/R/S and DG states; loader reverse stop order and plan-change in-place; Mark loaded disabled until ready; failed delivery requires reason; POD requirement; offline first-save and reconnection; after-cutoff warning; deferral reasons mandatory; store arrival **window** and GRN; small-screen driver/loader usability; no dead ends in the README judge walkthrough.

Use Jest for TS and `go test` for Go; add regression tests for every bug. Run `npm run verify` (lint, typecheck, test, build), Go formatting and Docker image builds before merging. Do not weaken failing tests to pass CI.

## 15. Build plan, commands and delivery checklist

**Backend sequence:** (1) clock/config/Postgres migrations, (2) idempotent reference+S1+account seed, (3) auth/RBAC, (4) catalog/order/cutoff/queue, (5) all hard constraints + official formula tests, (6) planning engine/prioritisation/jobs, (7) validate/recalculate/transactional confirm, (8) deferrals/notifications, (9) routes/dispatch/loading, (10) delivery/offline sync/receipts, (11) live tracker/breakdown, (12) forecast/demo controls.

**Frontend sequence:** (1) sign-in/role redirect, (2) store order/status, (3) dispatcher queue/close/plan/DG-A/publish, (4) loader reverse list/flags, (5) driver cockpit/POD/offline/log, (6) store deferral/GRN, (7) dispatcher tracker/deferrals/fleet/forecast/DG-C, (8) all tablet/phone states and E2E polish. Frontend can use **typed fixture data** until Go endpoints are ready, but remove fixtures from the final workflow.

**Commands** (repo root):

```bash
npm ci
npx nx dev web                 # http://localhost:3000
npx nx serve api               # http://localhost:8080
docker compose up postgres rabbitmq
npx nx test api
npx nx test web
npx nx fmt api
npx nx tidy api
npm run verify
docker compose up --build
```

**Required repository deliverables:** GitHub monorepo named `TeamName_SolutionName`; root `README.md` with setup/configuration, seeded accounts, numbered four-role judge walkthrough and any significant departures from Day 5 design; root `docker-compose.yml` and `.env.example`; `docs/architecture.md` with diagram; `docs/data-model.md`; `docs/api.md`; `docs/prioritisation-policy.md`; `docs/ai-disclosure.md` describing assisted and unassisted work; public deployed URL; **unlisted 5–8 minute Hackathon YouTube demo** covering all four roles plus code/architecture; submit repository/deployed URL/credentials/video before deadline. Ensure the service stays deployed during review. Do not confuse the Designathon's 3–5 minute video with the Hackathon's 5–8 minute video.

**Git:** branch from `development` using `feat/<area>-<thing>` or `fix/<area>-<thing>`; small PRs; never commit `.env`, secrets, build artifacts or restricted competition datasets. Update AI disclosure accurately and document material Designathon departures in README. A change to an API route requires the Go handler, TS client, docs and tests in the same PR.

**Must ship even under time pressure:** four-role auth; real orders/cutoff/queue; all hard constraints; assisted proposal and transactionally confirmed plan; DG-A explained deferrals; loader reverse list/line-level flags; driver outcomes/POD/local-first offline queue/idempotent sync; store status/deferral/GRN; dispatcher tracker/deferral log; Docker+seed+walkthrough+deployment. If necessary reduce polish in this order: static read-only forecast → fleet filters → tablet polish → DG-C drawer → drag-and-drop (use accessible buttons) → RabbitMQ transport (preserve job API/in-process runner) → service-worker shell caching. **Never cut validator, mandatory deferral reasons or offline sync idempotency.** Record any omitted submitted feature honestly.

## 16. Datathon separation and future compatibility

The Datathon is **separately judged and not required to be integrated into the Hackathon**. The operational `GET /forecast/demand` can serve seeded, read-only weekly depot × brand series (total and chilled m³), with calendar festival/payday context. Keep future prediction inputs modular rather than inventing a trained model for the Hackathon.

If a teammate later tackles the Datathon, the booklet describes **Task 1** service-time/lateness predictions, **Task 2A** weekly demand forecasts and **Task 2B** S1 peak-day allocation plus written prioritisation policy. Task 2B output must preserve `scenario`, `order_ref`, `outlet_id` and row order, mark every order `served`/`deferred`, give served rows a valid vehicle and trip 1/2, leave deferred vehicle/trip blank, and pass official `check_allocation.py`. Do not modify official templates/identifiers. **No prohibited pretrained models, proprietary API-based modelling/preprocessing, low-code/fully automated end-to-end modelling, or external dataset sharing.** An agent must not independently run an end-to-end Datathon modelling pipeline or expose supplied training rows. The competition requires a written policy; Task 2B does **not** require ML. See the official booklet for complete separate submission instructions and constraints.

## 17. Agent execution protocol and traceability

Before every feature:

1. Identify its **screen IDs**, workflow, hard/soft constraints, DB tables, exact endpoint/DTO and tests from this file, `CLAUDE.md`, Figma, final spec and `docs/`.
2. Inspect the actual repository; do not assume a planned package, endpoint, dataset or screen is implemented merely because it is described here.
3. Implement vertically: migrations/queries → pure domain/service → handler/RBAC → shared DTO/client → Next.js screen → unit/integration/UI tests → docs.
4. Recheck cross-role propagation: what Dispatcher changes must reach Loader/Driver/Store; what Loader/Driver records must reach Dispatcher/Store; preserve audit, version and idempotency.
5. Verify failure, exact boundary, stale-data, post-cutoff and offline paths—not only the happy path.
6. Run formatting, relevant tests and `npm run verify`; update `docs/ai-disclosure.md` and README departures if needed. Never claim success without test evidence.

**Completion criterion:** from a fresh clone, a judge starts the complete stack, signs into all four accounts and follows one numbered seeded scenario: store order → cutoff/queue close → feasible suggestion + overcapacity reasons → Dispatcher validation/confirmation/publication → Loader reverse loading and shortfall → Driver route/POD while offline and subsequent sync → Store status/deferral/receipt → Dispatcher live/audit/deferral review. Every action must have a real persistent backend effect; no dead-end stub or fake successful UI.

---

## 18. Clarifications versus `CLAUDE.md` — NOT automatic new product scope

This guide is intentionally broader than `CLAUDE.md` in **traceability and implementation checklists**. Items that are additional **documentation emphasis**, not newly invented competition features:

- Explicitly distinguishes official competition constraints from our **Go/Next.js/RabbitMQ/SKU/API/demo-clock implementation choices**. The uploaded final specification still says Spring Boot; this document makes the agreed Go substitution explicit while retaining behaviour.
- Expands the **full database entity inventory**, REST endpoint checklist, per-role acceptance checks, security/responsiveness/reliability requirements, cross-role effects, detailed test matrix, exact Hackathon submission package and Datathon separation.
- Identifies **five potential API contract gaps** needed to make already-designed actions functional: breakdown, mark-trip-loaded, acknowledge-deferral, notification list/read and store order-list endpoints. These are **implementation questions**, not authorization to invent additional features. Inspect existing `docs/api.md` first and resolve only if missing.
- Flags the need to reconcile **route operational scheduling** (including trip-2 return/reload) with the **official Task 2B formula that excludes return**; never mix these calculations.
- Highlights that prototype demo counts may differ from actual supplied S1 records; **data wins**.
- Adds an explicit definition-of-done and vertical-slice agent protocol, but does not expand user roles or UI scope.

**Do not treat any suggested API completion, optional advanced reference table or documentation clarification as an already-approved product change.** If a requirement is ambiguous, surface it to the team with its source and propose the smallest compliant resolution.
