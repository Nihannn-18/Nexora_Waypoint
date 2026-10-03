# CLAUDE.md — Waypoint Delivery Planning System

Guidance for Claude Code and any other coding agent working in this repository. Read it
fully before changing code. It is the single source of truth for what we are building, how
it is structured and the rules that must not be broken. If code and this file disagree, stop
and raise it — do not silently pick one.

Team **Nexora** · Rootcode Tech-Triathlon 2026 · Hackathon phase.

> Companion checklist: **`AGENTS.md`** consolidates the Challenge Booklet, final
> specification and Designathon into the authoritative build checklists (offline/sync
> contract, non-functional gates, test matrix, delivery package, Datathon separation).
> Read both; this file wins on detail where the two overlap.

---

## 1. Mission

A delivery planning system for **Waypoint Group**: 120 outlets, 60 vehicles, two depots
(Peliyagoda, Kandy), three brands (Fresh, Style, Tech) sharing one fleet. On most days the
fleet cannot serve everyone, so the product's real job is to build a feasible plan **and say
which orders were deferred and why**.

One responsive web app connects four roles through one order record:
Store Manager → Dispatcher → Loader → Driver → Store Manager.

**Deadline: Sunday 4 October 2026, 23:59 Asia/Colombo.** Code pushed later is not judged.

**Our Day 5 Designathon submission is the implementation specification.** Judges score how
faithfully we build it. Screen IDs (D-03, L-02, DG-A1 …) in this file refer to that design;
section 7 lists every screen and the behaviour that must survive implementation.

| Judging criterion                             | Weight | What earns it                                                    |
| --------------------------------------------- | ------ | ---------------------------------------------------------------- |
| Engineering quality and architecture          | 25%    | Clean layering, one constraint authority, tests, docs            |
| Functional completeness across all four roles | 20%    | A judge completes the whole workflow end to end                  |
| Planning and allocation engine                | 20%    | Respects every constraint; handles overcapacity; explains itself |
| Degradation, offline operation, recovery      | 10%    | DG-A, DG-B, DG-C behave as designed                              |
| Fidelity to the Day 5 design                  | 10%    | Same screens, IDs, copy, tokens                                  |
| Demo video                                    | 10%    | —                                                                |
| Creativity                                    | 5%     | —                                                                |

**Definition of done:** on a fresh clone, `cp .env.example .env && docker compose up --build`
starts everything with seed data, the four seeded accounts sign in, and the numbered judge
walkthrough in `README.md` runs start to finish without a dead end.

---

## 2. Rules that must not be broken

1. **One authority on feasibility.** Every hard constraint is implemented once, in the Go
   constraint validator. Never re-implement a rule in a handler, a SQL query or the client.
   The UI may grey out an impossible move; the server revalidates regardless.
2. **Suggestions are never allocations.** A planning run writes `planning_result` rows only.
   `route`, `route_leg` and `allocation` rows are created solely by
   `POST /allocations/confirm`, which revalidates the whole plan against current database
   state inside the transaction that persists it.
3. **Every unserved order is deferred with a reason.** Confirm is rejected if any confirmed
   order is neither allocated nor deferred. A deferral always carries reason type, reason
   text, who decided and when.
4. **Hard rules cannot be overridden. Soft rules can, with a typed reason.** Hard = the
   constraint codes in section 5. Soft = the prioritisation policy (fairness guard). An
   override is stored as `reasonType: POLICY`.
5. **Never persist a derived value as authoritative.** Remaining capacity, minutes and fuel
   are computed from the route and its orders on read.
6. **Use the official trip-time formula exactly** (section 5). Never call a routing or maps
   API, and never substitute another formula.
7. **The contract is paired.** Anything in `libs/shared-types/src/lib/{domain,constraints}.ts`
   has a mirror in `apps/api/internal/domain`. Change both in the same commit. Enums travel as
   UPPER_SNAKE strings.
8. **Time is injected, never read directly.** The business timezone is `Asia/Colombo`. Go code
   takes a `Clock`; it does not call `time.Now()`. The client takes "now" from the API. The
   seeded demo day is in the past, so wall-clock time would break every screen (section 6).
9. **Offline events are idempotent.** The device generates `clientEventId` before any network
   call; the server enforces a unique index on it. Device timestamps are preserved through
   sync. A queued event is never silently dropped or overwritten — conflicts go to the
   dispatcher.
10. **Competition data is confidential.** Work from the schemas in section 6. Do not print,
    paste or upload rows from the supplied CSVs to any external service. Only the files
    listed in section 6 belong in this repository, and a human teammate places them there —
    an agent never adds, moves or regenerates a supplied data file, and never commits
    Datathon training files.
11. **Do not invent scope.** Build the designed screens and the endpoints in `docs/api.md`.
    No extra roles, settings pages, dark mode, i18n or admin tooling.
12. **Leave the repo green.** `npm run verify` passes before every commit.

---

## 3. Repository map

Nx 23 monorepo, npm, `project.json`-based. Node 22, Go 1.24.

