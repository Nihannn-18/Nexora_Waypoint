package planning

import (
	"context"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/store"
)

// TestPlanningIntegration exercises the loader and repository against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves the
// hand-written SQL matches the schema end to end: build reference + a confirmed
// order, load the input, run the engine, persist and read back the proposals.
func TestPlanningIntegration(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Minimal reference world in its own depot so the run is isolated.
	var depotID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('PLANDEPOT', 'Plan Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('OUTPLAN1', 'Plan Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '07:30')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO district_travel (district, depot_id, road_class, free_flow_kmh, depot_to_district_km, depot_to_district_freeflow_min, inter_stop_km, inter_stop_freeflow_min)
		VALUES ('Colombo', $1, 'urban', 30, 12, 24, 4, 8)
		ON CONFLICT (district, depot_id) DO UPDATE SET depot_to_district_freeflow_min = 24`, depotID); err != nil {
		t.Fatalf("travel: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO service_allowance (brand, dock_type, service_allowance_min)
		VALUES ('FRESH', 'STREET', 16) ON CONFLICT (brand, dock_type) DO NOTHING`); err != nil {
		t.Fatalf("allowance: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ('VEHPLAN1', 'TRUCK', 'AMBIENT', 5000, 20, 'diesel', 5, 400, $1)
		ON CONFLICT (vehicle_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO calendar_day (date, dow, dow_name, is_weekend, iso_year, iso_week, is_payday, is_holiday, monsoon, is_operating)
		VALUES ('2026-09-26', 5, 'Sat', TRUE, 2026, 39, FALSE, FALSE, FALSE, TRUE)
		ON CONFLICT (date) DO UPDATE SET is_operating = TRUE`); err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO customer_order (order_number, outlet_id, brand, order_date, requested_delivery_date, total_units, total_weight_kg, total_volume_m3, temp_requirement, status)
		VALUES ('PLAN-ORD-1', 'OUTPLAN1', 'FRESH', '2026-09-25', '2026-09-26', 10, 100, 1.5, 'AMBIENT', 'CONFIRMED')
		ON CONFLICT (order_number) DO NOTHING`); err != nil {
		t.Fatalf("order: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM customer_order WHERE order_number = 'PLAN-ORD-1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM vehicle WHERE vehicle_id = 'VEHPLAN1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM outlet WHERE outlet_id = 'OUTPLAN1'`)
	})

	loader := NewPGLoader(db.Pool())
	in, err := loader.LoadInput(ctx, mustDate(2026, 9, 26), depotID)
	if err != nil {
		t.Fatalf("load input: %v", err)
	}
	if len(in.Orders) != 1 {
		t.Fatalf("loaded %d orders, want 1", len(in.Orders))
	}
	if len(in.Vehicles) != 1 || in.Vehicles[0].VehicleID != "VEHPLAN1" {
		t.Fatalf("loaded vehicles = %+v", in.Vehicles)
	}
	if !in.Calendar.IsOperating {
		t.Fatal("calendar should be operating")
	}
	// The authoritative weekly quota must be loaded from the vehicle row.
	if got := in.FuelQuotaL["VEHPLAN1"]; got != 400 {
		t.Fatalf("FuelQuotaL[VEHPLAN1] = %v, want 400 (vehicle.weekly_fuel_quota_l)", got)
	}

	// The weekly ledger must be read for the exact planning week, and a row from
	// another week must be ignored.
	weekStart := mustDate(2026, 9, 21) // Monday of the ISO week containing 2026-09-26
	priorWeek := mustDate(2026, 9, 14)
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO vehicle_fuel_usage (vehicle_id, week_start_date, distance_km, estimated_fuel_l)
		VALUES ('VEHPLAN1', $1, 100, 25), ('VEHPLAN1', $2, 50, 99)
		ON CONFLICT (vehicle_id, week_start_date) DO UPDATE SET estimated_fuel_l = EXCLUDED.estimated_fuel_l`,
		weekStart, priorWeek); err != nil {
		t.Fatalf("seed fuel usage: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM vehicle_fuel_usage WHERE vehicle_id = 'VEHPLAN1'`)
	})
	in2, err := loader.LoadInput(ctx, mustDate(2026, 9, 26), depotID)
	if err != nil {
		t.Fatalf("reload input: %v", err)
	}
	if got := in2.FuelUsedL["VEHPLAN1"]; got != 25 {
		t.Fatalf("FuelUsedL[VEHPLAN1] = %v, want 25 (this week only, not 99 from the prior week)", got)
	}

	res := New().Plan(in2)
	if len(res.Trips) != 1 {
		t.Fatalf("expected one trip, got %d (deferred %+v)", len(res.Trips), res.Deferred)
	}

	repo := NewPGRepository(db.Pool())
	job, err := repo.CreateJob(ctx, Job{PlanningDate: mustDate(2026, 9, 26), DepotID: depotID, RequestedBy: ""})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.UpdateJobStatus(ctx, job.JobID, JobRunning, ""); err != nil {
		t.Fatalf("running: %v", err)
	}
	if err := repo.SaveProposals(ctx, job.JobID, proposalsFor(res)); err != nil {
		t.Fatalf("save proposals: %v", err)
	}
	if _, err := repo.UpdateJobStatus(ctx, job.JobID, JobCompleted, ""); err != nil {
		t.Fatalf("complete: %v", err)
	}

	stored, err := repo.GetJob(ctx, job.JobID)
	if err != nil || stored.Status != JobCompleted {
		t.Fatalf("job round-trip = %+v, %v", stored, err)
	}
	props, err := repo.LoadProposals(ctx, job.JobID)
	if err != nil {
		t.Fatalf("load proposals: %v", err)
	}
	if len(props) != 1 || props[0].Decision != "SERVE" || props[0].VehicleID != "VEHPLAN1" {
		t.Fatalf("proposals = %+v", props)
	}
	// Re-saving must be idempotent (replaces, no duplicates).
	if err := repo.SaveProposals(ctx, job.JobID, proposalsFor(res)); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	props2, _ := repo.LoadProposals(ctx, job.JobID)
	if len(props2) != len(props) {
		t.Fatalf("re-save duplicated: %d -> %d", len(props), len(props2))
	}

	// A deferral's binding constraint must survive the round trip: the board
	// groups deferrals by it and shows its E-0x ID (regression: it was stored
	// but never read back, so every deferral read as "No single rule named").
	deferJob, err := repo.CreateJob(ctx, Job{PlanningDate: mustDate(2026, 9, 26), DepotID: depotID})
	if err != nil {
		t.Fatalf("create defer job: %v", err)
	}
	if err := repo.SaveProposals(ctx, deferJob.JobID, []Proposal{{
		OrderID: props[0].OrderID, Decision: "DEFER", Explanation: "over budget",
		Constraint:        domain.ConstraintFreshTimeBudget,
		ConstraintResults: []domain.ConstraintResult{{Code: domain.ConstraintFreshTimeBudget, Passed: false, Detail: "over budget"}},
	}}); err != nil {
		t.Fatalf("save defer proposal: %v", err)
	}
	deferred, err := repo.LoadProposals(ctx, deferJob.JobID)
	if err != nil {
		t.Fatalf("load defer proposal: %v", err)
	}
	if len(deferred) != 1 || deferred[0].Constraint != domain.ConstraintFreshTimeBudget {
		t.Fatalf("deferred proposal = %+v, want constraint %s", deferred, domain.ConstraintFreshTimeBudget)
	}
}

func mustDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
