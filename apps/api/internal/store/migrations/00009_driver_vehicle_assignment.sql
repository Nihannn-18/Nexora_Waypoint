-- +goose Up
-- Dispatcher operational assignment: which driver is on which vehicle for an
-- operating date. This is the missing link between a driver and a run.
--
-- The schema previously linked no driver to a vehicle: route carries a vehicle
-- but not a driver, so the driver cockpit could only resolve the depot's routes
-- and ask the driver to pick one. A date-based assignment is the smallest model
-- that answers "which run is mine?" without pretending a permanent one-driver-
-- per-vehicle relationship the domain does not have.
--
-- Two uniqueness rules make the assignment unambiguous for a date:
--   * one driver drives at most one vehicle per operating date;
--   * one vehicle has at most one driver per operating date.
-- Together they prevent a driver being double-booked and a vehicle being handed
-- two drivers. Both are per date, so history accumulates rather than overwriting.
--
-- depot_id is denormalised from vehicle so a planner/loader can scope an
-- assignment by depot without a join; it is validated to match the driver's and
-- the vehicle's depot before the row is written.
CREATE TABLE driver_vehicle_assignment (
    assignment_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id       TEXT NOT NULL REFERENCES app_user (user_id) ON DELETE CASCADE,
    vehicle_id      TEXT NOT NULL REFERENCES vehicle (vehicle_id),
    assignment_date DATE NOT NULL,
    depot_id        UUID NOT NULL REFERENCES depot (depot_id),
    assigned_by     TEXT REFERENCES app_user (user_id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (driver_id, assignment_date),
    UNIQUE (vehicle_id, assignment_date)
);
CREATE INDEX driver_vehicle_assignment_date_idx ON driver_vehicle_assignment (assignment_date);
CREATE INDEX driver_vehicle_assignment_depot_idx ON driver_vehicle_assignment (depot_id);

-- +goose Down
DROP TABLE IF EXISTS driver_vehicle_assignment;
