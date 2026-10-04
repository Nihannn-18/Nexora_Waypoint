# Data model

PostgreSQL is the authoritative store of operational facts. Required Hackathon deliverable.

**The rule that shapes everything below:** derived values — remaining capacity, remaining
minutes, remaining fuel — are **never** persisted as authoritative. They are computed from a
route and its current orders on read. A stored copy can go stale after a route edit, and a
stale number that looks authoritative is worse than no number.

## Schema and seed files

- **Schema:** numbered forward-only goose migrations under
  `apps/api/internal/store/migrations/`, embedded in the binary and applied at start-up
  (`store.Migrate`): `00001_init.sql` (the full schema), `00002_load_item_photo_ref.sql`
  (adds `load_item.photo_ref`) and `00003_order_service_history.sql` (adds the deferral
  history columns to `customer_order`) and `00004_load_item_unique.sql` (adds
  `UNIQUE (route_id, order_item_id)` on `load_item`) and `00005_notification_unique.sql` (adds
  `UNIQUE (user_id, type, reference)` on `notification`) and `00006_go_auth.sql` (adds
  `app_user.password_hash`/`display_name` and the `session` table). A new change is a new
  numbered file, never an edit to an applied one.
- **Reference seed:** the five operational CSVs are copied into
  `apps/api/internal/seed/data/` and embedded with `//go:embed` (`apps/api/internal/seed`).
  Seeding runs after migrations and is idempotent (upsert on the natural key). It is seeded
  from Go, not SQL — the numbered migrations are schema changes only, never reference inserts.
- **Source of truth for the data:** `docs/general-data/` remains the human-supplied original;
  the copies under `seed/data/` are what the binary embeds. Do not edit the copies by hand.
- **Demo accounts:** the seed upserts the four `app_user` rows with role, depot/outlet scope
  and an Argon2id `password_hash` (plaintext from `DEMO_SEED_PASSWORD`). `user_id` is a stable
  placeholder (`seed-dispatcher`, …) and a re-seed never rewrites it on an existing email.
- **Demo day (Task 2B S1):** the two `task2b_peak_day_*.csv` files are embedded and seeded:
  85 orders (`order_ref` is the order number, status `CONFIRMED`, ordered Fri 25 Sep for
  Sat 26 Sep) and a `vehicle_daily_availability` row for all 60 vehicles on Sat 26 Sep. The
  fleet file lists 38 vehicles (28 available, 10 in workshop); a vehicle it omits is treated as
  available. Orders arrive as aggregates, so each gets one line of a stand-in `DEMO-<BRAND>-<TEMP>`
  item whose totals are the CSV values verbatim; the line's unit snapshots are derived from them
  and may differ from the totals by rounding. An existing order number is never overwritten, so a
  re-seed cannot rewind an order that has moved through the lifecycle.
- **Demo-day calendar window (seed-side extension).** The supplied `calendar.csv` ends on
  2026-06-28, before the demo delivery day, so relying on it alone would make
  `POST /allocations/suggest` fail with a missing `calendar_day` row. The seed therefore derives
  and upserts the demo window (25 Sep – 3 Oct 2026) **without editing the supplied CSV**:
  Monday–Saturday are operating and Sunday is not, the convention the file itself records. The
  insert is `ON CONFLICT (date) DO NOTHING`, so a supplied row always wins and a re-seed changes
  nothing. `calendar_day.is_operating` remains the single authority; no business logic derives an
  operating day from the weekday.
- **Deferral history:** `customer_order.deferred_yesterday` and `days_since_last_served`
  (migration `00003`) carry the S1 history that the fairness guard and D-07 read. The source
  rows are per order; the prioritisation policy (rules 1–2) is stated per **outlet**, so the
  planning engine must aggregate these to the outlet level (a repeated skip is an outlet
  property). The seed stores the supplied values verbatim; it does not interpret them.
- **Not seeded:** a real item catalogue (the scenario has only aggregates);
  `traffic_speed` and `road_condition` are Datathon-only
  and carry no rows.
- **Display names.** The supplied CSVs contain no outlet names or vehicle registrations. The
  seed derives a deterministic display name from the identifier: `Outlet OUT001 … Outlet
OUT120` and `Vehicle VEH001 … Vehicle VEH060`. The natural identifier remains the key
  everywhere; names are display-only.

---

## Entity relationships

