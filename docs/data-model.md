# Data model

PostgreSQL is the authoritative store of operational facts. Required Hackathon deliverable.

**The rule that shapes everything below:** derived values — remaining capacity, remaining
minutes, remaining fuel — are **never** persisted as authoritative. They are computed from a
route and its current orders on read. A stored copy can go stale after a route edit, and a
stale number that looks authoritative is worse than no number.

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

| Table      | Key columns                                                                                                                                      | Notes                                                                                                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `depot`    | `depot_id` PK, `code`, `name`, `is_active`                                                                                                       | Peliyagoda and Kandy.                                                                                                                                                     |
| `outlet`   | `outlet_id` PK, `code`, `brand`, `district`, `depot_id` FK, `dock_type`, `van_only`, `window_open_time`, `window_close_time`, `mall_window`      | OUT001–OUT120. `depot_id` decides which vehicles may serve it; `van_only` excludes trucks; `dock_type` drives the service allowance lookup.                               |
| `vehicle`  | `vehicle_id` PK, `registration_no`, `type`, `temp`, `weight_cap_kg`, `volume_cap_m3`, `km_per_l`, `weekly_fuel_quota_l`, `depot_id` FK, `status` | VEH001–VEH060: 12 reefer trucks, 40 dry-box trucks, 8 vans (4 refrigerated).                                                                                              |
| `item`     | `item_id` PK, `sku` UNIQUE, `brand`, `unit_weight_kg`, `unit_volume_m3`, `temperature_requirement`                                               | Real SKUs, not aggregates.                                                                                                                                                |
| `app_user` | `user_id` PK, `email` UNIQUE, `password_hash`, `role`, `depot_id` nullable, `outlet_id` nullable                                                 | Nullable scope columns: a dispatcher has a depot and no outlet, a store manager the reverse. Enforced server-side on every query, never trusted from a token claim alone. |

---

## Orders

| Table            | Key columns                                                                                                                                                                                               | Notes                                                                                                                                                                                                          |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `customer_order` | `order_id` PK, `order_number` UNIQUE, `outlet_id` FK, `brand`, `order_date`, `requested_delivery_date`, `total_units`, `total_weight_kg`, `total_volume_m3`, `temp_requirement`, `status`, `after_cutoff` | Header. Totals are computed from lines on write, not supplied by the client.                                                                                                                                   |
| `order_item`     | `order_item_id` PK, `order_id` FK, `item_id` FK, `quantity`, `unit_weight_kg_snapshot`, `unit_volume_m3_snapshot`, `total_weight_kg`, `total_volume_m3`                                                   | The snapshot columns are the point: a catalogue correction next week must not retroactively change this order's weight, because that would retroactively invalidate a plan that was feasible when it was made. |

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

---

## Execution

| Table                 | Key columns                                                                                                                                        | Notes                                                                                                                                                                           |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `load_item`           | `load_item_id` PK, `route_id`, `order_item_id`, `ordered_qty`, `loaded_qty`, `damaged_qty`, `missing_qty`, `recorded_by`, `recorded_at`            | Per order line, never per route. Invariant: `loaded + damaged + missing = ordered`, validated server-side against the database's own ordered quantity rather than the client's. |
| `delivery_event`      | `event_id` PK, `leg_id`, `recorded_by`, `outcome`, POD metadata, `client_event_id` **UNIQUE**, `created_offline`, `client_created_at`, `synced_at` | The unique index on `client_event_id` is the entire idempotency mechanism. `client_created_at` is the device clock, preserved through sync.                                     |
| `order_item_delivery` | `id` PK, `delivery_event_id`, `order_item_id`, `delivered_qty`, `damaged_qty`, `short_qty`, `issue_note`                                           | Item-level outcome.                                                                                                                                                             |
| `receipt`             | `receipt_id` PK, `order_id`, `status`, `received_at`, `received_by`, `issue_type`, `notes`, `proof`                                                | Store manager's confirmation or issue report.                                                                                                                                   |
| `vehicle_fuel_usage`  | `id` PK, `vehicle_id`, `week_start_date`, `distance_km`, `estimated_fuel_l`                                                                        | Weekly quota accounting by ISO week.                                                                                                                                            |
| `notification`        | `id` PK, target user or outlet, `type`, `title`, `message`, `reference`, `read_at`                                                                 | ETA, deferral, shortfall, re-plan.                                                                                                                                              |
| `audit_log`           | `id` PK, `actor`, `action`, `entity_type`, `entity_id`, `before_json`, `after_json`, `timestamp`                                                   | Append-only. Never updated or deleted.                                                                                                                                          |

---

## The constraint catalogue

Defined once in `libs/shared-types/src/lib/constraints.ts` and mirrored in
`apps/api/internal/domain/constraints.go`. `E-0x` is the rule-panel group from the Day 5
design (screen D-03) — kept because judges compare the two.

| Code                       | Rule | Check                                         |
| -------------------------- | ---- | --------------------------------------------- |
| `WEIGHT_CAPACITY_EXCEEDED` | E-01 | trip weight + order weight ≤ `weight_cap_kg`  |
| `VOLUME_CAPACITY_EXCEEDED` | E-01 | trip volume + order volume ≤ `volume_cap_m3`  |
| `ORDER_SPLIT_FORBIDDEN`    | E-01 | an order goes to exactly one vehicle and trip |
| `DUPLICATE_ASSIGNMENT`     | E-01 | an order is not already allocated elsewhere   |
| `REEFER_REQUIRED`          | E-02 | chilled or frozen requires `temp = reefer`    |
| `VAN_ONLY_ACCESS`          | E-03 | a `van_only` outlet requires `type = van`     |
| `DEPOT_MISMATCH`           | E-04 | `vehicle.depot_id = outlet.depot_id`          |
| `TRIP_LIMIT_EXCEEDED`      | E-05 | at most 2 trips per vehicle per day           |
| `TRIP_NUMBER_INVALID`      | E-05 | `trip_no ∈ {1, 2}`                            |
| `FRESH_TIME_BUDGET`        | E-05 | Fresh minutes for the vehicle ≤ 270           |
| `STYLE_TECH_TIME_BUDGET`   | E-05 | Style **+** Tech minutes ≤ 480 combined       |
| `BRAND_DISTRICT_MIX`       | E-05 | one brand and one district per vehicle + trip |
| `VEHICLE_UNAVAILABLE`      | E-05 | not in workshop or otherwise unavailable      |
| `NON_OPERATING_DAY`        | E-05 | the planning date is an operating day         |
| `DELIVERY_WINDOW_MISSED`   | E-06 | planned arrival ≤ `window_close_time`         |
| `MALL_WINDOW_MISSED`       | E-06 | arrival inside the mall access window         |
| `FUEL_QUOTA_EXCEEDED`      | E-07 | projected weekly fuel ≤ `weekly_fuel_quota_l` |

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