**State of the code.** Built and tested: workspace and CI, Docker stack, the shared contract
(`shared-types`), the API client, UI primitives and design tokens, route shells for all four
roles, and in Go: config, router, JSON helpers, health endpoints and the trip-time package.
**Everything else in this file describes what to build** — no database, auth, validator,
planning engine or real screens exist yet.

```
apps/
  web/                  Next.js 16 (App Router) · React 19 · Tailwind v4 · TypeScript 6
    src/app/(auth)/signin   G-01…G-03
    src/app/dispatcher      D-01…D-09, DG-A, DG-C
    src/app/loader          L-01…L-04, L-T1…L-T3
    src/app/driver          R-01…R-04, DG-B
    src/app/store           S-01…S-07
    src/components          web-only shared components
    src/lib                 api.ts (client + token store), roles.ts
  api/                  Go service · module path `waypoint.lk/api`
    cmd/api               HTTP server entrypoint
    internal/config       env settings, validated at start-up
    internal/domain       vocabulary mirrored from shared-types
    internal/httpx        router, JSON helpers, middleware
    internal/planning     trip time, budgets, windows, fuel (tested)
libs/
  shared-types/         @waypoint/shared-types — enums, DTOs, constraint catalogue, formulas
  api-client/           @waypoint/api-client — typed REST client, offline-aware errors
  ui/                   @waypoint/ui — cross-role components + design tokens
docs/                   architecture.md, data-model.md, api.md, prioritisation-policy.md,
                        ai-disclosure.md
infra/postgres/init/    bootstrap SQL (extensions, timezone) — not migrations
docker-compose.yml      postgres 17 · rabbitmq 4 · api · web
```

### Commands (run from the repo root)

- `npm ci` — Install the workspace
- `npx nx dev web` — Web dev server → http://localhost:3000
- `npx nx serve api` — `go run ./cmd/api` → http://localhost:8080
- `docker compose up postgres rabbitmq` — Just the database and queue, for local development
- `npm run verify` — lint + typecheck + test + build, both languages
- `npx nx test <project>` — One project: `web`, `api`, `shared-types`, `api-client`, `ui`
- `npx nx fmt api` / `fmt-check api` — `gofmt -w .` / fail if unformatted
- `npx nx tidy api` — `go mod tidy`
- `npm run reset` — `docker compose down -v` then up — forces a re-seed

Nx has no Go plugin; `apps/api/project.json` wraps the Go toolchain with `nx:run-commands`.
Do not add a third-party Nx Go plugin.

---

## 4. Architecture

```
 Next.js web app (PWA) ── HTTPS · opaque bearer session · /api/v1 ──► Go REST API ──► PostgreSQL (facts, audit)
   4 role workspaces                                   │
   Driver: IndexedDB outbox ─► POST /sync/events       └─ AMQP ─► Planning worker ─► planning_result
```

- **PostgreSQL** is the only store of operational facts and the immutable audit log.
- **Go API** calculates and validates. Handlers are thin; logic lives in packages that are
  unit-tested without a server.
- **Planning worker** runs long allocation jobs so no HTTP request blocks.
  `POST /allocations/suggest` returns `202 {jobId}`; the client polls
  `GET /planning-jobs/{id}`. The job API is the contract. The transport behind it is
  RabbitMQ (`waypoint.planning` exchange, `planning.generate` key, DLQ after retries); an
  in-process goroutine runner implementing the same interface is acceptable until the AMQP
  consumer is wired.
- **Web app** renders and explains. It holds no business rules beyond the mirrored arithmetic
  in `libs/shared-types/src/lib/trip-time.ts`, used only for live meters on D-03.
- **Realtime is polling.** Trip tracker every 30 s (design). Loader list polls and compares
  `routeVersion`. No WebSockets.

### Authentication boundary (Go-owned sessions)

```
Next.js ── POST /api/v1/auth/login (email + password)
        └─> Go API (issue opaque session · verify bearer token · RBAC · depot/outlet scope)
                └─> PostgreSQL (app_user credentials+scope, session hashes)
```

**The Go API owns authentication, sessions and identity.** It verifies the submitted password
against `app_user.password_hash` (Argon2id, constant-time), mints a random opaque session token
and stores only its SHA-256 hash in `session`. Every request presents the token as
`Authorization: Bearer <token>`; the API hashes it, requires a live, unexpired session for an
active `app_user`, loads role and depot/outlet scope, and enforces RBAC and all business rules.
`POST /api/v1/auth/logout` deletes the caller's session. There is no second identity system and
no cross-system mapping. Client-supplied identity headers are never trusted.

### Target Go package layout

Add packages under `apps/api/internal/` as features land. One package per bounded area;
packages depend inward on `domain` and `planning`, never on `httpx`.