```
depot ──┬─< outlet ──< customer_order ──< order_item >── item
        │                    │                 │
        │                    │                 └──< order_item_delivery
        │                    │
        │                    ├──< allocation >── route
        │                    ├──< deferral_log
        │                    └──< receipt
        │
        ├──< vehicle ──┬──< route ──< route_leg ──< delivery_event
        │              │                              │
        │              ├──< vehicle_daily_availability │
        │              └──< vehicle_fuel_usage         │
        │                                             │
        └──< app_user ────────────────────────────────┘
                 │
                 └──< audit_log

reference, seeded and read-only:
  calendar_day · district_travel · service_allowance · traffic_speed · road_condition

planning, transient:
  planning_job ──< planning_result
```

---

## Master data

| Table      | Key columns                                                                                                                                                                     | Notes                                                                                                                                                                                                                                                                                                                                                                              |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `depot`    | `depot_id` PK, `code`, `name`, `is_active`                                                                                                                                      | Peliyagoda and Kandy.                                                                                                                                                                                                                                                                                                                                                              |
| `outlet`   | `outlet_id` PK, `name`, `brand`, `district`, `depot_id` FK, `dock_type`, `parking_constraint`, `mall_window_open`, `mall_window_close`, `window_open_time`, `window_close_time` | OUT001–OUT120, from `outlets.csv`. `parking_constraint` is one column with three values: `normal`, `van_only` (no trucks) or `mall_dock` (the mall window applies). `mall_window` arrives as `HH:MM-HH:MM` and is split on load; blank outside malls. `name` is seed-generated — the CSV has none. `dock_type` drives the service allowance lookup.                                |
| `vehicle`  | `vehicle_id` PK, `type`, `temp`, `weight_cap_kg`, `volume_cap_m3`, `fuel_type`, `km_per_l`, `weekly_fuel_quota_l`, `depot_id` FK                                                | VEH001–VEH060, from `vehicles.csv`: 12 reefer trucks, 40 dry-box trucks, 8 vans (4 refrigerated). Day-to-day availability lives in `vehicle_daily_availability`, not here.                                                                                                                                                                                                         |
| `item`     | `item_id` PK, `sku` UNIQUE, `brand`, `unit_weight_kg`, `unit_volume_m3`, `temperature_requirement`                                                                              | Real SKUs, not aggregates.                                                                                                                                                                                                                                                                                                                                                         |
| `app_user` | `user_id` PK, `email` UNIQUE, `display_name`, `password_hash`, `role`, `depot_id` nullable, `outlet_id` nullable, `is_active`                                                   | The application identity and the credential. Authentication is owned by the Go API: `password_hash` is an Argon2id PHC string (never plaintext), verified in constant time at `POST /api/v1/auth/login`. Nullable scope columns: a dispatcher has a depot and no outlet, a store manager the reverse. Enforced server-side on every query, never trusted from a token claim alone. |

### Sessions (`00006_go_auth.sql`)

Opaque, server-side sessions. A login mints a random 256-bit token and stores only its SHA-256
hash; the raw token is returned to the client once and presented as `Authorization: Bearer`.

| Table     | Key columns                                                                                                   | Notes                                                                                                                                                                         |
| --------- | ------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `session` | `session_id` PK, `user_id` FK → `app_user` ON DELETE CASCADE, `token_hash` UNIQUE, `expires_at`, `created_at` | A request hashes the bearer token and requires a matching row with `expires_at > now()` for an active user. Logout deletes the row; expired rows are rejected, never revived. |

---

## Orders

| Table            | Key columns                                                                                                                                                                                                                                               | Notes                                                                                                                                                                                                                                                                |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `customer_order` | `order_id` PK, `order_number` UNIQUE, `outlet_id` FK, `brand`, `order_date`, `requested_delivery_date`, `total_units`, `total_weight_kg`, `total_volume_m3`, `temp_requirement`, `status`, `after_cutoff`, `deferred_yesterday`, `days_since_last_served` | Header. Totals are computed from lines on write, not supplied by the client. `deferred_yesterday` and `days_since_last_served` (migration `00003`) carry the supplied S1 fairness history; the policy reads them per outlet, so the planning engine aggregates them. |
| `order_item`     | `order_item_id` PK, `order_id` FK, `item_id` FK, `quantity`, `unit_weight_kg_snapshot`, `unit_volume_m3_snapshot`, `total_weight_kg`, `total_volume_m3`                                                                                                   | The snapshot columns are the point: a catalogue correction next week must not retroactively change this order's weight, because that would retroactively invalidate a plan that was feasible when it was made.                                                       |

