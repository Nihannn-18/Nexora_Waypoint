package routes

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"waypoint.lk/api/internal/store"
)

// TestRoutesIntegration exercises the confirmation transaction against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves the
// operational guarantees: atomic route/leg/allocation writes, order status
// transitions, the vehicle/date/trip uniqueness, duplicate-allocation rejection
// under concurrency, idempotent retry, and the deferral_log projection.
func TestRoutesIntegration(t *testing.T) {
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

	// Minimal reference world in its own depot.
	var depotID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('RTDEPOT', 'Routes Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('OUTRT1', 'Routes Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ('VEHRT1', 'TRUCK', 'AMBIENT', 5000, 20, 'diesel', 5, 400, $1)
		ON CONFLICT (vehicle_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
		t.Fatalf("vehicle: %v", err)
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
	// Two confirmed orders and one order to defer.
	mkOrder := func(num string) string {
		var id string
		if err := db.Pool().QueryRow(ctx, `
			INSERT INTO customer_order (order_number, outlet_id, brand, order_date, requested_delivery_date, total_units, total_weight_kg, total_volume_m3, temp_requirement, status)
			VALUES ($1, 'OUTRT1', 'FRESH', '2026-09-25', '2026-09-26', 10, 100, 1.5, 'AMBIENT', 'CONFIRMED')
			ON CONFLICT (order_number) DO UPDATE SET status = 'CONFIRMED' RETURNING order_id`, num).Scan(&id); err != nil {
			t.Fatalf("order %s: %v", num, err)
		}
		return id
	}
	o1, o2, o3 := mkOrder("RT-ORD-1"), mkOrder("RT-ORD-2"), mkOrder("RT-ORD-3")
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM allocation WHERE order_id IN ($1,$2,$3)`, o1, o2, o3)
		_, _ = db.Pool().Exec(bg, `DELETE FROM allocation WHERE route_id IN (SELECT route_id FROM route WHERE vehicle_id = 'VEHRT1')`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM deferral_log WHERE order_id IN ($1,$2,$3)`, o1, o2, o3)
		_, _ = db.Pool().Exec(bg, `DELETE FROM route_leg WHERE route_id IN (SELECT route_id FROM route WHERE vehicle_id = 'VEHRT1')`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM route WHERE vehicle_id = 'VEHRT1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM customer_order WHERE order_number LIKE 'RT-ORD-%'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM vehicle WHERE vehicle_id = 'VEHRT1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM outlet WHERE outlet_id = 'OUTRT1'`)
	})

	repo := NewPGRepository(db.Pool())
	readers := NewPGReaders(db.Pool())

	plan := ConfirmationPlan{
		DepotID: depotID, RouteDate: "2026-09-26", Actor: "",
		Routes: []Route{{
			VehicleID: "VEHRT1", DepotID: depotID, RouteDate: "2026-09-26", TripNo: 1,
			Brand: "FRESH", District: "Colombo", Status: RouteConfirmed,
			OutboundMin: 24, InterStopMin: 8, HandlingMin: 32, TotalTripMin: 64, DistanceKm: 16,
			Legs: []RouteLeg{
				{OrderID: o1, Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUTRT1", Status: LegPending},
				{OrderID: o2, Seq: 1, FromPoint: "OUTRT1", ToOutlet: "OUTRT1", Status: LegPending},
			},
		}},
		Deferrals: []DeferredOrderForConfirm{{OrderID: o3, ReasonType: "CONSTRAINT", Reason: "no budget", ConstraintCode: "FRESH_TIME_BUDGET"}},
	}

	res, err := repo.Confirm(ctx, plan)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(res.RouteIDs) != 1 || len(res.AllocatedOrders) != 2 || len(res.DeferredOrders) != 1 {
		t.Fatalf("result = %+v", res)
	}

	// Route + legs persisted and ordered.
	got, err := repo.GetRoute(ctx, res.RouteIDs[0])
	if err != nil {
		t.Fatalf("get route: %v", err)
	}
	if len(got.Legs) != 2 || got.Legs[0].Seq != 0 || got.Legs[1].Seq != 1 {
		t.Fatalf("legs = %+v", got.Legs)
	}

	// Order statuses transitioned.
	var st string
	_ = db.Pool().QueryRow(ctx, `SELECT status FROM customer_order WHERE order_id = $1`, o1).Scan(&st)
	if st != "ALLOCATED" {
		t.Fatalf("o1 status = %s, want ALLOCATED", st)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT status FROM customer_order WHERE order_id = $1`, o3).Scan(&st)
	if st != "DEFERRED" {
		t.Fatalf("o3 status = %s, want DEFERRED", st)
	}

	// Allocation rows written.
	var allocs int
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM allocation WHERE decision = 'ALLOCATED' AND order_id IN ($1,$2)`, o1, o2).Scan(&allocs)
	if allocs != 2 {
		t.Fatalf("allocated rows = %d, want 2", allocs)
	}

	// Deferral projection written.
	defs, err := repo.ListDeferrals(ctx, "OUTRT1")
	if err != nil {
		t.Fatalf("list deferrals: %v", err)
	}
	if len(defs) != 1 || defs[0].OrderID != o3 || defs[0].ConstraintCode != "FRESH_TIME_BUDGET" {
		t.Fatalf("deferrals = %+v", defs)
	}

	// Idempotency: a second identical confirmation conflicts (no duplicates).
	if _, err := repo.Confirm(ctx, plan); !errors.Is(err, ErrConflict) {
		t.Fatalf("second confirm err = %v, want ErrConflict", err)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM route WHERE vehicle_id = 'VEHRT1' AND route_date = '2026-09-26'`).Scan(&allocs)
	if allocs != 1 {
		t.Fatalf("routes after retry = %d, want 1", allocs)
	}

	// Concurrency: reset order 1 to confirmed, then confirm the same order from
	// two goroutines; exactly one must win.
	if _, err := db.Pool().Exec(ctx, `UPDATE customer_order SET status = 'CONFIRMED' WHERE order_id = $1`, o1); err != nil {
		t.Fatalf("reset o1: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `DELETE FROM allocation WHERE order_id IN ($1,$2)`, o1, o2); err != nil {
		t.Fatalf("clear alloc: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `DELETE FROM route_leg WHERE order_id IN ($1,$2)`, o1, o2); err != nil {
		t.Fatalf("clear leg: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `DELETE FROM route WHERE vehicle_id = 'VEHRT1'`); err != nil {
		t.Fatalf("clear route: %v", err)
	}

	single := ConfirmationPlan{
		DepotID: depotID, RouteDate: "2026-09-26",
		Routes: []Route{{
			VehicleID: "VEHRT1", DepotID: depotID, RouteDate: "2026-09-26", TripNo: 1,
			Brand: "FRESH", District: "Colombo", Status: RouteConfirmed,
			Legs: []RouteLeg{{OrderID: o1, Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUTRT1", Status: LegPending}},
		}},
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = repo.Confirm(context.Background(), single)
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent confirm successes = %d, want exactly 1 (errs=%v)", successes, errs)
	}
	var n int
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM allocation WHERE order_id = $1 AND decision = 'ALLOCATED'`, o1).Scan(&n)
	if n != 1 {
		t.Fatalf("o1 allocated rows after concurrent confirm = %d, want 1", n)
	}
	_ = readers
}
