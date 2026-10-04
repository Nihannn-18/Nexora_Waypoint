package receipts

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"waypoint.lk/api/internal/store"
)

// TestReceiptsIntegration exercises the receipts repository against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves: the S-06
// read carries the loader's count and flag and the driver's POD; a GRN persists
// with its lines and moves the order DELIVERED → RECEIVED atomically; and two
// concurrent submissions for one order record exactly one receipt.
func TestReceiptsIntegration(t *testing.T) {
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
	// Registered first so it runs last: the purge below still needs the pool.
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool := db.Pool()

	// purge removes this test's rows, children first. It also runs before the
	// fixture, so rows left by an interrupted run never break the next one.
	purge := func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM receipt WHERE order_id IN (SELECT order_id FROM customer_order WHERE order_number = 'RR-ORD-1')`,
			`DELETE FROM delivery_event WHERE leg_id IN (SELECT leg_id FROM route_leg WHERE to_outlet = 'OUTR1')`,
			`DELETE FROM load_item WHERE route_id IN (SELECT route_id FROM route WHERE vehicle_id = 'VEHR1')`,
			`DELETE FROM route_leg WHERE to_outlet = 'OUTR1'`,
			`DELETE FROM route WHERE vehicle_id = 'VEHR1'`,
			`DELETE FROM order_item WHERE order_id IN (SELECT order_id FROM customer_order WHERE order_number = 'RR-ORD-1')`,
			`DELETE FROM customer_order WHERE order_number = 'RR-ORD-1'`,
			`DELETE FROM item WHERE sku IN ('RR-SKU-1', 'RR-SKU-2')`,
			`DELETE FROM vehicle WHERE vehicle_id = 'VEHR1'`,
			`DELETE FROM outlet WHERE outlet_id = 'OUTR1'`,
		} {
			if _, err := pool.Exec(bg, stmt); err != nil {
				t.Logf("cleanup %q: %v", stmt, err)
			}
		}
	}
	purge()
	t.Cleanup(purge)

	var depotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('RDEPOT', 'Receipts Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('OUTR1', 'Receipts Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ('VEHR1', 'TRUCK', 'AMBIENT', 5000, 20, 'diesel', 5, 400, $1)
		ON CONFLICT (vehicle_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	itemIDs := make([]string, 2)
	for i, sku := range []string{"RR-SKU-1", "RR-SKU-2"} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO item (sku, name, brand, unit_weight_kg, unit_volume_m3, temperature_requirement)
			VALUES ($1, 'Receipt item', 'FRESH', 1, 0.01, 'AMBIENT')
			ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name RETURNING item_id`, sku).Scan(&itemIDs[i]); err != nil {
			t.Fatalf("item %s: %v", sku, err)
		}
	}

	var orderID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO customer_order (order_number, outlet_id, brand, order_date, requested_delivery_date, total_units, total_weight_kg, total_volume_m3, temp_requirement, status)
		VALUES ('RR-ORD-1', 'OUTR1', 'FRESH', '2026-09-25', '2026-09-26', 15, 15, 0.15, 'AMBIENT', 'DELIVERED')
		RETURNING order_id`).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	lineIDs := make([]string, 2)
	for i, qty := range []int{10, 5} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO order_item (order_id, item_id, quantity, unit_weight_kg_snapshot, unit_volume_m3_snapshot, total_weight_kg, total_volume_m3)
			VALUES ($1, $2, $3::int, 1, 0.01, $3::int, $3::int * 0.01) RETURNING order_item_id`, orderID, itemIDs[i], qty).Scan(&lineIDs[i]); err != nil {
			t.Fatalf("order_item: %v", err)
		}
	}
	var routeID, legID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO route (vehicle_id, depot_id, route_date, trip_no, brand, district, status)
		VALUES ('VEHR1', $1, '2026-09-26', 1, 'FRESH', 'Colombo', 'CONFIRMED')
		ON CONFLICT (vehicle_id, route_date, trip_no) DO UPDATE SET status = 'CONFIRMED' RETURNING route_id`, depotID).Scan(&routeID); err != nil {
		t.Fatalf("route: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO route_leg (route_id, order_id, seq, from_point, to_outlet, status)
		VALUES ($1, $2, 0, 'DEPOT', 'OUTR1', 'DELIVERED') RETURNING leg_id`, routeID, orderID).Scan(&legID); err != nil {
		t.Fatalf("route_leg: %v", err)
	}
	// The loader loaded 8 of 10 on the first line and flagged 2 missing; the
	// second line was never counted.
	if _, err := pool.Exec(ctx, `
		INSERT INTO load_item (route_id, order_item_id, ordered_qty, loaded_qty, damaged_qty, missing_qty, recorded_at)
		VALUES ($1, $2, 10, 8, 0, 2, now())`, routeID, lineIDs[0]); err != nil {
		t.Fatalf("load_item: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery_event (leg_id, outcome, pod_receiver_name, pod_photo, client_event_id, client_created_at)
		VALUES ($1, 'DELIVERED', 'Ishara', $2, gen_random_uuid(), '2026-09-26T07:42:00+05:30')`, legID, "pod/"+legID+"/abc"); err != nil {
		t.Fatalf("delivery_event: %v", err)
	}

	repo := NewPGRepository(pool)

	// The read model: the loader's count and flag, and the driver's POD.
	order, err := repo.OrderContext(ctx, orderID)
	if err != nil {
		t.Fatalf("order context: %v", err)
	}
	if order.Status != "DELIVERED" || order.DepotID != depotID || len(order.Lines) != 2 {
		t.Fatalf("order = %+v", order)
	}
	if order.Lines[0].ExpectedQty() != 8 || order.Lines[0].LoaderFlag == nil || order.Lines[0].LoaderFlag.MissingQty != 2 {
		t.Fatalf("line 1 = %+v, want 8 expected with 2 flagged missing", order.Lines[0])
	}
	if order.Lines[1].ExpectedQty() != 5 || order.Lines[1].ExpectedSource() != ExpectedFromOrder {
		t.Fatalf("line 2 = %+v, want 5 expected from the order", order.Lines[1])
	}
	if _, err := repo.OrderContext(ctx, "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id err = %v, want ErrNotFound", err)
	}
	pod, err := repo.Pod(ctx, orderID)
	if err != nil || pod == nil || pod.ReceiverName != "Ishara" || pod.PhotoRef != "pod/"+legID+"/abc" {
		t.Fatalf("pod = %+v, err = %v", pod, err)
	}

	lines, err := BuildLines(order, Input{Lines: []LineInput{
		{OrderItemID: lineIDs[0], ReceivedQty: 7, DamagedQty: 1},
		{OrderItemID: lineIDs[1], ReceivedQty: 5},
	}})
	if err != nil {
		t.Fatalf("build lines: %v", err)
	}
	receivedAt := time.Date(2026, 9, 26, 7, 50, 0, 0, time.FixedZone("Asia/Colombo", 5*3600+1800))
	rec := Receipt{OrderID: orderID, ReceivedAt: receivedAt, Notes: "one carton crushed", Lines: lines}

	// Two concurrent submissions: exactly one records, the other is refused.
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = repo.Create(ctx, order, rec)
		}(i)
	}
	wg.Wait()
	succeeded := 0
	for _, err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAlreadyReceived):
		default:
			t.Fatalf("create: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful submissions = %d, want 1", succeeded)
	}

	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM customer_order WHERE order_id = $1`, orderID).Scan(&status)
	if status != "RECEIVED" {
		t.Fatalf("order status = %q, want RECEIVED", status)
	}
	stored, err := repo.Receipt(ctx, orderID)
	if err != nil || stored == nil {
		t.Fatalf("receipt = %+v, err = %v", stored, err)
	}
	if stored.Status() != StatusReceivedWithIssue || len(stored.Lines) != 2 || stored.Notes != "one carton crushed" {
		t.Fatalf("stored receipt = %+v", stored)
	}
	var dbStatus string
	_ = pool.QueryRow(ctx, `SELECT status FROM receipt WHERE order_id = $1`, orderID).Scan(&dbStatus)
	if dbStatus != StatusReceivedWithIssue {
		t.Fatalf("receipt.status = %q, want RECEIVED_WITH_ISSUE", dbStatus)
	}
}