Aggregation, computed on write:

```
total_units  = Σ quantity
total_weight = Σ (quantity × unit_weight_kg_snapshot)
total_volume = Σ (quantity × unit_volume_m3_snapshot)
temp         = CHILLED/FROZEN if ANY line requires it, else AMBIENT
```

A Fresh outlet may have **two orders for the same delivery day** — one chilled, one dry.
They are never merged: one needs a reefer and the other does not.

---

## Reference data, seeded from the supplied CSVs

| Table                        | Key columns                                                                                                                               | Used for                                                                |
| ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `calendar_day`               | `date` PK, `dow`, `is_weekend`, `iso_year`, `iso_week`, `is_payday`, `festival`, `festival_ramp`, `is_holiday`, `monsoon`, `is_operating` | Operating-day check; `iso_year` + `iso_week` group the demand forecast. |
| `district_travel`            | `depot_id`, `district`, `depot_to_district_freeflow_min`, `inter_stop_km`, `inter_stop_freeflow_min`                                      | The official trip-time inputs.                                          |
| `service_allowance`          | `brand`, `dock_type`, `service_allowance_min`                                                                                             | Handling time per stop.                                                 |
| `traffic_speed`              | `monsoon`, `speed_index`                                                                                                                  | Datathon only.                                                          |
| `road_condition`             | `date`, `district`, `disruption_index`                                                                                                    | Datathon only.                                                          |
| `vehicle_daily_availability` | `vehicle_id`, `date`, `available`, `status`, `reason`                                                                                     | Workshop, unavailable, broken down. Checked before any allocation.      |

---

## Planning — transient

| Table             | Key columns                                                                                                                                           | Notes                                                                                                                                                |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `planning_job`    | `job_id` PK, `planning_date`, `depot_id`, `status`, `requested_by`, `created_at`, `started_at`, `completed_at`, `error_message`                       | `QUEUED → RUNNING → COMPLETED \| FAILED`.                                                                                                            |
| `planning_result` | `result_id` PK, `job_id` FK, `order_id`, `decision`, `vehicle_id`, `trip_no`, `seq`, `eta`, `trip_minutes`, `explanation`, `constraint_results` JSONB | **A proposal, never an allocation.** `constraint_results` holds every rule verdict, passed and failed, which is what makes "why this plan" possible. |

A suggestion run touches nothing else. Discarding one requires no cleanup.

---

## Routes — authoritative after confirmation

| Table          | Key columns                                                                                                                                                                                                                               | Notes                                                                                                                 |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `route`        | `route_id` PK, `vehicle_id`, `depot_id`, `route_date`, `trip_no`, `brand`, `district`, `status`, `route_version`, `outbound_min`, `inter_stop_min`, `handling_min`, `total_trip_min`, `total_weight_kg`, `total_volume_m3`, `distance_km` | One vehicle, one trip, one brand, one district, one day. `route_version` is the optimistic lock.                      |
| `route_leg`    | `leg_id` PK, `route_id` FK, `order_id`, `seq`, `from_point`, `to_outlet`, `distance_km`, `planned_arrival`, `actual_arrival`, `service_time_min`, `status`                                                                                | One stop. `seq` starts at **0**; `from_point` is `DEPOT` for the first leg and the previous outlet thereafter.        |
| `allocation`   | `allocation_id` PK, `order_id`, `route_id` nullable, `decision`, `is_automatic`, `explanation`, `constraint_results` JSONB, `created_by`                                                                                                  | The decision audit. `route_id` is null for a deferral.                                                                |
| `deferral_log` | `deferral_id` PK, `order_id`, `reason_type`, `reason`, `constraint_code`, `decided_by`, `decided_at`, `deferred_to_date`                                                                                                                  | Every unserved order gets a row. This table answers _why was this outlet skipped_, and makes repeat skipping visible. |

Constraints worth enforcing in the database rather than only in code:

```sql
UNIQUE (vehicle_id, route_date, trip_no)   -- one route per vehicle per trip per day
CHECK  (trip_no IN (1, 2))                 -- at most two trips
UNIQUE (route_id, seq)                     -- no two stops at the same position
UNIQUE (route_id, order_id)                -- an order appears once in a route
```