- `clock` — `Clock` interface, real and demo implementations
- `store` — `pgxpool` setup, embedded migrations, transaction helper
- `seed` — Embedded reference CSVs + demo-day seeding, idempotent
- `auth` — Verify the opaque bearer session, `RequireRole` middleware, scope checks. Authentication and sessions are owned by the Go API (see §4 Authentication boundary); `authstore` and `authapi` hold the database and HTTP pieces.
- `catalog` — Outlets, vehicles, items, calendar, district travel, service allowance
- `orders` — Order lifecycle, cutoff, order numbers, queue, close
- `constraint` — **The validator.** Pure functions over loaded state → `[]domain.ConstraintResult`
- `planning` — Trip time (exists), `PlanningEngine`, prioritisation, job runner
- `routes` — Routes, legs, reorder, dispatch, ETA, live state, breakdown re-plan
- `deferrals` — Deferral log, protected-next-run flag, repeat-skip detection
- `loading` — Load items, shortfall flags
- `delivery` — Delivery events, POD, offline sync reconciliation
- `receipts` — GRN and issues
- `notify` — Notifications to users and outlets
- `forecast` — Demand forecast read endpoint
- `audit` — Append-only audit log

Each feature package exposes a `Service` (logic, takes a `Clock` and a store) and a
`Handler` (HTTP only). Wire them in `httpx.Router`.

### Fixed backend choices — do not introduce alternatives

Standard-library `net/http` `ServeMux` with method patterns (`"POST /api/v1/orders"`,
`r.PathValue("id")`) · `log/slog` · `github.com/jackc/pgx/v5` with hand-written SQL, no ORM
· `github.com/pressly/goose/v3` with numbered SQL migrations in
`apps/api/internal/store/migrations/`, embedded in the binary and applied at start-up ·
`github.com/rabbitmq/amqp091-go`. Authentication and session are owned by the Go API with
opaque, database-backed sessions and Argon2id password hashes (`golang.org/x/crypto/argon2`);
Go verifies the bearer token and enforces RBAC, scope and domain rules. Do not add a JWT or
an external auth framework as the mechanism.

---

## 5. Domain

### Network

- Brands: **Fresh** 80 outlets (daily, before stores open), **Style** 25 (weekly; about half
  in malls), **Tech** 15 (as needed; heavy, fragile).
- Fleet: 12 reefer trucks, 40 dry-box trucks, 8 vans (4 refrigerated) → **16 vehicles can
  carry chilled**. Reefer capacity is the scarce resource.
- 12 districts. Each outlet and each vehicle belongs to exactly one depot.
- Operates **Monday to Saturday**. Use `calendar_day.is_operating`, never weekday arithmetic.
- Orders for the next operating day close at **16:00**. Later orders are **accepted** for the
  following operating day with a warning (S-02b) — never rejected.
- A Fresh outlet may hold **two orders for one delivery day**: one chilled, one dry. Never
  merge them.
- The dispatcher plans **both depots** from Peliyagoda. A planning run is per depot.

### Order lifecycle

```
PLACED → CONFIRMED → ALLOCATED → LOADED → IN_TRANSIT → DELIVERED → RECEIVED
   └─────────┴───────────┴──────────┴──────► DEFERRED → re-enters a later run
IN_TRANSIT → FAILED → DEFERRED
```

`DELAYED` is a delivery **outcome**, not an order status. Legal transitions are in
`ORDER_STATUS_TRANSITIONS` (`libs/shared-types/src/lib/domain.ts`); the server enforces them.

### Hard constraints

Catalogue: `libs/shared-types/src/lib/constraints.ts` ⇄ `apps/api/internal/domain/constraints.go`.
`E-0x` is the rule group shown in the D-03 rule panel — keep the IDs visible in the UI.

- `WEIGHT_CAPACITY_EXCEEDED` · E-01 — Σ order weight on the trip ≤ `weight_cap_kg`
- `VOLUME_CAPACITY_EXCEEDED` · E-01 — Σ order volume on the trip ≤ `volume_cap_m3`
- `ORDER_SPLIT_FORBIDDEN` · E-01 — An order rides exactly one vehicle and trip
- `DUPLICATE_ASSIGNMENT` · E-01 — An order is not already allocated elsewhere
- `REEFER_REQUIRED` · E-02 — Chilled/frozen order → vehicle `temp = reefer`
- `VAN_ONLY_ACCESS` · E-03 — `parking_constraint = van_only` → vehicle `type = van`
- `DEPOT_MISMATCH` · E-04 — Vehicle depot = outlet depot
- `TRIP_LIMIT_EXCEEDED` · E-05 — ≤ 2 trips per vehicle per day
- `TRIP_NUMBER_INVALID` · E-05 — `trip_no ∈ {1, 2}`
- `FRESH_TIME_BUDGET` · E-05 — Σ Fresh trip minutes for the vehicle that day ≤ **270**
- `STYLE_TECH_TIME_BUDGET` · E-05 — Σ Style **+** Tech trip minutes ≤ **480** (one shared pool)
- `BRAND_DISTRICT_MIX` · E-05 — One brand and one district per vehicle + trip
- `VEHICLE_UNAVAILABLE` · E-05 — Vehicle not `in_workshop` / unavailable / broken down on the date
- `NON_OPERATING_DAY` · E-05 — Planning date has `is_operating = 1`
- `DELIVERY_WINDOW_MISSED` · E-06 — Planned arrival ≤ outlet `window_close_time`
- `MALL_WINDOW_MISSED` · E-06 — `mall_dock` outlet: arrival inside the mall window
- `FUEL_QUOTA_EXCEEDED` · E-07 — Vehicle's ISO-week fuel + this route ≤ `weekly_fuel_quota_l`

