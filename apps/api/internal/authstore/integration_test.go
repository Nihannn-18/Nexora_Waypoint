package authstore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/store"
)

// TestAuthStoreIntegration exercises the Better Auth -> app_user resolution
// against a real PostgreSQL when one is reachable, and skips otherwise. It
// proves session verification, email mapping, expiry, and media owner scope.
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

	// Remove this test's rows even if it fails, so a strict seed test sharing the
	// database still sees exactly the seeded reference rows.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, stmt := range []string{
			`DELETE FROM "session" WHERE "id" LIKE 'bs-%'`,
			`DELETE FROM "user" WHERE "id" LIKE 'ba-%'`,
			`DELETE FROM app_user WHERE user_id LIKE 'auth-%'`,
			`DELETE FROM route_leg WHERE to_outlet = 'OUTA1'`,
			`DELETE FROM route WHERE vehicle_id = 'VEHA1'`,
			`DELETE FROM order_item WHERE order_id IN (SELECT order_id FROM customer_order WHERE order_number = 'AUTH-ORD-1')`,
			`DELETE FROM customer_order WHERE order_number = 'AUTH-ORD-1'`,
			`DELETE FROM outlet WHERE outlet_id = 'OUTA1'`,
			`DELETE FROM item WHERE sku = 'AUTH-SKU-1'`,
			`DELETE FROM vehicle WHERE vehicle_id = 'VEHA1'`,
			`DELETE FROM depot WHERE code = 'ATHDEPOT'`,
		} {
			_, _ = pool.Exec(cleanup, stmt)
		}
	})

	var depotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('ATHDEPOT', 'Auth Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type, parking_constraint, window_open_time, window_close_time)
		VALUES ('OUTA1', 'Auth Outlet', 'FRESH', 'Colombo', $1, 'STREET', 'NORMAL', '05:00', '08:00')
		ON CONFLICT (outlet_id) DO NOTHING`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO app_user (user_id, email, role, depot_id, is_active)
		VALUES ('auth-dispatcher', 'auth.dispatcher@waypoint.lk', 'DISPATCHER', $1, true)
		ON CONFLICT (user_id) DO UPDATE SET email = EXCLUDED.email, role = EXCLUDED.role, depot_id = EXCLUDED.depot_id, is_active = true`, depotID); err != nil {
		t.Fatalf("dispatcher app_user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO app_user (user_id, email, role, outlet_id, is_active)
		VALUES ('auth-store', 'auth.store@waypoint.lk', 'STORE_MANAGER', 'OUTA1', true)
		ON CONFLICT (user_id) DO UPDATE SET email = EXCLUDED.email, role = EXCLUDED.role, outlet_id = EXCLUDED.outlet_id, is_active = true`); err != nil {
		t.Fatalf("store app_user: %v", err)
	}

	baUser := func(id, email, name string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO "user" ("id", "name", "email", "emailVerified", "createdAt", "updatedAt")
			VALUES ($1, $2, $3, true, now(), now())
			ON CONFLICT ("id") DO UPDATE SET "email" = EXCLUDED."email", "name" = EXCLUDED."name"`, id, name, email); err != nil {
			t.Fatalf("better auth user %s: %v", id, err)
		}
	}
	baSession := func(id, token, userID string, expiresAt time.Time) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO "session" ("id", "expiresAt", "token", "createdAt", "updatedAt", "userId")
			VALUES ($1, $2, $3, now(), now(), $4)
			ON CONFLICT ("id") DO UPDATE SET "expiresAt" = EXCLUDED."expiresAt", "token" = EXCLUDED."token"`, id, expiresAt, token, userID); err != nil {
			t.Fatalf("better auth session %s: %v", id, err)
		}
	}

	baUser("ba-dispatcher", "auth.dispatcher@waypoint.lk", "Auth Dispatcher")
	baUser("ba-store", "auth.store@waypoint.lk", "Auth Store")
	baUser("ba-orphan", "auth.orphan@waypoint.lk", "Orphan")
	baSession("bs-dispatcher", "token-dispatcher", "ba-dispatcher", time.Now().Add(time.Hour))
	baSession("bs-store", "token-store", "ba-store", time.Now().Add(time.Hour))
	baSession("bs-expired", "token-expired", "ba-dispatcher", time.Now().Add(-time.Hour))
	baSession("bs-orphan", "token-orphan", "ba-orphan", time.Now().Add(time.Hour))

	t.Run("valid session maps to app_user by email", func(t *testing.T) {
		userID, email, name, err := s.VerifySession(ctx, "token-dispatcher")
		if err != nil {
			t.Fatalf("VerifySession: %v", err)
		}
		if userID != "auth-dispatcher" || email != "auth.dispatcher@waypoint.lk" || name != "Auth Dispatcher" {
			t.Fatalf("got %q %q %q", userID, email, name)
		}
	})

	t.Run("unknown token is invalid session", func(t *testing.T) {
		if _, _, _, err := s.VerifySession(ctx, "nope"); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("expired session is invalid", func(t *testing.T) {
		if _, _, _, err := s.VerifySession(ctx, "token-expired"); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("valid session without app_user is forbidden", func(t *testing.T) {
		if _, _, _, err := s.VerifySession(ctx, "token-orphan"); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("LoadIdentity returns role and scope", func(t *testing.T) {
		id, err := s.LoadIdentity(ctx, "auth-store")
		if err != nil {
			t.Fatalf("LoadIdentity: %v", err)
		}
		if id.Role != domain.RoleStoreManager || id.OutletID != "OUTA1" || id.DepotID != "" {
			t.Fatalf("identity = %+v", id)
		}
	})

	t.Run("inactive app_user is forbidden", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO app_user (user_id, email, role, depot_id, is_active)
			VALUES ('auth-inactive', 'auth.inactive@waypoint.lk', 'LOADER', $1, false)
			ON CONFLICT (user_id) DO UPDATE SET is_active = false`, depotID); err != nil {
			t.Fatalf("inactive user: %v", err)
		}
		if _, err := s.LoadIdentity(ctx, "auth-inactive"); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
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
			VALUES ('AUTH-ORD-1', 'OUTA1', 'FRESH', '2026-09-25', '2026-09-26', 1, 1, 0.01, 'AMBIENT', 'ALLOCATED')
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
			VALUES ($1, $2, 0, 'DEPOT', 'OUTA1', 'PENDING')
			ON CONFLICT (route_id, seq) DO UPDATE SET order_id = EXCLUDED.order_id RETURNING leg_id`, routeID, orderID).Scan(&legID); err != nil {
			t.Fatalf("leg: %v", err)
		}

		depot, outlet, found, err := s.OrderItemScope(ctx, orderItemID)
		if err != nil || !found || depot != depotID || outlet != "OUTA1" {
			t.Fatalf("OrderItemScope = %q %q %v %v", depot, outlet, found, err)
		}
		depot, outlet, found, err = s.LegScope(ctx, legID)
		if err != nil || !found || depot != depotID || outlet != "OUTA1" {
			t.Fatalf("LegScope = %q %q %v %v", depot, outlet, found, err)
		}
		if _, _, found, err := s.LegScope(ctx, "00000000-0000-0000-0000-000000000000"); err != nil || found {
			t.Fatalf("missing leg found=%v err=%v, want false/nil", found, err)
		}
	})
}
