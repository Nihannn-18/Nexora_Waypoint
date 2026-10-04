package assignment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"waypoint.lk/api/internal/audit"
	"waypoint.lk/api/internal/store"
)

// testAudit adapts audit.RecordTx to the assignment AuditSink for the
// integration test.
type testAudit struct{}

func (testAudit) RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error {
	return audit.RecordTx(ctx, tx, action, entityType, entityID, actor, depotID, outletID, result, detail)
}

// TestAssignmentIntegration exercises the assignment store against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves the SQL
// matches the schema, the uniqueness rules hold, and each mutation writes its
// audit row in the same transaction.
func TestAssignmentIntegration(t *testing.T) {
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
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var depotID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('ASGDEPOT', 'Assignment Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}

	const (
		driverID = "it-driver"
		otherID  = "it-driver-2"
		storeID  = "it-store"
		actorID  = "it-actor"
		vehID    = "VEH992"
		veh2ID   = "VEH993"
		outID    = "OUT992"
		date     = "2026-09-26"
	)
	deleteRows := func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM driver_vehicle_assignment WHERE vehicle_id IN ($1,$2)`, vehID, veh2ID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM vehicle WHERE vehicle_id IN ($1,$2)`, vehID, veh2ID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM app_user WHERE user_id IN ($1,$2,$3,$4)`, driverID, otherID, storeID, actorID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM outlet WHERE outlet_id = $1`, outID)
		_, _ = db.Pool().Exec(bg, `DELETE FROM audit_log WHERE entity_id IN ($1,$2,$3)`, vehID, veh2ID, outID)
	}
	deleteRows()
	t.Cleanup(func() {
		deleteRows()
		db.Close()
	})

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Pool().Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec %s: %v", sql, err)
		}
	}
	mustExec(`INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ($1,'TRUCK','REEFER',3000,18,'diesel',6,400,$2)`, vehID, depotID)
	mustExec(`INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ($1,'TRUCK','AMBIENT',3000,18,'diesel',6,400,$2)`, veh2ID, depotID)
	mustExec(`INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ($1,'Assignment Outlet','FRESH','Colombo',$2,'STREET','NORMAL','05:00','08:00')`, outID, depotID)
	mustExec(`INSERT INTO app_user (user_id, email, display_name, role, depot_id, is_active)
		VALUES ($1,'it-driver@example.com','IT Driver','DRIVER',$2,TRUE)`, driverID, depotID)
	mustExec(`INSERT INTO app_user (user_id, email, display_name, role, depot_id, is_active)
		VALUES ($1,'it-driver-2@example.com','IT Driver 2','DRIVER',$2,TRUE)`, otherID, depotID)
	mustExec(`INSERT INTO app_user (user_id, email, display_name, role, is_active)
		VALUES ($1,'it-store@example.com','IT Store','STORE_MANAGER',TRUE)`, storeID)
	mustExec(`INSERT INTO app_user (user_id, email, display_name, role, is_active)
		VALUES ($1,'it-actor@example.com','IT Actor','DISPATCHER',TRUE)`, actorID)

	pg := NewPGStore(db.Pool(), testAudit{})
	svc := NewService(pg, nil)

	// --- driver assignment ---
	a, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: vehID, DriverID: driverID, Date: date}, actorID)
	if err != nil {
		t.Fatalf("assign driver: %v", err)
	}
	if a.Driver.UserID != driverID {
		t.Fatalf("assigned = %+v", a)
	}

	// The DB's unique rules make a second vehicle for the same driver a conflict.
	if _, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: veh2ID, DriverID: driverID, Date: date}, actorID); !errors.Is(err, ErrConflict) {
		t.Fatalf("double-book err = %v, want ErrConflict", err)
	}

	// Changing the vehicle's driver is allowed.
	if _, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: vehID, DriverID: otherID, Date: date}, actorID); err != nil {
		t.Fatalf("reassign driver: %v", err)
	}

	// Driver self-read sees the current vehicle.
	got, ok, err := svc.DriverAssignment(ctx, otherID, date)
	if err != nil || !ok || got.VehicleID != vehID {
		t.Fatalf("driver assignment = %+v %v %v", got, ok, err)
	}

	if _, err := svc.UnassignDriver(ctx, vehID, date, actorID); err != nil {
		t.Fatalf("unassign driver: %v", err)
	}
	if _, ok, _ := svc.VehicleAssignment(ctx, vehID, date); ok {
		t.Fatal("assignment still present after unassign")
	}

	// --- outlet manager ---
	m, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: outID, UserID: storeID}, actorID)
	if err != nil || m.UserID != storeID {
		t.Fatalf("assign manager = %+v %v", m, err)
	}

	// The authoritative app_user.outlet_id is what the store's scope reads.
	var outletRef string
	if err := db.Pool().QueryRow(ctx, `SELECT COALESCE(outlet_id,'') FROM app_user WHERE user_id = $1`, storeID).Scan(&outletRef); err != nil || outletRef != outID {
		t.Fatalf("app_user.outlet_id = %q, %v", outletRef, err)
	}

	if _, err := svc.UnassignManager(ctx, outID, actorID); err != nil {
		t.Fatalf("unassign manager: %v", err)
	}

	// --- audit ---
	for _, want := range []struct{ action, entity string }{
		{"DRIVER_ASSIGNED", vehID}, {"DRIVER_UNASSIGNED", vehID},
		{"MANAGER_ASSIGNED", outID}, {"MANAGER_UNASSIGNED", outID},
	} {
		var n int
		if err := db.Pool().QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE action = $1 AND entity_id = $2`, want.action, want.entity).Scan(&n); err != nil {
			t.Fatalf("count audit %s: %v", want.action, err)
		}
		if n == 0 {
			t.Fatalf("%s audit rows = 0, want at least 1", want.action)
		}
	}
}
