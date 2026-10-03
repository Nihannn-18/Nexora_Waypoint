package orders

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/store"
)

// TestRepositoryIntegration exercises the pgx repository against a real
// PostgreSQL when one is reachable, and skips otherwise so the unit suite stays
// green without a database. Point WAYPOINT_TEST_DATABASE_URL at a disposable
// database to run it.
//
// It proves what the unit tests cannot: that the hand-written SQL matches the
// migration — the insert/scan columns, the snapshot round-trip, the generated
// order number, and the line totals.
func TestRepositoryIntegration(t *testing.T) {
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

	// The repository needs an outlet and an item to reference. Create the
	// minimum in a depot so the foreign keys resolve, without depending on the
	// full reference seed.
	var depotID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('TESTDEPOT', 'Test Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("create depot: %v", err)
	}
	var outletID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint,
			window_open_time, window_close_time)
		VALUES ('OUTTEST1', 'Test Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO UPDATE SET name = EXCLUDED.name
		RETURNING outlet_id`, depotID).Scan(&outletID); err != nil {
		t.Fatalf("create outlet: %v", err)
	}
	var itemID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO item (sku, name, brand, unit_weight_kg, unit_volume_m3, temperature_requirement)
		VALUES ('TEST-INT-1', 'Integration item', 'FRESH', 2.5, 0.02, 'AMBIENT')
		ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name
		RETURNING item_id`).Scan(&itemID); err != nil {
		t.Fatalf("create item: %v", err)
	}

	repo := NewPGRepository(db.Pool())
	delivery := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)
	order := Order{
		OutletID:              outletID,
		Brand:                 domain.BrandFresh,
		OrderDate:             time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC),
		RequestedDeliveryDate: delivery,
		TotalUnits:            4,
		TotalWeightKg:         10.0,
		TotalVolumeM3:         0.08,
		TempRequirement:       domain.TempAmbient,
		Status:                domain.OrderPlaced,
		Lines: []OrderLine{{
			ItemID: itemID, Quantity: 4,
			UnitWeightKgSnapshot: 2.5, UnitVolumeM3Snapshot: 0.02,
			TotalWeightKg: 10.0, TotalVolumeM3: 0.08,
		}},
	}

	created, err := repo.Create(ctx, order)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !ValidateOrderNumber(created.OrderNumber) {
		t.Fatalf("generated order number %q is malformed", created.OrderNumber)
	}
	if len(created.Lines) != 1 || created.Lines[0].Quantity != 4 {
		t.Fatalf("lines not persisted: %+v", created.Lines)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM customer_order WHERE order_id = $1`, created.OrderID)
	})

	// Round-trip: read back by id and by number.
	got, err := repo.GetByID(ctx, created.OrderID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.TotalWeightKg != 10.0 || got.TotalUnits != 4 {
		t.Fatalf("header round-trip = %+v", got)
	}
	if len(got.Lines) != 1 || got.Lines[0].UnitWeightKgSnapshot != 2.5 {
		t.Fatalf("line snapshot round-trip = %+v", got.Lines)
	}
	if _, err := repo.GetByNumber(ctx, created.OrderNumber); err != nil {
		t.Fatalf("get by number: %v", err)
	}

	// A second create must get a distinct, higher number for the year.
	second, err := repo.Create(ctx, order)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM customer_order WHERE order_id = $1`, second.OrderID)
	})
	if second.OrderNumber == created.OrderNumber {
		t.Fatalf("duplicate order number %q", second.OrderNumber)
	}

	// Confirm persists the status.
	confirmed, err := repo.UpdateStatus(ctx, created.OrderID, string(domain.OrderConfirmed))
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if confirmed.Status != domain.OrderConfirmed {
		t.Fatalf("status = %s, want CONFIRMED", confirmed.Status)
	}

	// Not found is reported as ErrNotFound.
	if _, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing order = %v, want ErrNotFound", err)
	}
}