An order appearing on two routes for one date is prevented by the validator, because it is a
cross-row rule the database cannot express cheaply. A partial unique index on the active
allocation per `(order_id, route_date)` is the belt-and-braces version.

> **Allocation invariant — decided (Routes/Allocation foundation).** History is allowed: an
> order can be `DEFERRED` and re-enter a later run, so `allocation` holds several rows per order
> across dates and is a decision audit. A plain `UNIQUE (order_id)` would therefore be wrong,
> and no partial index on `(order_id, route_date)` is added: `allocation` has no
> "active"/superseded column, a breakdown re-plan (DG-C) intentionally creates a second
> allocation for the same order and date, and `route_date` lives on `route` (a partial unique
> index cannot reference another table). The enforceable invariants are instead:
>
> - **An order appears at most once on a route** — `UNIQUE (route_id, order_id)` (in the schema).
> - **At most one route per vehicle/trip/day** — `UNIQUE (vehicle_id, route_date, trip_no)` (in
>   the schema).
> - **An order cannot be actively allocated twice on one operating day.** This cross-row rule is
>   enforced by confirmation in `internal/routes`: the orders in a confirmation are locked with
>   `SELECT ... FOR UPDATE` in a deterministic id order, then checked for an existing `ALLOCATED`
>   allocation on the route date inside the same transaction. The row lock makes concurrent
>   confirmations serialise instead of both passing; no speculative column was added.

---

## Execution

| Table                 | Key columns                                                                                                                                          | Notes                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `load_item`           | `load_item_id` PK, `route_id`, `order_item_id`, `ordered_qty`, `loaded_qty`, `damaged_qty`, `missing_qty`, `recorded_by`, `recorded_at`, `photo_ref` | Per order line, never per route. Invariant: `loaded + damaged + missing = ordered`, validated server-side against the database's own ordered quantity rather than the client's. `photo_ref` is a server-generated media key for an optional shortfall photo (mirrors `delivery_event.pod_photo`). Migration `00004` adds `UNIQUE (route_id, order_item_id)` so one load state exists per line per route: a retry updates it and concurrent submissions cannot duplicate it. A shortfall never mutates `order_item.quantity`. |
| `delivery_event`      | `event_id` PK, `leg_id`, `recorded_by`, `outcome`, POD metadata, `client_event_id` **UNIQUE**, `created_offline`, `client_created_at`, `synced_at`, `reason_code`   | The unique index on `client_event_id` is the entire idempotency mechanism. `client_created_at` is the device clock, preserved through sync. The delivery foundation writes these rows driver-only, in one transaction with the leg/order status update and `order_item_delivery` lines; `DELIVERED` requires `pod_receiver_name` (schema CHECK) plus a POD artefact. Migration `00007` adds nullable `reason_code` (CHECK against `DELIVERY_FAILURE_REASONS`); the service requires it on `FAILED`.                                                                                                                                |
| `order_item_delivery` | `id` PK, `delivery_event_id`, `order_item_id`, `delivered_qty`, `damaged_qty`, `short_qty`, `issue_note`                                             | Item-level outcome.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `receipt`             | `receipt_id` PK, `order_id`, `status`, `received_at`, `received_by`, `issue_type`, `notes`, `proof`                                                  | Store manager's confirmation or issue report.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `vehicle_fuel_usage`  | `id` PK, `vehicle_id`, `week_start_date`, `distance_km`, `estimated_fuel_l`                                                                          | Weekly quota accounting by ISO week.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `notification`        | `id` PK, target user or outlet, `type`, `title`, `message`, `reference`, `read_at`                                                                   | ETA, deferral, shortfall, re-plan. Migration `00005` adds `UNIQUE (user_id, type, reference)` so a retried business event cannot create a duplicate notification; creation uses `ON CONFLICT DO NOTHING`. Addressed to a resolved recipient (or an outlet scope); read state is `read_at`.                                                                                                                                                                                                                                   |
| `audit_log`           | `id` PK, `actor`, `action`, `entity_type`, `entity_id`, `before_json`, `after_json`, `timestamp`                                                     | Append-only. Never updated or deleted. Written by `internal/audit` inside the business transaction it describes (so a rolled-back mutation leaves no audit row). Scope/result/detail are stored as a small envelope in `after_json`; `before_json` stays NULL (no before-state diff in this foundation). No secrets or request bodies are stored.                                                                                                                                                                            |

---

## The constraint catalogue

