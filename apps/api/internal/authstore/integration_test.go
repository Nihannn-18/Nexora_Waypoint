package authstore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/password"
	"waypoint.lk/api/internal/store"
)

// TestAuthStoreIntegration exercises the Go authentication store against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves credential
// verification, opaque session lifecycle (create/validate/expire/delete),
// identity loading and media owner scope.
func TestAuthStoreIntegration(t *testing.T) {
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
	pool := db.Pool()
	s := New(pool)

	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, stmt := range []string{
			`DELETE FROM session WHERE user_id LIKE 'auth-%'`,
			`DELETE FROM route_leg WHERE to_outlet = 'AUTHO1'`,
			`DELETE FROM route WHERE vehicle_id = 'VEHA1'`,
			`DELETE FROM order_item WHERE order_id IN (SELECT order_id FROM customer_order WHERE order_number = 'AUTH-ORD-1')`,
			`DELETE FROM customer_order WHERE order_number = 'AUTH-ORD-1'`,
			`DELETE FROM app_user WHERE user_id LIKE 'auth-%'`,
			`DELETE FROM outlet WHERE outlet_id = 'AUTHO1'`,
			`DELETE FROM item WHERE sku = 'AUTH-SKU-1'`,
			`DELETE FROM vehicle WHERE vehicle_id = 'VEHA1'`,
			`DELETE FROM depot WHERE code = 'AUTHDEPOT'`,
		} {
			_, _ = pool.Exec(cleanup, stmt)
		}
	})

	var depotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('AUTHDEPOT', 'Auth Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('AUTHO1', 'Auth Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}

	hash, err := password.Hash("secret-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO app_user (user_id, email, display_name, role, depot_id, password_hash, is_active)
		VALUES ('auth-driver', 'auth.driver@waypoint.lk', 'Auth Driver', 'DRIVER', $1, $2, true)
		ON CONFLICT (user_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, is_active = true, depot_id = EXCLUDED.depot_id`,
		depotID, hash); err != nil {
		t.Fatalf("driver app_user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO app_user (user_id, email, display_name, role, depot_id, password_hash, is_active)
		VALUES ('auth-inactive', 'auth.inactive@waypoint.lk', 'Inactive', 'LOADER', $1, $2, false)
		ON CONFLICT (user_id) DO UPDATE SET is_active = false`,
		depotID, hash); err != nil {
		t.Fatalf("inactive app_user: %v", err)
	}

	t.Run("valid credentials authenticate", func(t *testing.T) {
		id, err := s.Authenticate(ctx, "auth.driver@waypoint.lk", "secret-password")
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if id.UserID != "auth-driver" || id.Role != domain.RoleDriver || id.DepotID != depotID || id.Name != "Auth Driver" {
			t.Fatalf("identity = %+v", id)
		}
	})

	t.Run("wrong password is rejected", func(t *testing.T) {
		if _, err := s.Authenticate(ctx, "auth.driver@waypoint.lk", "wrong"); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("unknown email is rejected", func(t *testing.T) {
		if _, err := s.Authenticate(ctx, "nobody@waypoint.lk", "whatever"); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("inactive account is forbidden", func(t *testing.T) {
		if _, err := s.Authenticate(ctx, "auth.inactive@waypoint.lk", "secret-password"); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("session lifecycle", func(t *testing.T) {
		raw, tokenHash, err := auth.NewSessionToken()
		if err != nil {
			t.Fatalf("NewSessionToken: %v", err)
		}
		if err := s.CreateSession(ctx, "auth-driver", tokenHash, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		userID, err := s.ValidateToken(ctx, raw)
		if err != nil || userID != "auth-driver" {
			t.Fatalf("ValidateToken = %q, %v", userID, err)
		}
		if err := s.DeleteSession(ctx, tokenHash); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		if _, err := s.ValidateToken(ctx, raw); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("after logout err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("expired session is rejected", func(t *testing.T) {
		raw, tokenHash, err := auth.NewSessionToken()
		if err != nil {
			t.Fatalf("NewSessionToken: %v", err)
		}
		if err := s.CreateSession(ctx, "auth-driver", tokenHash, time.Now().Add(-time.Minute)); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if _, err := s.ValidateToken(ctx, raw); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("unknown token is rejected", func(t *testing.T) {
		if _, err := s.ValidateToken(ctx, "no-such-token"); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("LoadIdentity returns role and scope", func(t *testing.T) {
		id, err := s.LoadIdentity(ctx, "auth-driver")
		if err != nil {
			t.Fatalf("LoadIdentity: %v", err)
		}
		if id.Role != domain.RoleDriver || id.DepotID != depotID || id.Name != "Auth Driver" {
			t.Fatalf("identity = %+v", id)
		}
	})

	t.Run("media owner scope resolves depot and outlet", func(t *testing.T) {
		var itemID, orderID, orderItemID, routeID, legID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO item (sku, name, brand, unit_weight_kg, unit_volume_m3, temperature_requirement)
			VALUES ('AUTH-SKU-1', 'Auth item', 'FRESH', 1, 0.01, 'AMBIENT')
			ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name RETURNING item_id`).Scan(&itemID); err != nil {
			t.Fatalf("item: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO customer_order (order_number, outlet_id, brand, order_date, requested_delivery_date, total_units, total_weight_kg, total_volume_m3, temp_requirement, status)
			VALUES ('AUTH-ORD-1', 'AUTHO1', 'FRESH', '2026-09-25', '2026-09-26', 1, 1, 0.01, 'AMBIENT', 'ALLOCATED')
			ON CONFLICT (order_number) DO UPDATE SET status = 'ALLOCATED' RETURNING order_id`).Scan(&orderID); err != nil {
			t.Fatalf("order: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO order_item (order_id, item_id, quantity, unit_weight_kg_snapshot, unit_volume_m3_snapshot, total_weight_kg, total_volume_m3)
			VALUES ($1, $2, 1, 1, 0.01, 1, 0.01) RETURNING order_item_id`, orderID, itemID).Scan(&orderItemID); err != nil {
			t.Fatalf("order item: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
			VALUES ('VEHA1', 'TRUCK', 'AMBIENT', 5000, 20, 'diesel', 5, 400, $1)
			ON CONFLICT (vehicle_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
			t.Fatalf("vehicle: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO route (vehicle_id, depot_id, route_date, trip_no, brand, district, status)
			VALUES ('VEHA1', $1, '2026-09-26', 1, 'FRESH', 'Colombo', 'CONFIRMED')
			ON CONFLICT (vehicle_id, route_date, trip_no) DO UPDATE SET status = 'CONFIRMED' RETURNING route_id`, depotID).Scan(&routeID); err != nil {
			t.Fatalf("route: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO route_leg (route_id, order_id, seq, from_point, to_outlet, status)
			VALUES ($1, $2, 0, 'DEPOT', 'AUTHO1', 'PENDING')
			ON CONFLICT (route_id, seq) DO UPDATE SET order_id = EXCLUDED.order_id RETURNING leg_id`, routeID, orderID).Scan(&legID); err != nil {
			t.Fatalf("leg: %v", err)
		}

		depot, outlet, found, err := s.OrderItemScope(ctx, orderItemID)
		if err != nil || !found || depot != depotID || outlet != "AUTHO1" {
			t.Fatalf("OrderItemScope = %q %q %v %v", depot, outlet, found, err)
		}
		depot, outlet, found, err = s.LegScope(ctx, legID)
		if err != nil || !found || depot != depotID || outlet != "AUTHO1" {
			t.Fatalf("LegScope = %q %q %v %v", depot, outlet, found, err)
		}
		if _, _, found, err := s.LegScope(ctx, "00000000-0000-0000-0000-000000000000"); err != nil || found {
			t.Fatalf("missing leg found=%v err=%v, want false/nil", found, err)
		}
	})
}
