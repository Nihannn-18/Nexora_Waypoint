-- +goose Up
-- Schema for the Waypoint Delivery Planning System.
-- Mirrors docs/data-model.md exactly: tables, keys, constraints, indexes.
-- Derived values (remaining capacity/minutes/fuel) are never stored here.
--
-- Enum-compatible columns are TEXT with CHECK constraints rather than native
-- PostgreSQL enums, so a new value is a forward migration, not a type rewrite.

-- --------------------------------------------------------------------------
-- Master data
-- --------------------------------------------------------------------------

CREATE TABLE depot (
    depot_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE outlet (
    outlet_id         TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    brand             TEXT NOT NULL CHECK (brand IN ('FRESH', 'STYLE', 'TECH')),
    district          TEXT NOT NULL,
    depot_id          UUID NOT NULL REFERENCES depot (depot_id),
    dock_type         TEXT NOT NULL CHECK (dock_type IN ('REAR_DOCK', 'STREET', 'MALL_BAY')),
    parking_constraint TEXT NOT NULL CHECK (parking_constraint IN ('NORMAL', 'VAN_ONLY', 'MALL_DOCK')),
    mall_window_open  TIME,
    mall_window_close TIME,
    window_open_time  TIME NOT NULL,
    window_close_time TIME NOT NULL
);
CREATE INDEX outlet_district_idx ON outlet (district);
CREATE INDEX outlet_depot_idx ON outlet (depot_id);
CREATE INDEX outlet_brand_idx ON outlet (brand);

CREATE TABLE vehicle (
    vehicle_id          TEXT PRIMARY KEY,
    type                TEXT NOT NULL CHECK (type IN ('TRUCK', 'VAN')),
    temp                TEXT NOT NULL CHECK (temp IN ('REEFER', 'AMBIENT')),
    weight_cap_kg       NUMERIC(10, 2) NOT NULL CHECK (weight_cap_kg > 0),
    volume_cap_m3       NUMERIC(10, 2) NOT NULL CHECK (volume_cap_m3 > 0),
    fuel_type           TEXT NOT NULL,
    km_per_l            NUMERIC(6, 2) NOT NULL CHECK (km_per_l > 0),
    weekly_fuel_quota_l NUMERIC(8, 2) NOT NULL CHECK (weekly_fuel_quota_l >= 0),
    depot_id            UUID NOT NULL REFERENCES depot (depot_id)
);
CREATE INDEX vehicle_depot_idx ON vehicle (depot_id);

CREATE TABLE item (
    item_id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sku                     TEXT NOT NULL UNIQUE,
    name                    TEXT NOT NULL,
    brand                   TEXT NOT NULL CHECK (brand IN ('FRESH', 'STYLE', 'TECH')),
    category                TEXT,
    unit_weight_kg          NUMERIC(10, 3) NOT NULL CHECK (unit_weight_kg >= 0),
    unit_volume_m3          NUMERIC(10, 4) NOT NULL CHECK (unit_volume_m3 >= 0),
    temperature_requirement TEXT NOT NULL CHECK (temperature_requirement IN ('AMBIENT', 'CHILLED', 'FROZEN'))
);

CREATE TABLE app_user (
    user_id    TEXT PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    role       TEXT NOT NULL CHECK (role IN ('STORE_MANAGER', 'DISPATCHER', 'LOADER', 'DRIVER')),
    depot_id   UUID REFERENCES depot (depot_id),
    outlet_id  TEXT REFERENCES outlet (outlet_id),
    is_active  BOOLEAN NOT NULL DEFAULT TRUE
);

-- --------------------------------------------------------------------------
-- Orders
-- --------------------------------------------------------------------------

CREATE TABLE customer_order (
    order_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number            TEXT NOT NULL UNIQUE,
    outlet_id               TEXT NOT NULL REFERENCES outlet (outlet_id),
    brand                   TEXT NOT NULL CHECK (brand IN ('FRESH', 'STYLE', 'TECH')),
    order_date              DATE NOT NULL,
    requested_delivery_date DATE NOT NULL,
    total_units             INTEGER NOT NULL DEFAULT 0 CHECK (total_units >= 0),
    total_weight_kg         NUMERIC(12, 3) NOT NULL DEFAULT 0 CHECK (total_weight_kg >= 0),
    total_volume_m3         NUMERIC(12, 4) NOT NULL DEFAULT 0 CHECK (total_volume_m3 >= 0),
    temp_requirement        TEXT NOT NULL CHECK (temp_requirement IN ('AMBIENT', 'CHILLED', 'FROZEN')),
    status                  TEXT NOT NULL CHECK (status IN (
        'PLACED', 'CONFIRMED', 'ALLOCATED', 'LOADED', 'IN_TRANSIT',
        'DELIVERED', 'RECEIVED', 'DEFERRED', 'FAILED'
    )),
    after_cutoff            BOOLEAN NOT NULL DEFAULT FALSE,
    notes                   TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX customer_order_outlet_idx ON customer_order (outlet_id);
CREATE INDEX customer_order_delivery_date_idx ON customer_order (requested_delivery_date);
CREATE INDEX customer_order_status_idx ON customer_order (status);

CREATE TABLE order_item (
    order_item_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id               UUID NOT NULL REFERENCES customer_order (order_id) ON DELETE CASCADE,
    item_id                UUID NOT NULL REFERENCES item (item_id),
    quantity               INTEGER NOT NULL CHECK (quantity > 0),
    unit_weight_kg_snapshot NUMERIC(10, 3) NOT NULL CHECK (unit_weight_kg_snapshot >= 0),
    unit_volume_m3_snapshot NUMERIC(10, 4) NOT NULL CHECK (unit_volume_m3_snapshot >= 0),
    total_weight_kg        NUMERIC(12, 3) NOT NULL CHECK (total_weight_kg >= 0),
    total_volume_m3        NUMERIC(12, 4) NOT NULL CHECK (total_volume_m3 >= 0)
);
CREATE INDEX order_item_order_idx ON order_item (order_id);

-- --------------------------------------------------------------------------
-- Reference data, seeded from the supplied CSVs
-- --------------------------------------------------------------------------

CREATE TABLE calendar_day (
    date           DATE PRIMARY KEY,
    dow            SMALLINT NOT NULL CHECK (dow BETWEEN 0 AND 6),
    dow_name       TEXT NOT NULL,
    is_weekend     BOOLEAN NOT NULL,
    iso_year       INTEGER NOT NULL,
    iso_week       INTEGER NOT NULL CHECK (iso_week BETWEEN 1 AND 53),
    is_payday      BOOLEAN NOT NULL,
    festival       TEXT,
    festival_ramp  NUMERIC(4, 2),
    is_holiday     BOOLEAN NOT NULL,
    monsoon        BOOLEAN NOT NULL,
    is_operating   BOOLEAN NOT NULL
);
CREATE INDEX calendar_day_iso_week_idx ON calendar_day (iso_year, iso_week);

CREATE TABLE district_travel (
    district                     TEXT NOT NULL,
    depot_id                     UUID NOT NULL REFERENCES depot (depot_id),
    road_class                   TEXT NOT NULL,
    free_flow_kmh                NUMERIC(6, 2) NOT NULL,
    depot_to_district_km         NUMERIC(8, 2) NOT NULL,
    depot_to_district_freeflow_min NUMERIC(8, 2) NOT NULL,
    inter_stop_km                NUMERIC(8, 2) NOT NULL,
    inter_stop_freeflow_min      NUMERIC(8, 2) NOT NULL,
    PRIMARY KEY (district, depot_id)
);

CREATE TABLE service_allowance (
    brand                TEXT NOT NULL CHECK (brand IN ('FRESH', 'STYLE', 'TECH')),
    dock_type            TEXT NOT NULL CHECK (dock_type IN ('REAR_DOCK', 'STREET', 'MALL_BAY')),
    service_allowance_min NUMERIC(6, 2) NOT NULL CHECK (service_allowance_min >= 0),
    PRIMARY KEY (brand, dock_type)
);

-- Reference tables for the Datathon. Tables exist so the schema is complete,
-- but NO rows are seeded: these are not operational Hackathon inputs.
CREATE TABLE traffic_speed (
    district    TEXT NOT NULL,
    hour        SMALLINT NOT NULL CHECK (hour BETWEEN 0 AND 23),
    monsoon     BOOLEAN NOT NULL,
    speed_index NUMERIC(6, 2) NOT NULL,
    PRIMARY KEY (district, hour, monsoon)
);

CREATE TABLE road_condition (
    district         TEXT NOT NULL,
    date             DATE NOT NULL,
    disruption_index NUMERIC(6, 2) NOT NULL,
    PRIMARY KEY (district, date)
);

CREATE TABLE vehicle_daily_availability (
    vehicle_id TEXT NOT NULL REFERENCES vehicle (vehicle_id),
    date       DATE NOT NULL,
    available  BOOLEAN NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('AVAILABLE', 'IN_WORKSHOP', 'BREAKDOWN')),
    reason     TEXT,
    PRIMARY KEY (vehicle_id, date)
);

-- --------------------------------------------------------------------------
-- Planning (transient; a proposal, never an allocation)
-- --------------------------------------------------------------------------

CREATE TABLE planning_job (
    job_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    planning_date DATE NOT NULL,
    depot_id      UUID NOT NULL REFERENCES depot (depot_id),
    status        TEXT NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'COMPLETED', 'FAILED')),
    requested_by  TEXT REFERENCES app_user (user_id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    error_message TEXT
);
CREATE INDEX planning_job_status_idx ON planning_job (status);