Boundaries: **exactly at a limit passes; one unit over fails.** Each rule needs a passing,
a failing and a boundary test. The validator returns **every** rule's verdict, passed and
failed, with a `detail` naming the binding quantity (`"2840 / 2500 kg"`).

### Formulas

```
trip_minutes   = outbound + inter_stop + handling
outbound       = district_travel.depot_to_district_freeflow_min          (once per trip)
inter_stop     = district_travel.inter_stop_freeflow_min × max(n − 1, 0)  (n = orders on trip)
handling       = Σ service_allowance_min(brand, outlet.dock_type)
```

The return journey is **not** counted. Pinned vector in both languages:
37 + 9×2 + (15+15+16) = **101 minutes**.

```
arrival < window_open  → wait; service starts at window_open
late                   = arrival > window_close        (arrival AT close is on time)
fuel_litres            = route_distance_km / km_per_l
```

Implemented and tested in `apps/api/internal/planning/triptime.go` (authoritative) and
`libs/shared-types/src/lib/trip-time.ts` (mirror). Change both, and both test files, together.

### Prioritisation policy (soft)

Applied only after feasibility, in this order. Full text: `docs/prioritisation-policy.md`.

1. Never defer an outlet that was deferred on the previous run ("protected next run").
2. Protect outlets unserved for 7 or more days.
3. Chilled before ambient.
4. Narrow delivery windows before wide ones.
5. Among equals, the largest order that still fits.
6. Style and Tech move before Fresh.

The engine's output must say, per deferral: the binding constraint, its `E-0x` ID, and a
plain-language explanation. Stores show an **arrival window**, never a single exact minute.

### Assumptions we chose (also recorded in the README)

- Fresh loading starts 03:30; Fresh trip 1 departs **03:45**. Style/Tech trip 1 departs 09:00.
  These are configuration values, not constants scattered in code.
- Trip 2 departs after trip 1's last stop + the outbound time back + 20 minutes reload.
- Fuel distance per route = `depot_to_district_km + inter_stop_km × max(n − 1, 0)` — outbound
  and inter-stop legs only, matching the supplied route-leg records, which have no return leg.
- The dataset's `temp_requirement` is `chilled` or `ambient` only. `FROZEN` exists in the enum
  and is treated exactly like chilled; do not build separate UI for it.

---

## 6. Data and seeding

### Supplied reference files (column names are exact)

- `outlets.csv`: `outlet_id` (OUT001–OUT120), `brand` (Fresh/Style/Tech), `district`, `depot` (Peliyagoda/Kandy), `dock_type` (rear_dock/street/mall_bay), `parking_constraint` (normal/van_only/mall_dock), `mall_window` (`HH:MM-HH:MM` or blank), `window_open_time`, `window_close_time`
- `vehicles.csv`: `vehicle_id` (VEH001–VEH060), `type` (truck/van), `temp` (reefer/ambient), `weight_cap_kg`, `volume_cap_m3`, `fuel_type`, `km_per_l`, `weekly_fuel_quota_l`, `depot`
- `calendar.csv`: `date`, `dow` (0 = Monday), `dow_name`, `is_weekend`, `iso_year`, `iso_week`, `is_payday`, `festival`, `festival_ramp` (0–1), `is_holiday`, `monsoon`, `is_operating`
- `district_travel.csv`: `district`, `depot`, `road_class`, `free_flow_kmh`, `depot_to_district_km`, `depot_to_district_freeflow_min`, `inter_stop_km`, `inter_stop_freeflow_min`
- `service_allowance.csv`: `brand`, `dock_type`, `service_allowance_min`
- `task2b_peak_day_scenarios.csv`: `scenario` (S1), `order_ref`, `outlet_id`, `brand`, `district`, `depot`, `dock_type`, `parking_constraint`, `mall_window`, `window_open_time`, `window_close_time`, `temp_requirement` (chilled/ambient), `order_units`, `order_weight_kg`, `order_volume_m3`, `deferred_yesterday` (0/1), `days_since_last_served`
- `task2b_peak_day_fleet.csv`: `scenario` (S1), `vehicle_id`, `status` (available/in_workshop)

Clock times are `HH:MM` in Asia/Colombo. CSV values are lower_snake; convert to UPPER_SNAKE
enums on load (`van_only` → `VAN_ONLY`, `Fresh` → `FRESH`). `outlets.csv` has no outlet name
and `vehicles.csv` has no registration number — the seed generates display names; IDs are
the identifiers everywhere.