Defined once in `libs/shared-types/src/lib/constraints.ts` and mirrored in
`apps/api/internal/domain/constraints.go`. `E-0x` is the rule-panel group from the Day 5
design (screen D-03) — kept because judges compare the two.

| Code                       | Rule | Check                                                     |
| -------------------------- | ---- | --------------------------------------------------------- |
| `WEIGHT_CAPACITY_EXCEEDED` | E-01 | trip weight + order weight ≤ `weight_cap_kg`              |
| `VOLUME_CAPACITY_EXCEEDED` | E-01 | trip volume + order volume ≤ `volume_cap_m3`              |
| `ORDER_SPLIT_FORBIDDEN`    | E-01 | an order goes to exactly one vehicle and trip             |
| `DUPLICATE_ASSIGNMENT`     | E-01 | an order is not already allocated elsewhere               |
| `REEFER_REQUIRED`          | E-02 | chilled or frozen requires `temp = reefer`                |
| `VAN_ONLY_ACCESS`          | E-03 | a `van_only` outlet requires `type = van`                 |
| `DEPOT_MISMATCH`           | E-04 | `vehicle.depot_id = outlet.depot_id`                      |
| `TRIP_LIMIT_EXCEEDED`      | E-05 | at most 2 trips per vehicle per day                       |
| `TRIP_NUMBER_INVALID`      | E-05 | `trip_no ∈ {1, 2}`                                        |
| `FRESH_TIME_BUDGET`        | E-05 | Fresh minutes for the vehicle ≤ 270                       |
| `STYLE_TECH_TIME_BUDGET`   | E-05 | Style **+** Tech minutes ≤ 480 combined                   |
| `BRAND_DISTRICT_MIX`       | E-05 | one brand and one district per vehicle + trip             |
| `VEHICLE_UNAVAILABLE`      | E-05 | not in workshop or otherwise unavailable                  |
| `NON_OPERATING_DAY`        | E-05 | the planning date is an operating day                     |
| `DELIVERY_WINDOW_MISSED`   | E-06 | planned arrival ≤ `window_close_time`                     |
| `MALL_WINDOW_MISSED`       | E-06 | arrival inside the mall access window                     |
| `FUEL_QUOTA_EXCEEDED`      | E-07 | projected weekly fuel ≤ `weekly_fuel_quota_l`             |
| `FUEL_EFFICIENCY_INVALID`  | E-07 | `vehicle.km_per_l` is positive, so fuel use is calculable |

Boundary behaviour, tested on both sides: **exactly at a limit is accepted; one unit over is
rejected.** 270 minutes passes, 271 fails. Arrival at the closing minute is on time; one
minute later is late.

---

## Formulas

### Trip time — the official formula, not a routing API

```
outbound_min   = district_travel.depot_to_district_freeflow_min      (once per trip)
inter_stop_min = district_travel.inter_stop_freeflow_min × max(n − 1, 0)
handling_min   = Σ service_allowance(brand, outlet.dock_type)
trip_minutes   = outbound_min + inter_stop_min + handling_min
```

The return journey to the depot is **not** counted. The booklet's worked example — a
three-order Fresh trip to Gampaha — is pinned as a test in both languages:

```
37 + (9 × 2) + (15 + 15 + 16) = 101 minutes
```

### Delivery windows

```
if arrival < window_open:  waiting = window_open − arrival;  service_start = window_open
else:                      waiting = 0;                      service_start = arrival
late = arrival > window_close
```

Early arrival waits. Lateness is arrival **after the window closes** — the same definition
the Datathon uses for Task 1, so the operational app and the model agree on what "late" means.

### Fuel

```
estimated_fuel_l   = route_distance_km / vehicle.km_per_l
weekly_used_l      = Σ estimated_fuel_l for the vehicle in the ISO week
fuel_ok            = weekly_used_l ≤ vehicle.weekly_fuel_quota_l
```

---

## Order lifecycle

```
PLACED → CONFIRMED → ALLOCATED → LOADED → IN_TRANSIT → DELIVERED → RECEIVED
   │          │           │          │
   └──────────┴───────────┴──────────┴──────→ DEFERRED → a later planning run

IN_TRANSIT may also produce FAILED.
DELAYED is a delivery OUTCOME, not an order state — a delayed stop is still expected.
```

Legal transitions are enumerated in `ORDER_STATUS_TRANSITIONS`
(`libs/shared-types/src/lib/domain.ts`). The client uses them to disable impossible actions;
the server enforces them.