CREATE TABLE planning_result (
    result_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID NOT NULL REFERENCES planning_job (job_id) ON DELETE CASCADE,
    order_id          UUID NOT NULL REFERENCES customer_order (order_id),
    decision          TEXT NOT NULL CHECK (decision IN ('ALLOCATED', 'DEFERRED')),
    vehicle_id        TEXT REFERENCES vehicle (vehicle_id),
    trip_no           SMALLINT CHECK (trip_no IN (1, 2)),
    seq               INTEGER CHECK (seq >= 0),
    eta               TIMESTAMPTZ,
    trip_minutes      NUMERIC(8, 2),
    explanation       TEXT,
    constraint_results JSONB
);
CREATE INDEX planning_result_job_idx ON planning_result (job_id);

-- --------------------------------------------------------------------------
-- Routes (authoritative after confirmation)
-- --------------------------------------------------------------------------

CREATE TABLE route (
    route_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id       TEXT NOT NULL REFERENCES vehicle (vehicle_id),
    depot_id         UUID NOT NULL REFERENCES depot (depot_id),
    route_date       DATE NOT NULL,
    trip_no          SMALLINT NOT NULL CHECK (trip_no IN (1, 2)),
    brand            TEXT NOT NULL CHECK (brand IN ('FRESH', 'STYLE', 'TECH')),
    district         TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN (
        'DRAFT', 'CONFIRMED', 'DISPATCHED', 'IN_TRANSIT', 'COMPLETED', 'CANCELLED'
    )),
    route_version    INTEGER NOT NULL DEFAULT 1,
    outbound_min     NUMERIC(8, 2),
    inter_stop_min   NUMERIC(8, 2),
    handling_min     NUMERIC(8, 2),
    total_trip_min   NUMERIC(8, 2),
    total_weight_kg  NUMERIC(12, 3),
    total_volume_m3  NUMERIC(12, 4),
    distance_km      NUMERIC(10, 2),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (vehicle_id, route_date, trip_no)
);
CREATE INDEX route_date_idx ON route (route_date);