### Seeding rules

- Seed files live in `apps/api/internal/seed/data/` and are embedded with `//go:embed`, so the
  API image needs no volume. Seeding runs at start-up after migrations and is idempotent.
- **The demo day is Task 2B scenario S1**: planning day Fri 25 Sep 2026, delivery day
  **Sat 26 Sep 2026**, Peliyagoda. S1 orders keep `order_ref` as their order number. They
  arrive as aggregates; the seed creates order lines whose totals equal the CSV values
  exactly — never adjust a total.
- `deferred_yesterday` and `days_since_last_served` seed the deferral history that the
  fairness guard and D-07 read.
- The design quotes 186 orders, 54 vehicles available, 6 in workshop, 22 deferred. If the
  seeded data produces different numbers, **the data wins** — update the README walkthrough,
  never the data.
- Seeded accounts (password `waypoint2026`): `priyantha.w@waypoint.lk` Dispatcher ·
  `nadeesha.p@waypoint.lk` Loader, Peliyagoda Bay 2 · `kasun.p@waypoint.lk` Driver, VEH014 ·
  `ishara.s@waypoint.lk` Store Manager, OUT014.

### The demo clock

Judges run this days after the seeded date, and the walkthrough spans Friday 16:00 to
Saturday morning. So:

- `internal/clock` provides `Now()`. With `DEMO_MODE=true` the clock starts at
  `DEMO_CLOCK_START` (default `2026-09-25T15:40:00+05:30`) and ticks from there.
- `POST /api/v1/demo/clock {stage}` (dispatcher, demo mode only) jumps to a named stage:
  `BEFORE_CUTOFF` Fri 15:40 · `AFTER_CUTOFF` Fri 16:05 · `LOADING` Sat 03:30 · `ON_ROUTE` Sat 05:00.
  `POST /api/v1/demo/reset` re-seeds.
- `GET /api/v1/meta` returns `now` and `demoMode`; the web app derives every countdown and
  "today" from it, never from the browser clock.

---

## 7. Features by role

Device targets: Dispatcher and Store Manager desktop 1440; Loader dock tablet 1024 and phone
402; Driver phone 402. **Judges test Loader and Driver on phone-sized screens.**

### Shared

- **G-01 – G-03** · `/signin` — Four role cards; picking one pre-fills its seeded account. Error state names the problem and is not colour-only. Short loading state, then route to the role workspace.

### Dispatcher — Priyantha (desktop)

- **D-01** · `/dispatcher` — KPI cards (queue size, vehicles available, trips allocated, deferrals) each linking to the screen that resolves it. Alerts phrased as decisions with a one-click fix. Live countdown to 16:00 in the top bar.
- **D-02, a, b** · `/dispatcher/queue` — All brands' confirmed orders for the delivery day; filters (Fresh chilled, Style, Tech); card shows temperature, window, access, load; detail shows lines. **Close queue** is an explicit action with a per-brand confirmation and a warning about unconfirmed orders. Closed queue is read-only, shows late orders rolling to the next operating day, and offers only "Start planning".
- **D-03, a** · `/dispatcher/plan` — Unallocated orders left; vehicle lanes with Trip 1 / Trip 2 and live weight, volume, time-budget and weekly-fuel meters; right panel lists every rule with its E-0x ID and "Why this plan". **Auto-allocate** proposes; the dispatcher confirms. A violating drop turns the trip red, names the failing rules and offers a feasible alternative in one click.
- **DG-A1, DG-A2** · `/dispatcher/plan` — Overcapacity state — see section 8.
- **D-04** · modal on plan — Defer with a reason picker of constraint-based reasons; **Defer disabled until one is chosen**; new date marked "protected"; one reason can be applied to all orders blocked by the same constraint.
- **D-05** · state on plan — Plan published: loaders get lists, stores get arrival windows or deferral notices. Publish becomes a disabled confirmation so a plan cannot be sent twice.
- **D-06** · `/dispatcher/tracker` — Per trip: stops done / total, next stop, ETA against window, driver, sync status. An offline driver shows **"Offline since HH:MM · events queued" in neutral grey**, not late. Problems panel ranks shortfalls, delays, breakdowns with a recommended action. Polls every 30 s.
- **DG-C1** · drawer on tracker — Breakdown re-plan — see section 8.
- **D-07, a** · `/dispatcher/deferrals` — Searchable by outlet; each row has constraint, decision maker, new date. **Outlets skipped twice in 14 days are highlighted.** Empty state explains why and gives a next step.
- **D-08** · `/dispatcher/forecast` — Read-only. Chilled volume against reefer capacity by week; paydays and festivals flagged from the calendar; recommendations. Fed by `GET /forecast/demand`.
- **D-09** · `/dispatcher/fleet` — Vehicles with type, temperature, depot, availability; filters refrigerated / dry-box / van.

### Loader — Nadeesha (tablet + phone)

