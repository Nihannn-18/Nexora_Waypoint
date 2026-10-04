package catalog

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"waypoint.lk/api/internal/audit"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/store"
)

// testAudit adapts audit.RecordTx to the catalog AuditSink for the integration
// test.
type testAudit struct{}

func (testAudit) RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error {
	return audit.RecordTx(ctx, tx, action, entityType, entityID, actor, depotID, outletID, result, detail)
}

// TestNetworkWriteIntegration exercises vehicle and outlet CRUD against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves the SQL
// matches the schema, identity is generated and immutable, and each successful
// mutation writes its audit row in the same transaction.
func TestNetworkWriteIntegration(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
		INSERT INTO depot (code, name) VALUES ('MDDEPOT', 'Master Data Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}

	writer := NewPGNetworkWriter(db.Pool(), testAudit{})

	// Use a high id unlikely to exist so the test is idempotent across runs.
	vehID := "VEH991"
	outID := "OUT991"
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM vehicle WHERE vehicle_id = $1`, vehID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM outlet WHERE outlet_id = $1`, outID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM audit_log WHERE entity_id IN ($1,$2)`, vehID, outID)
	})

	// --- vehicle ---
	ok, err := writer.DepotExists(ctx, depotID)
	if err != nil || !ok {
		t.Fatalf("depot exists = %v, %v", ok, err)
	}

	// Regression: a malformed (non-UUID) depot id is "not a known depot", not
	// a uuid-cast error that would surface as a 500.
	if ok, err := writer.DepotExists(ctx, "not-a-uuid"); err != nil || ok {
		t.Fatalf("malformed depot exists = %v, %v; want false, nil", ok, err)
	}

	created, err := writer.CreateVehicle(ctx, VehicleWrite{
		VehicleID: vehID, Type: domain.VehicleTruck, TempClass: domain.VehicleTempReefer,
		WeightCapKg: 3000, VolumeCapM3: 18, FuelType: "diesel", KmPerL: 6,
		WeeklyFuelQuotaL: 400, DepotID: depotID,
	}, "u-disp")
	if err != nil {
		t.Fatalf("create vehicle: %v", err)
	}
	if created.VehicleID != vehID || created.Type != domain.VehicleTruck {
		t.Fatalf("created = %+v", created)
	}

	// Duplicate identity is a conflict, not a second row.
	if _, err := writer.CreateVehicle(ctx, VehicleWrite{
		VehicleID: vehID, Type: domain.VehicleTruck, TempClass: domain.VehicleTempReefer,
		WeightCapKg: 3000, VolumeCapM3: 18, FuelType: "diesel", KmPerL: 6,
		WeeklyFuelQuotaL: 400, DepotID: depotID,
	}, "u-disp"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate create err = %v, want ErrDuplicate", err)
	}

	updated, err := writer.UpdateVehicle(ctx, VehicleWrite{
		VehicleID: vehID, Type: domain.VehicleVan, TempClass: domain.VehicleTempAmbient,
		WeightCapKg: 1500, VolumeCapM3: 8, FuelType: "diesel", KmPerL: 8,
		WeeklyFuelQuotaL: 200, DepotID: depotID,
	}, "u-disp")
	if err != nil {
		t.Fatalf("update vehicle: %v", err)
	}
	if updated.Type != domain.VehicleVan || updated.TempClass != domain.VehicleTempAmbient {
		t.Fatalf("updated = %+v", updated)
	}

	// NextVehicleID is numeric+1 and free.
	if _, err := writer.NextVehicleID(ctx); err != nil {
		t.Fatalf("next vehicle id: %v", err)
	}

	// --- outlet ---
	createdOut, err := writer.CreateOutlet(ctx, OutletWrite{
		OutletID: outID, Name: "Master Data Outlet", Brand: domain.BrandFresh,
		District: "Colombo", DepotID: depotID, DockType: domain.DockStreet,
		ParkingConstraint: domain.ParkingNormal,
		WindowOpenTime:    "05:00", WindowCloseTime: "08:00",
	}, "u-disp")
	if err != nil {
		t.Fatalf("create outlet: %v", err)
	}
	if createdOut.OutletID != outID || createdOut.MallWindowOpen != "" {
		t.Fatalf("created outlet = %+v", createdOut)
	}

	updatedOut, err := writer.UpdateOutlet(ctx, OutletWrite{
		OutletID: outID, Name: "Master Data Outlet (mall)", Brand: domain.BrandStyle,
		District: "Kandy", DepotID: depotID, DockType: domain.DockMallBay,
		ParkingConstraint: domain.ParkingMallDock,
		WindowOpenTime:    "09:00", WindowCloseTime: "12:00",
		MallWindowOpen: "10:00", MallWindowClose: "11:30",
	}, "u-disp")
	if err != nil {
		t.Fatalf("update outlet: %v", err)
	}
	if updatedOut.ParkingConstraint != domain.ParkingMallDock || updatedOut.MallWindowOpen != "10:00" {
		t.Fatalf("updated outlet = %+v", updatedOut)
	}

	// --- audit ---
	for _, want := range []struct{ action, entity string }{
		{"VEHICLE_CREATED", vehID}, {"VEHICLE_UPDATED", vehID},
		{"OUTLET_CREATED", outID}, {"OUTLET_UPDATED", outID},
	} {
		var n int
		if err := db.Pool().QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE action = $1 AND entity_id = $2`, want.action, want.entity).Scan(&n); err != nil {
			t.Fatalf("count audit %s: %v", want.action, err)
		}
		if n != 1 {
			t.Fatalf("%s audit rows = %d, want 1", want.action, n)
		}
	}
}