CREATE TABLE route_leg (
    leg_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id         UUID NOT NULL REFERENCES route (route_id) ON DELETE CASCADE,
    order_id         UUID NOT NULL REFERENCES customer_order (order_id),
    seq              INTEGER NOT NULL CHECK (seq >= 0),
    from_point       TEXT NOT NULL,
    to_outlet        TEXT NOT NULL REFERENCES outlet (outlet_id),
    distance_km      NUMERIC(10, 2),
    planned_arrival  TIMESTAMPTZ,
    actual_arrival   TIMESTAMPTZ,
    service_time_min NUMERIC(8, 2),
    status           TEXT NOT NULL CHECK (status IN (
        'PENDING', 'IN_TRANSIT', 'DELIVERED', 'FAILED', 'DELAYED', 'SKIPPED'
    )),
    UNIQUE (route_id, seq),
    UNIQUE (route_id, order_id)
);

CREATE TABLE allocation (
    allocation_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          UUID NOT NULL REFERENCES customer_order (order_id),
    route_id          UUID REFERENCES route (route_id),
    decision          TEXT NOT NULL CHECK (decision IN ('ALLOCATED', 'DEFERRED')),
    is_automatic      BOOLEAN NOT NULL DEFAULT FALSE,
    explanation       TEXT,
    constraint_results JSONB,
    created_by        TEXT REFERENCES app_user (user_id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX allocation_order_idx ON allocation (order_id);

CREATE TABLE deferral_log (
    deferral_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         UUID NOT NULL REFERENCES customer_order (order_id),
    reason_type      TEXT NOT NULL,
    reason           TEXT NOT NULL,
    constraint_code  TEXT,
    decided_by       TEXT REFERENCES app_user (user_id),
    decided_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deferred_to_date DATE
);
CREATE INDEX deferral_log_order_idx ON deferral_log (order_id);
CREATE INDEX deferral_log_decided_at_idx ON deferral_log (decided_at);

-- --------------------------------------------------------------------------
-- Execution
-- --------------------------------------------------------------------------

CREATE TABLE load_item (
    load_item_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id      UUID NOT NULL REFERENCES route (route_id) ON DELETE CASCADE,
    order_item_id UUID NOT NULL REFERENCES order_item (order_item_id),
    ordered_qty   INTEGER NOT NULL CHECK (ordered_qty >= 0),
    loaded_qty    INTEGER NOT NULL DEFAULT 0 CHECK (loaded_qty >= 0),
    damaged_qty   INTEGER NOT NULL DEFAULT 0 CHECK (damaged_qty >= 0),
    missing_qty   INTEGER NOT NULL DEFAULT 0 CHECK (missing_qty >= 0),
    recorded_by   TEXT REFERENCES app_user (user_id),
    recorded_at   TIMESTAMPTZ,
    CHECK (loaded_qty + damaged_qty + missing_qty <= ordered_qty)
);
CREATE INDEX load_item_route_idx ON load_item (route_id);

CREATE TABLE delivery_event (
    event_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    leg_id            UUID NOT NULL REFERENCES route_leg (leg_id),
    recorded_by       TEXT REFERENCES app_user (user_id),
    outcome           TEXT NOT NULL CHECK (outcome IN ('DELIVERED', 'FAILED', 'DELAYED')),
    pod_receiver_name TEXT,
    pod_signature     TEXT,
    pod_photo         TEXT,
    note              TEXT,
    client_event_id   UUID NOT NULL UNIQUE,
    created_offline   BOOLEAN NOT NULL DEFAULT FALSE,
    client_created_at TIMESTAMPTZ NOT NULL,
    synced_at         TIMESTAMPTZ,
    CHECK (outcome <> 'DELIVERED' OR pod_receiver_name IS NOT NULL)
);
CREATE INDEX delivery_event_leg_idx ON delivery_event (leg_id);

CREATE TABLE order_item_delivery (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_event_id UUID NOT NULL REFERENCES delivery_event (event_id) ON DELETE CASCADE,
    order_item_id     UUID NOT NULL REFERENCES order_item (order_item_id),
    delivered_qty     INTEGER NOT NULL DEFAULT 0 CHECK (delivered_qty >= 0),
    damaged_qty       INTEGER NOT NULL DEFAULT 0 CHECK (damaged_qty >= 0),
    short_qty         INTEGER NOT NULL DEFAULT 0 CHECK (short_qty >= 0),
    issue_note        TEXT
);
CREATE INDEX order_item_delivery_event_idx ON order_item_delivery (delivery_event_id);

CREATE TABLE receipt (
    receipt_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID NOT NULL REFERENCES customer_order (order_id),
    status      TEXT NOT NULL CHECK (status IN ('PENDING', 'CONFIRMED', 'ISSUE')),
    received_at TIMESTAMPTZ,
    received_by TEXT REFERENCES app_user (user_id),
    issue_type  TEXT,
    notes       TEXT,
    proof       TEXT
);
CREATE INDEX receipt_order_idx ON receipt (order_id);

CREATE TABLE vehicle_fuel_usage (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id      TEXT NOT NULL REFERENCES vehicle (vehicle_id),
    week_start_date DATE NOT NULL,
    distance_km     NUMERIC(10, 2) NOT NULL DEFAULT 0,
    estimated_fuel_l NUMERIC(10, 2) NOT NULL DEFAULT 0,
    UNIQUE (vehicle_id, week_start_date)
);

CREATE TABLE notification (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT REFERENCES app_user (user_id),
    outlet_id  TEXT REFERENCES outlet (outlet_id),
    type       TEXT NOT NULL,
    title      TEXT NOT NULL,
    message    TEXT NOT NULL,
    reference  TEXT,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX notification_user_idx ON notification (user_id);

CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor       TEXT,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT,
    before_json JSONB,
    after_json  JSONB,
    "timestamp" TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS notification;
DROP TABLE IF EXISTS vehicle_fuel_usage;
DROP TABLE IF EXISTS receipt;
DROP TABLE IF EXISTS order_item_delivery;
DROP TABLE IF EXISTS delivery_event;
DROP TABLE IF EXISTS load_item;
DROP TABLE IF EXISTS deferral_log;
DROP TABLE IF EXISTS allocation;
DROP TABLE IF EXISTS route_leg;
DROP TABLE IF EXISTS route;
DROP TABLE IF EXISTS planning_result;
DROP TABLE IF EXISTS planning_job;
DROP TABLE IF EXISTS vehicle_daily_availability;
DROP TABLE IF EXISTS road_condition;
DROP TABLE IF EXISTS traffic_speed;
DROP TABLE IF EXISTS service_allowance;
DROP TABLE IF EXISTS district_travel;
DROP TABLE IF EXISTS calendar_day;
DROP TABLE IF EXISTS order_item;
DROP TABLE IF EXISTS customer_order;
DROP TABLE IF EXISTS app_user;
DROP TABLE IF EXISTS item;
DROP TABLE IF EXISTS vehicle;
DROP TABLE IF EXISTS outlet;
DROP TABLE IF EXISTS depot;