- **L-01** · `/loader` — Today's trips for her bay ordered by departure, with progress and status chips (Not started, Loading, Ready, Flagged). Empty state before the plan is published.
- **L-02** · `/loader/trips/[routeId]` — Rows in **reverse stop order — last drop loads first**. Per-order check-off. Row shows stop number, units, weight, volume, temperature, unloading type. **"Mark trip loaded" disabled until every row is checked or flagged.**
- **L-02a** · state on L-02 — A live plan change updates the list **in place**, attributes the change, and **outlines changed rows — never re-orders them under her fingers**.
- **L-03, a** · sheet on L-02 — Flag an item: item, missing vs damaged, quantity, optional photo. Validation says what is needed and why. Recorded against the **order line**, never the trip.
- **L-04** · state — "Flag sent" names who now knows: dispatcher, driver, store.
- **L-T1 – L-T3** · same routes at ≥ 1024 — Master–detail: trips left, loading list right in two columns; the flag form opens as a side panel so the list stays visible.

Deferred orders never appear on a loading list.

### Driver — Kasun (phone)

- **R-01** · `/driver` — One scrolling cockpit: active run, current stop with window and unloading bay, planned route with completed stops, POD. A **"Data mismatch" alert carries the loader's shortfall flag**.
- **R-02, a** · `/driver/stops/[legId]/outcome` — Delivered / Failed / Delayed as three large choices. **Failed requires a reason chip**; optional note and photo. POD = receiver name plus signature or photo. Copy under the action: "Works without signal — saved on your phone first".
- **DG-B1–3** · cockpit + `/driver/log` — Offline behaviour — see section 8.
- **R-04** · `/driver/log` — All synced: states how many events matched the plan, that there were no conflicts, and what the store now sees.

Every driver action works offline. Primary actions are 56 px. Designed for use only when
safely stopped.

### Store Manager — Ishara (desktop + phone)

- **S-01** · `/store` — Tomorrow's orders and their state, today's delivery, the cutoff countdown, any deferrals. Shortcut tiles to Place order, Order status, Delivery receipts.
- **S-02, a, b** · `/store/order` — **Chilled and dry are separate orders.** Running weight/volume and cutoff countdown. Delivery date defaults to the next operating day. Errors marked in place and summarised at top. After 16:00 the order is accepted for the following operating day with a clear warning.
- **S-03** · state — Immediate confirmation with order number and next steps. A "Submitting" state prevents double submission.
- **S-04** · `/store/orders/[orderId]` — Arrival **window**, stops away, driver, and the loader's shortfall flag shown in advance.
- **S-05** · same route, deferred — Fact first ("Moved to Mon 28 Sep — first run"), then the reason in one plain sentence, who decided and when, and that the outlet is **protected on the next run**. Acknowledge is the primary action.
- **S-06, b** · `/store/orders/[orderId]/receipt` — GRN: expected vs received per line with condition; loader flag pre-filled; driver's POD attached. Success state shows the GRN and issues raised.
- **S-07** · `/store/orders` — Ongoing tab: one row per live order, click opens a details overlay. Completed tab: last seven days with outcomes and a Reorder shortcut.

Route folders that do not exist yet are created as their screens are built. Replace each
`ScreenStub` (`apps/web/src/components/screen-stub.tsx`) when its screen lands.

---

## 8. Degradation scenarios

**DG-A · Overcapacity Day (primary).** Auto-allocate hits the limit. DG-A1 leads with the
cause in an amber banner ("N of M orders can't be served on Sat 26 Sep — refrigerated capacity
is the limit"), shows meters for which resource ran out (reefer trips, Fresh time budget,
van-only access, dry-box capacity, weekly fuel) and "How the engine chose". Each proposed
deferral names its blocking constraint and requirement ID. **Save plan is disabled until
every deferral has a reason**; bulk-apply by constraint; overrides need a justification.
DG-A2: all reasons set, Save unlocks, "Notify N stores with reason" is checked by default.
Then D-05 → S-05 → D-07.

**DG-B · Driver offline mid-route.** Offline is normal, not an error. A persistent bar says
"No signal · N actions saved on this phone" (DG-B1); every button still works; an "Everything
still works" card replaces any error. DG-B2: success copy is honest — "saved on this phone,
uploading automatically"; next stop is the primary action. DG-B3: the Log lists each queued
event with its capture time in upload order. On reconnect the outbox flushes in order → R-04.
Offline UI is **neutral grey, never red**.

**DG-C · Vehicle breakdown mid-route.** From D-06 the dispatcher marks a vehicle broken
down. The DG-C1 drawer re-queues **only unserved stops**, recommends a replacement vehicle
showing each check (capacity, mall window, arrival, trips per day, time budget, home depot,
fuel), lists failing vehicles as "not available" with the reason, and offers deferral as the
fallback. "Notify stores and the driver" is on by default.

---

## 9. API conventions

Base `/api/v1`. Go opaque bearer session except login. Full list with payloads: `docs/api.md`.
Adding or changing an endpoint is three edits in one commit: the Go handler, the
`WaypointClient` method (`libs/api-client/src/lib/waypoint-client.ts`) and `docs/api.md`.

