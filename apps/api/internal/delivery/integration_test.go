package delivery

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"waypoint.lk/api/internal/store"
)

// TestDeliveryIntegration exercises the delivery repository against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves: a delivery
// event persists with POD, item lines and status updates atomically; a replayed
// client_event_id is a duplicate with no second write; and two concurrent events
// for the same leg cannot both create a delivery event for the same idempotency
// key.
func TestDeliveryIntegration(t *testing.T) {
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

	var depotID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('DDEPOT', 'Delivery Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('OUTD1', 'Delivery Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ('VEHD1', 'TRUCK', 'AMBIENT', 5000, 20, 'diesel', 5, 400, $1)
		ON CONFLICT (vehicle_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	var itemID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO item (sku, name, brand, unit_weight_kg, unit_volume_m3, temperature_requirement)
		VALUES ('DD-SKU-1', 'Delivery item', 'FRESH', 1, 0.01, 'AMBIENT')
		ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name RETURNING item_id`).Scan(&itemID); err != nil {
		t.Fatalf("item: %v", err)
	}
	var orderID, orderItemID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO customer_order (order_number, outlet_id, brand, order_date, requested_delivery_date, total_units, total_weight_kg, total_volume_m3, temp_requirement, status)
		VALUES ('DD-ORD-1', 'OUTD1', 'FRESH', '2026-09-25', '2026-09-26', 10, 10, 0.1, 'AMBIENT', 'IN_TRANSIT')
		ON CONFLICT (order_number) DO UPDATE SET status = 'IN_TRANSIT' RETURNING order_id`).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO order_item (order_id, item_id, quantity, unit_weight_kg_snapshot, unit_volume_m3_snapshot, total_weight_kg, total_volume_m3)
		VALUES ($1, $2, 10, 1, 0.01, 10, 0.1) RETURNING order_item_id`, orderID, itemID).Scan(&orderItemID); err != nil {
		if err2 := db.Pool().QueryRow(ctx, `SELECT order_item_id FROM order_item WHERE order_id = $1 LIMIT 1`, orderID).Scan(&orderItemID); err2 != nil {
			t.Fatalf("order_item: %v (fallback %v)", err, err2)
		}
	}
	var routeID, legID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO route (vehicle_id, depot_id, route_date, trip_no, brand, district, status)
		VALUES ('VEHD1', $1, '2026-09-26', 1, 'FRESH', 'Colombo', 'CONFIRMED')
		ON CONFLICT (vehicle_id, route_date, trip_no) DO UPDATE SET status = 'CONFIRMED' RETURNING route_id`, depotID).Scan(&routeID); err != nil {
		t.Fatalf("route: %v", err)
	}
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO route_leg (route_id, order_id, seq, from_point, to_outlet, status)
		VALUES ($1, $2, 0, 'DEPOT', 'OUTD1', 'IN_TRANSIT')
		ON CONFLICT (route_id, order_id) DO UPDATE SET status = 'IN_TRANSIT' RETURNING leg_id`, routeID, orderID).Scan(&legID); err != nil {
		t.Fatalf("route_leg: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM order_item_delivery WHERE delivery_event_id IN (SELECT event_id FROM delivery_event WHERE leg_id = $1)`, legID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM delivery_event WHERE leg_id = $1`, legID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM route_leg WHERE route_id = $1`, routeID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM route WHERE route_id = $1`, routeID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM order_item WHERE order_id = $1`, orderID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM customer_order WHERE order_number = 'DD-ORD-1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM item WHERE sku = 'DD-SKU-1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM vehicle WHERE vehicle_id = 'VEHD1'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM outlet WHERE outlet_id = 'OUTD1'`)
	})

	repo := NewPGRepository(db.Pool())
	leg, err := repo.LegContext(ctx, legID)
	if err != nil {
		t.Fatalf("leg context: %v", err)
	}
	if leg.RouteID != routeID || leg.DepotID != depotID || len(leg.OrderIDs) != 1 {
		t.Fatalf("leg = %+v", leg)
	}

	in := EventInput{
		LegID: legID, ClientEventID: "11111111-1111-1111-1111-111111111111",
		Outcome: OutcomeDelivered, OccurredAt: "2026-09-26T07:42:00+05:30", CreatedOffline: true,
		Items: []ItemDelivery{{OrderItemID: orderItemID, DeliveredQty: 10}},
		Pod:   Pod{Type: PodPhoto, ReceiverName: "Nimal", PhotoRef: "pod/" + legID + "/abc"},
	}

	res, err := repo.Record(ctx, "", in, leg)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if res.Status != SyncAccepted || res.ServerEventID == "" {
		t.Fatalf("result = %+v", res)
	}

	// Event, item line and status persisted.
	var n int
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM delivery_event WHERE leg_id = $1`, legID).Scan(&n)
	if n != 1 {
		t.Fatalf("delivery events = %d, want 1", n)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM order_item_delivery WHERE delivery_event_id = $1`, res.ServerEventID).Scan(&n)
	if n != 1 {
		t.Fatalf("item deliveries = %d, want 1", n)
	}
	var legStatus, orderStatus string
	_ = db.Pool().QueryRow(ctx, `SELECT status FROM route_leg WHERE leg_id = $1`, legID).Scan(&legStatus)
	if legStatus != "DELIVERED" {
		t.Fatalf("leg status = %s, want DELIVERED", legStatus)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT status FROM customer_order WHERE order_id = $1`, orderID).Scan(&orderStatus)
	if orderStatus != "DELIVERED" {
		t.Fatalf("order status = %s, want DELIVERED", orderStatus)
	}

	// POD round-trip.
	got, err := repo.GetEvent(ctx, in.ClientEventID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got.Pod.ReceiverName != "Nimal" || got.Pod.PhotoRef == "" || got.Outcome != OutcomeDelivered {
		t.Fatalf("event = %+v", got)
	}

	// Idempotent replay: DUPLICATE, no second event.
	replay, err := repo.Record(ctx, "", in, leg)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Status != SyncDuplicate || replay.ServerEventID != res.ServerEventID {
		t.Fatalf("replay = %+v", replay)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM delivery_event WHERE leg_id = $1`, legID).Scan(&n)
	if n != 1 {
		t.Fatalf("events after replay = %d, want 1", n)
	}

	// Concurrency: two events with the SAME client_event_id must not both insert.
	var wg sync.WaitGroup
	results := make([]EventResult, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			e := in
			e.ClientEventID = "22222222-2222-2222-2222-222222222222"
			e.Outcome = OutcomeDelayed
			e.Pod = Pod{}
			results[idx], errs[idx] = repo.Record(context.Background(), "", e, leg)
		}(i)
	}
	wg.Wait()
	accepted := 0
	for i, e := range errs {
		if e == nil && results[i].Status == SyncAccepted {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent same-id accepts = %d, want exactly 1 (errs=%v)", accepted, errs)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM delivery_event WHERE client_event_id = '22222222-2222-2222-2222-222222222222'`).Scan(&n)
	if n != 1 {
		t.Fatalf("concurrent same-id events = %d, want 1", n)
	}

	// Driver read model: route, outlet window and order lines come from the DB.
	detail, err := repo.LegDetail(ctx, legID)
	if err != nil {
		t.Fatalf("leg detail: %v", err)
	}
	if detail.Route.VehicleID != "VEHD1" || detail.Outlet.WindowOpen != "05:00" || detail.Outlet.WindowClose != "08:00" ||
		len(detail.Orders) != 1 || len(detail.Orders[0].Lines) != 1 || detail.Orders[0].Lines[0].Quantity != 10 {
		t.Fatalf("detail = %+v", detail)
	}
	routes, err := repo.DriverRoutes(ctx, depotID, "2026-09-26")
	if err != nil || len(routes) != 1 || len(routes[0].Stops) != 1 || routes[0].Stops[0].LegID != legID {
		t.Fatalf("routes = %+v, %v", routes, err)
	}
	if _, err := repo.LegDetail(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing leg = %v, want ErrNotFound", err)
	}

	// FAILED with a reason: persisted, read back, and a replay stays one row.
	failed := EventInput{
		LegID: legID, ClientEventID: "33333333-3333-3333-3333-333333333333",
		Outcome: OutcomeFailed, OccurredAt: "2026-09-26T08:10:00+05:30", CreatedOffline: true,
		ReasonCode: "OUTLET_CLOSED",
	}
	for i := 0; i < 2; i++ {
		if _, err := repo.Record(ctx, "", failed, leg); err != nil {
			t.Fatalf("record failed #%d: %v", i, err)
		}
	}
	gotFailed, err := repo.GetEvent(ctx, failed.ClientEventID)
	if err != nil || gotFailed.ReasonCode != "OUTLET_CLOSED" || gotFailed.Outcome != OutcomeFailed {
		t.Fatalf("failed event = %+v, %v", gotFailed, err)
	}
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM delivery_event WHERE client_event_id = $1`, failed.ClientEventID).Scan(&n)
	if n != 1 {
		t.Fatalf("failed events after replay = %d, want 1", n)
	}
	if got.ReasonCode != "" {
		t.Fatalf("delivered event reasonCode = %q, want empty", got.ReasonCode)
	}
}
