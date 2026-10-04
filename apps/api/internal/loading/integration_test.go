package loading

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"waypoint.lk/api/internal/store"
)

// TestLoadingIntegration exercises the loading repository against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves: the picking
// list is derived from the confirmed route's order lines, a shortfall submission
// persists load_item rows, a retry updates rather than duplicates (the
// route_id/order_item_id unique index), and a concurrent double-submit does not
// create duplicates.
func TestLoadingIntegration(t *testing.T) {
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
		INSERT INTO depot (code, name) VALUES ('LDEPOT', 'Loading Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('OUTLD1', 'Loading Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ('VEHLD1', 'TRUCK', 'AMBIENT', 5000, 20, 'diesel', 5, 400, $1)
		ON CONFLICT (vehicle_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	var itemID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO item (sku, name, brand, unit_weight_kg, unit_volume_m3, temperature_requirement)
		VALUES ('LD-SKU-1', 'Loading item', 'FRESH', 1, 0.01, 'AMBIENT')
		ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name RETURNING item_id`).Scan(&itemID); err != nil {
		t.Fatalf("item: %v", err)
	}
	var orderID, orderItemID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO customer_order (order_number, outlet_id, brand, order_date, requested_delivery_date, total_units, total_weight_kg, total_volume_m3, temp_requirement, status)
		VALUES ('LD-ORD-1', 'OUTLD1', 'FRESH', '2026-09-25', '2026-09-26', 10, 10, 0.1, 'AMBIENT', 'ALLOCATED')
		ON CONFLICT (order_number) DO UPDATE SET status = 'ALLOCATED' RETURNING order_id`).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO order_item (order_id, item_id, quantity, unit_weight_kg_snapshot, unit_volume_m3_snapshot, total_weight_kg, total_volume_m3)
		VALUES ($1, $2, 10, 1, 0.01, 10, 0.1)
		ON CONFLICT DO NOTHING RETURNING order_item_id`, orderID, itemID).Scan(&orderItemID); err != nil {
		// If a previous run left a line, fetch it.
		if err2 := db.Pool().QueryRow(ctx, `SELECT order_item_id FROM order_item WHERE order_id = $1 LIMIT 1`, orderID).Scan(&orderItemID); err2 != nil {
			t.Fatalf("order_item: %v (fallback %v)", err, err2)
		}
	}
	var routeID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO route (vehicle_id, depot_id, route_date, trip_no, brand, district, status)
		VALUES ('VEHLD1', $1, '2026-09-26', 1, 'FRESH', 'Colombo', 'CONFIRMED')
		ON CONFLICT (vehicle_id, route_date, trip_no) DO UPDATE SET status = 'CONFIRMED'
		RETURNING route_id`, depotID).Scan(&routeID); err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO route_leg (route_id, order_id, seq, from_point, to_outlet, status)
		VALUES ($1, $2, 0, 'DEPOT', 'OUTLD1', 'PENDING')
		ON CONFLICT (route_id, order_id) DO NOTHING`, routeID, orderID); err != nil {
		t.Fatalf("route_leg: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM load_item WHERE route_id = $1`, routeID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM route_leg WHERE route_id = $1`, routeID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM route WHERE route_id = $1`, routeID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM order_item WHERE order_id = $1`, orderID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM customer_order WHERE order_number = 'LD-ORD-1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM item WHERE sku = 'LD-SKU-1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM vehicle WHERE vehicle_id = 'VEHLD1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM outlet WHERE outlet_id = 'OUTLD1'`)
	})

	repo := NewPGRepository(db.Pool())

	// Active-run resolution: with no date the depot's confirmed run is the one
	// on/after today. Before the run date it is 2026-09-26; a date after the run
	// still returns the latest run rather than an empty list.
	active, err := repo.ActiveRouteDate(ctx, depotID, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("active route date: %v", err)
	}
	if active != "2026-09-26" {
		t.Fatalf("active route date = %q, want 2026-09-26", active)
	}
	past, err := repo.ActiveRouteDate(ctx, depotID, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("active route date (past run): %v", err)
	}
	if past != "2026-09-26" {
		t.Fatalf("active route date after the run = %q, want the latest run 2026-09-26", past)
	}

	// Picking list reads the ordered quantity from order_item, not the client.
	rl, err := repo.RouteLoading(ctx, routeID)
	if err != nil {
		t.Fatalf("route loading: %v", err)
	}
	if len(rl.Lines) != 1 || rl.Lines[0].OrderedQty != 10 {
		t.Fatalf("picking list = %+v", rl.Lines)
	}
	if rl.Ready() {
		t.Fatal("a route with no load state is not ready")
	}

	// Record a shortfall (8 loaded, 2 missing).
	after, err := repo.RecordShortfalls(ctx, routeID, "", []LineUpdate{
		{OrderItemID: orderItemID, LoadedQty: 8, MissingQty: 2},
	})
	if err != nil {
		t.Fatalf("record shortfalls: %v", err)
	}
	if !after.Ready() || after.Lines[0].ShortfallQty() != 2 {
		t.Fatalf("after = %+v", after)
	}

	// Retry: update, not duplicate.
	if _, err := repo.RecordShortfalls(ctx, routeID, "", []LineUpdate{
		{OrderItemID: orderItemID, LoadedQty: 10},
	}); err != nil {
		t.Fatalf("retry: %v", err)
	}
	var n int
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM load_item WHERE route_id = $1 AND order_item_id = $2`, routeID, orderItemID).Scan(&n)
	if n != 1 {
		t.Fatalf("load_item rows after retry = %d, want 1", n)
	}
	// Order quantity untouched.
	var qty int
	_ = db.Pool().QueryRow(ctx, `SELECT quantity FROM order_item WHERE order_item_id = $1`, orderItemID).Scan(&qty)
	if qty != 10 {
		t.Fatalf("ordered quantity changed to %d; a shortfall must not mutate the order", qty)
	}

	// Concurrency: two submits of the same line must not duplicate.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(loaded int) {
			defer wg.Done()
			_, _ = repo.RecordShortfalls(context.Background(), routeID, "", []LineUpdate{
				{OrderItemID: orderItemID, LoadedQty: loaded, MissingQty: 10 - loaded},
			})
		}(7 + i)
	}
	wg.Wait()
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM load_item WHERE route_id = $1 AND order_item_id = $2`, routeID, orderItemID).Scan(&n)
	if n != 1 {
		t.Fatalf("load_item rows after concurrent submits = %d, want 1", n)
	}
	// The line still reconciles regardless of which writer won.
	final, err := repo.RouteLoading(ctx, routeID)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	if !final.Lines[0].Complete() {
		t.Fatalf("final line does not reconcile: %+v", final.Lines[0])
	}
}