- JSON is camelCase. Decode strictly (`httpx.DecodeJSON` rejects unknown fields).
- One error shape: `{ message, code?, constraintResults? }` via `httpx.WriteError` /
  `httpx.WriteConstraintViolation`.
- `400` malformed · `401` no/expired token · `403` out of role · `404` missing or out of scope
  · `409` stale `routeVersion` · `422` infeasible, with `constraintResults`.
- `POST /allocations/validate` returns `200 { valid: false, … }` — probing is not an error.
  Only an attempted confirm returns `422`.
- Scope is derived from the user record server-side: store manager → own outlet; loader and
  driver → own depot and routes; dispatcher → both depots, filtered by `depotId`.
- Totals (units, weight, volume, temperature) are computed from order lines on the server and
  never accepted from the client. SKU dimensions are snapshotted onto the order line.
- Loader shortfall invariant: `loaded + damaged + missing = ordered`, checked against the
  database's ordered quantity.

---

## 10. Backend conventions (Go)

- Handlers parse, authorise, call a service, write the response. No arithmetic, no SQL.
- Services take a `context.Context` first and return errors; wrap with context
  (`fmt.Errorf("confirming allocation: %w", err)`). Log once, at the HTTP boundary.
- Persistence structs never leave their package; map to response types with `json` tags.
- Multi-row state changes run in one transaction. Route edits use optimistic locking on
  `route_version`.
- Migrations are forward-only numbered SQL files. Enforce in the database:
  `UNIQUE (vehicle_id, route_date, trip_no)`, `CHECK (trip_no IN (1, 2))`,
  `UNIQUE (route_id, seq)`, `UNIQUE (client_event_id)`.
- Table and column names follow `docs/data-model.md`. Natural keys for outlets (`OUT014`) and
  vehicles (`VEH014`); UUIDs elsewhere.
- Every deferral, allocation, route change, shortfall and delivery outcome writes an
  `audit_log` row with actor and timestamp.
- Table-driven tests; `internal/planning/triptime_test.go` is the pattern. The constraint
  validator and planning engine are pure and must be testable without a database.
- Run `npx nx fmt api` before committing; CI fails on unformatted Go.

---

## 11. Frontend conventions

**Framework notes — these versions are newer than most training data. Check before assuming.**

- **Next.js 16, App Router.** `params` and `searchParams` are Promises: `await` them (or
  `use()` in client components). Request interception lives in `proxy.ts`, not
  `middleware.ts`. Server Components by default; add `'use client'` only where state, effects
  or browser APIs are needed.
- **Tailwind v4.** There is no `tailwind.config.js`. Tokens are CSS variables in the `@theme`
  block of `apps/web/src/app/global.css`. A library that renders markup must be listed in an
  `@source` line there or its classes compile to nothing.
- **TypeScript is strict** with `noUncheckedIndexedAccess`. No `any`; no non-null `!` to
  silence the compiler.

**Design system (from the Day 5 style guide)**

- Never hard-code a colour. Use tokens: `bg-page` `bg-card` `text-ink` `text-ink-muted`
  `text-brand` `text-link` `bg-action` `text-success` `text-warning` `text-error`
  `bg-offline`. Amber (`warning`) marks degradation states.
- **Status is never shown by colour alone** — always a label and a glyph/icon. Use
  `StatusBadge` / `OrderStatusBadge` from `@waypoint/ui`.
- IDs and times are monospace with tabular figures: `<Mono>`, `<ClockTimeText>`,
  `<MinuteDelta>`. Times are 24-hour `HH:MM`; dates read "Sat 26 Sep"; kg and m³.
- Touch targets: `tap-target` (48 px) everywhere on phone/tablet; `tap-target-driver` (56 px)
  for Driver primary actions. Minimum 14 px text on phone.
- Layout: desktop sidebar 252 px (`w-sidebar`), top bar 60 px (`h-topbar`) carrying the date
  and the cutoff countdown; phone = app bar, scrolling body, 4-tab bar; tablet = master–detail.
- Radius: `rounded-chip` (2), `rounded-control` (8), `rounded-card` (12), `rounded-pill`.
- Microcopy says what happened, why, and what happens next. Toasts close after 2.5 s.
  Loading states on Sign in, Publish plan, Submit order and Send GRN block double submission.
- Every list has an empty state that explains itself. Every form marks errors in place.

**Code organisation**

- All API access goes through `api` from `apps/web/src/lib/api.ts`. No raw `fetch`.
- `WaypointApiError.isOffline` means the request never reached the server — queue and retry.
  `isConstraintViolation` means a 422 with rule results — show them, do not retry.
- A component used by one role stays in that role's folder; cross-role components go in
  `libs/ui`. Types shared with the API go in `libs/shared-types`, never redefined locally.

**Driver offline design**

- IndexedDB via **Dexie**: a `routes` cache (the run sheet, saved when online) and an
  `outbox` of events `{ clientEventId, legId, payload, occurredAt, status }`.
- Recording an outcome writes to the outbox **first**, updates the UI from local state, then
  attempts the upload. Flush on reconnect in capture order with `POST /sync/events`; handle
  each result (`ACCEPTED`, `DUPLICATE`, `REJECTED`, `CONFLICT`) independently.
- `clientEventId = crypto.randomUUID()` at capture. `occurredAt` is the device clock.

---

## 12. Testing

- Jest for TypeScript (`*.spec.ts(x)` beside the source), `go test` for Go. `npx nx test <project>`.
- Mandatory backend cases: every constraint passing, failing and exactly at its boundary;
  the trip-time vectors; a post-cutoff order rolling to the next operating day; the shortfall
  invariant; a duplicate `clientEventId` persisting one event; a stale `routeVersion`
  returning 409; a breakdown re-queuing only unserved stops; confirm rejecting a plan that
  was valid when suggested but is not valid now.
- Add a test with every bug fix. Do not delete or weaken a test to make a change pass.
- UI tests assert behaviour a judge would notice (disabled until valid, reverse stop order,
  reason required), not markup details.

---

## 13. Git workflow

- Branch from `development`: `feat/<area>-<thing>`, `fix/<area>-<thing>`. Small pull requests.
- Commit messages: imperative summary, then why. Keep whatever attribution trailer your
  agent adds.
- Never commit `.env`, secrets, `node_modules` or build output. Supplied data files are added
  by a teammate only (rule 10).
- Before pushing: `npm run verify`. CI runs lint, typecheck, test, build and both image builds.
- If AI assistance produced the change, add a line to `docs/ai-disclosure.md` — the
  disclosure is a judged deliverable and must be accurate.
- Record any departure from the Day 5 design in the README's departures section, with the
  reason.

---

## 14. Build order and cut line

Each step leaves `main` working. Backend and frontend proceed in parallel against
`docs/api.md`; the frontend may use fixture data shaped by `@waypoint/shared-types` until an
endpoint exists.

**Backend**

1. `clock`, `store`, migrations for the full schema in `docs/data-model.md`
2. `seed`: reference CSVs, four accounts, demo day S1
3. `auth`: issue and verify opaque sessions, login/logout/me, `RequireRole`, scope
4. `catalog` + `orders`: create, confirm, queue, close, cutoff
5. `constraint`: the validator with a test per rule
6. `planning`: engine + prioritisation, job API, results; then validate / confirm
7. `deferrals` + `notify`
8. `routes`: legs, live state, dispatch · `loading`: shortfalls
9. `delivery`: events + `/sync/events` · `receipts`
10. Breakdown re-plan · `forecast` · demo clock endpoints

**Frontend**

1. Sign-in with real auth and role routing
2. Store: place order → status
3. Dispatcher: queue + close → plan board → overcapacity + defer → publish
4. Loader: trips → reverse-order list → flag
5. Driver: cockpit → outcome → offline outbox → log
6. Store: deferral notice → GRN
7. Dispatcher: tracker → deferral logs → fleet → forecast → breakdown drawer

**Must ship (the judge walkthrough depends on it):** auth for four roles; order placement and
cutoff; queue close; auto-allocate with the full validator; overcapacity deferrals with
mandatory reasons; publish; loader list in reverse order with item-level flag; driver outcome
with POD, offline queue and sync; store status, deferral notice and GRN; deferral logs;
trip tracker; `docker compose up` with seed; README walkthrough; deployed URL.

**Cut first if time runs out, in this order:** D-08 forecast (ship static, read-only) →
D-09 fleet filters → tablet master–detail polish → DG-C breakdown drawer → drag-and-drop on
D-03 (buttons suffice) → RabbitMQ transport (keep the in-process runner) → service-worker
app-shell caching. Never cut: the validator, deferral reasons, offline sync idempotency.

---

## 15. Datathon boundary

The Datathon (due 9 Oct) is judged separately and is **not** part of this build. The only
touchpoint is `GET /forecast/demand`, which may serve static seeded figures. Its rules bind
anyone working on it, agents included: no pre-trained models, no proprietary API-based
modelling or preprocessing, no low-code or fully automated end-to-end modelling tools. An
agent must not run an automated end-to-end modelling pipeline, must not read Datathon data
rows into its context, and must never send them to an external service. Modelling decisions
are the team's.

---

## 16. Where to look

- Components, request flows, degradation — `docs/architecture.md`
- Tables, columns, database constraints — `docs/data-model.md`
- Every endpoint with payloads — `docs/api.md`
- Fairness policy in full — `docs/prioritisation-policy.md`
- Go handoff and conventions — `CONTRIBUTING.md`, `apps/api/README.md`
- Enums, DTOs, constraint catalogue — `libs/shared-types/src/lib/`
- Design tokens — `apps/web/src/app/global.css`, `libs/ui/src/lib/tokens.ts`
- Judge walkthrough, seeded accounts — `README.md`
