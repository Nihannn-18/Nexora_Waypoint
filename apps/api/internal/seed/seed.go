// Package seed loads the supplied reference CSVs into PostgreSQL.
//
// The CSV files are embedded in the binary with go:embed, so the API image
// needs no data volume. Seeding runs at start-up after migrations and is
// idempotent: every insert is an upsert keyed on the natural identifier, so
// running it twice changes nothing and never duplicates a row.
//
// It seeds reference data, the four demo accounts' role and scope rows, and the
// demo day (Task 2B scenario S1: its orders and fleet availability).
// traffic_speed and road_condition are Datathon-only and are never seeded.
package seed

import (
	"context"
	"embed"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/password"
)

//go:embed data/*.csv
var dataFS embed.FS

// Depots are fixed reference facts, not supplied in the CSVs. The outlet and
// vehicle files reference them by name.
var depots = []struct {
	code string
	name string
}{
	{"PELIYAGODA", "Peliyagoda"},
	{"KANDY", "Kandy"},
}

// Result reports how many rows each dataset contributed, for logging and tests.
// DemoOrders counts the scenario's order rows read (stable across re-runs even
// when nothing is inserted); DemoAvailability counts vehicle-days written.
type Result struct {
	Depots           int
	Outlets          int
	Vehicles         int
	DistrictTravel   int
	ServiceAllowance int
	CalendarDays     int
	Users            int
	DemoOrders       int
	DemoAvailability int
	DemoAssignments  int
}

// Run seeds every reference dataset in foreign-key order: depots first, then
// outlets and vehicles, then the tables that reference them.
func Run(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	var res Result

	tx, err := pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("begin seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	depotIDs, err := seedDepots(ctx, tx)
	if err != nil {
		return res, err
	}
	res.Depots = len(depotIDs)

	if res.Outlets, err = seedOutlets(ctx, tx, depotIDs); err != nil {
		return res, err
	}
	if res.Vehicles, err = seedVehicles(ctx, tx, depotIDs); err != nil {
		return res, err
	}
	if res.DistrictTravel, err = seedDistrictTravel(ctx, tx, depotIDs); err != nil {
		return res, err
	}
	if res.ServiceAllowance, err = seedServiceAllowance(ctx, tx); err != nil {
		return res, err
	}
	if res.CalendarDays, err = seedCalendar(ctx, tx); err != nil {
		return res, err
	}
	if err := seedDemoCalendar(ctx, tx); err != nil {
		return res, err
	}

	if res.Users, err = seedUsers(ctx, tx, depotIDs); err != nil {
		return res, err
	}
	if res.DemoAssignments, err = seedDemoAssignment(ctx, tx); err != nil {
		return res, err
	}
	if res.DemoOrders, res.DemoAvailability, err = seedDemoDay(ctx, tx); err != nil {
		return res, err
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("commit seed: %w", err)
	}
	return res, nil
}

// demoUsers are the four seeded accounts. Their passwords are hashed with
// Argon2id and stored in app_user.password_hash; the plaintext comes from
// DEMO_SEED_PASSWORD (default "waypoint2026") and is never stored or logged.
// user_id is a stable placeholder; a re-seed never rewrites it on an existing
// email.
var demoUsers = []struct {
	userID   string
	email    string
	name     string
	role     string
	depot    string // depot name, empty when the user is outlet-scoped
	outletID string
}{
	{"seed-dispatcher", "priyantha.w@waypoint.lk", "Priyantha W.", "DISPATCHER", "Peliyagoda", ""},
	{"seed-loader", "nadeesha.p@waypoint.lk", "Nadeesha P.", "LOADER", "Peliyagoda", ""},
	{"seed-driver", "kasun.p@waypoint.lk", "Kasun P.", "DRIVER", "Peliyagoda", ""},
	{"seed-store-manager", "ishara.s@waypoint.lk", "Ishara S.", "STORE_MANAGER", "", "OUT014"},
}

// defaultDemoPassword is the seeded password when DEMO_SEED_PASSWORD is unset.
// It is a demo credential, documented in the README; never use it in a
// production-like deployment with real data.
const defaultDemoPassword = "waypoint2026"

func seedUsers(ctx context.Context, tx pgx.Tx, depotIDs map[string]string) (int, error) {
	demoPassword := os.Getenv("DEMO_SEED_PASSWORD")
	if demoPassword == "" {
		demoPassword = defaultDemoPassword
	}
	passwordHash, err := password.Hash(demoPassword)
	if err != nil {
		return 0, fmt.Errorf("hash demo password: %w", err)
	}

	for _, u := range demoUsers {
		var depotID *string
		if u.depot != "" {
			id, ok := depotIDs[u.depot]
			if !ok {
				return 0, fmt.Errorf("seed user %s: unknown depot %q", u.email, u.depot)
			}
			depotID = &id
		}
		var outletID *string
		if u.outletID != "" {
			outletID = &u.outletID
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO app_user (user_id, email, display_name, role, depot_id, outlet_id, password_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (email) DO UPDATE
			SET display_name = EXCLUDED.display_name,
			    role = EXCLUDED.role,
			    depot_id = EXCLUDED.depot_id,
			    outlet_id = EXCLUDED.outlet_id,
			    password_hash = EXCLUDED.password_hash`,
			u.userID, u.email, u.name, u.role, depotID, outletID, passwordHash)
		if err != nil {
			return 0, fmt.Errorf("seed user %s: %w", u.email, err)
		}
	}
	return len(demoUsers), nil
}

// seedDemoAssignment gives the demo day a deterministic driver-to-vehicle
// assignment: the seeded driver is on VEH014 for the S1 delivery date. It uses
// the vehicle's own depot so the row is always depot-consistent, and DO NOTHING
// on conflict so a dispatcher's later change is never rewound by a re-seed.
// The seeded demo can therefore show the driver cockpit resolving VEH014's run.
func seedDemoAssignment(ctx context.Context, tx pgx.Tx) (int, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user WHERE user_id = 'seed-driver')`).Scan(&exists); err != nil {
		return 0, fmt.Errorf("seed demo assignment: check driver: %w", err)
	}
	if !exists {
		return 0, fmt.Errorf("seed demo assignment: seeded driver is missing")
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vehicle WHERE vehicle_id = 'VEH014')`).Scan(&exists); err != nil {
		return 0, fmt.Errorf("seed demo assignment: check vehicle: %w", err)
	}
	if !exists {
		return 0, fmt.Errorf("seed demo assignment: VEH014 is missing from the fleet")
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO driver_vehicle_assignment (driver_id, vehicle_id, assignment_date, depot_id, assigned_by)
		SELECT 'seed-driver', v.vehicle_id, $1::date, v.depot_id, 'seed-dispatcher'
		FROM vehicle v
		WHERE v.vehicle_id = 'VEH014'
		ON CONFLICT (vehicle_id, assignment_date) DO NOTHING`, demoDeliveryDate)
	if err != nil {
		return 0, fmt.Errorf("seed demo assignment: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func seedDepots(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	ids := make(map[string]string, len(depots))
	for _, d := range depots {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO depot (code, name)
			VALUES ($1, $2)
			ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
			RETURNING depot_id`,
			d.code, d.name).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("seed depot %s: %w", d.code, err)
		}
		ids[d.name] = id
	}
	return ids, nil
}

func seedOutlets(ctx context.Context, tx pgx.Tx, depotIDs map[string]string) (int, error) {
	rows, err := readCSV("data/outlets.csv")
	if err != nil {
		return 0, err
	}

	n := 0
	for _, r := range rows {
		outletID := r["outlet_id"]
		brand, err := enum(r["brand"])
		if err != nil {
			return 0, fmt.Errorf("outlet %s brand: %w", outletID, err)
		}
		dockType, err := enum(r["dock_type"])
		if err != nil {
			return 0, fmt.Errorf("outlet %s dock_type: %w", outletID, err)
		}
		parking, err := enum(r["parking_constraint"])
		if err != nil {
			return 0, fmt.Errorf("outlet %s parking_constraint: %w", outletID, err)
		}
		depotID, ok := depotIDs[r["depot"]]
		if !ok {
			return 0, fmt.Errorf("outlet %s has unknown depot %q", outletID, r["depot"])
		}

		var mallOpen, mallClose *time.Time
		if w := strings.TrimSpace(r["mall_window"]); w != "" {
			o, c, err := parseWindow(w)
			if err != nil {
				return 0, fmt.Errorf("outlet %s mall_window %q: %w", outletID, w, err)
			}
			mallOpen, mallClose = &o, &c
		}

		open, err := parseClock(r["window_open_time"])
		if err != nil {
			return 0, fmt.Errorf("outlet %s window_open_time: %w", outletID, err)
		}
		close_, err := parseClock(r["window_close_time"])
		if err != nil {
			return 0, fmt.Errorf("outlet %s window_close_time: %w", outletID, err)
		}

		// The CSVs carry no outlet name; the deterministic display name is the
		// identifier itself (OUT001 … OUT120). IDs are the identifiers
		// everywhere; the name is display-only.
		name := "Outlet " + outletID

		_, err = tx.Exec(ctx, `
			INSERT INTO outlet (
				outlet_id, name, brand, district, depot_id, dock_type,
				parking_constraint, mall_window_open, mall_window_close,
				window_open_time, window_close_time
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (outlet_id) DO UPDATE SET
				name = EXCLUDED.name, brand = EXCLUDED.brand,
				district = EXCLUDED.district, depot_id = EXCLUDED.depot_id,
				dock_type = EXCLUDED.dock_type,
				parking_constraint = EXCLUDED.parking_constraint,
				mall_window_open = EXCLUDED.mall_window_open,
				mall_window_close = EXCLUDED.mall_window_close,
				window_open_time = EXCLUDED.window_open_time,
				window_close_time = EXCLUDED.window_close_time`,
			outletID, name, brand, r["district"], depotID, dockType,
			parking, mallOpen, mallClose, open, close_)
		if err != nil {
			return 0, fmt.Errorf("seed outlet %s: %w", outletID, err)
		}
		n++
	}
	return n, nil
}

func seedVehicles(ctx context.Context, tx pgx.Tx, depotIDs map[string]string) (int, error) {
	rows, err := readCSV("data/vehicles.csv")
	if err != nil {
		return 0, err
	}

	n := 0
	for _, r := range rows {
		vehicleID := r["vehicle_id"]
		typ, err := enum(r["type"])
		if err != nil {
			return 0, fmt.Errorf("vehicle %s type: %w", vehicleID, err)
		}
		temp, err := enum(r["temp"])
		if err != nil {
			return 0, fmt.Errorf("vehicle %s temp: %w", vehicleID, err)
		}
		depotID, ok := depotIDs[r["depot"]]
		if !ok {
			return 0, fmt.Errorf("vehicle %s has unknown depot %q", vehicleID, r["depot"])
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO vehicle (
				vehicle_id, type, temp, weight_cap_kg, volume_cap_m3,
				fuel_type, km_per_l, weekly_fuel_quota_l, depot_id
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (vehicle_id) DO UPDATE SET
				type = EXCLUDED.type, temp = EXCLUDED.temp,
				weight_cap_kg = EXCLUDED.weight_cap_kg,
				volume_cap_m3 = EXCLUDED.volume_cap_m3,
				fuel_type = EXCLUDED.fuel_type, km_per_l = EXCLUDED.km_per_l,
				weekly_fuel_quota_l = EXCLUDED.weekly_fuel_quota_l,
				depot_id = EXCLUDED.depot_id`,
			vehicleID, typ, temp, r["weight_cap_kg"], r["volume_cap_m3"],
			r["fuel_type"], r["km_per_l"], r["weekly_fuel_quota_l"], depotID)
		if err != nil {
			return 0, fmt.Errorf("seed vehicle %s: %w", vehicleID, err)
		}
		n++
	}
	return n, nil
}

func seedDistrictTravel(ctx context.Context, tx pgx.Tx, depotIDs map[string]string) (int, error) {
	rows, err := readCSV("data/district_travel.csv")
	if err != nil {
		return 0, err
	}

	n := 0
	for _, r := range rows {
		depotID, ok := depotIDs[r["depot"]]
		if !ok {
			return 0, fmt.Errorf("district_travel %s has unknown depot %q", r["district"], r["depot"])
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO district_travel (
				district, depot_id, road_class, free_flow_kmh,
				depot_to_district_km, depot_to_district_freeflow_min,
				inter_stop_km, inter_stop_freeflow_min
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (district, depot_id) DO UPDATE SET
				road_class = EXCLUDED.road_class,
				free_flow_kmh = EXCLUDED.free_flow_kmh,
				depot_to_district_km = EXCLUDED.depot_to_district_km,
				depot_to_district_freeflow_min = EXCLUDED.depot_to_district_freeflow_min,
				inter_stop_km = EXCLUDED.inter_stop_km,
				inter_stop_freeflow_min = EXCLUDED.inter_stop_freeflow_min`,
			r["district"], depotID, r["road_class"], r["free_flow_kmh"],
			r["depot_to_district_km"], r["depot_to_district_freeflow_min"],
			r["inter_stop_km"], r["inter_stop_freeflow_min"])
		if err != nil {
			return 0, fmt.Errorf("seed district_travel %s/%s: %w", r["district"], r["depot"], err)
		}
		n++
	}
	return n, nil
}

func seedServiceAllowance(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := readCSV("data/service_allowance.csv")
	if err != nil {
		return 0, err
	}

	n := 0
	for _, r := range rows {
		brand, err := enum(r["brand"])
		if err != nil {
			return 0, fmt.Errorf("service_allowance brand: %w", err)
		}
		dockType, err := enum(r["dock_type"])
		if err != nil {
			return 0, fmt.Errorf("service_allowance dock_type: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO service_allowance (brand, dock_type, service_allowance_min)
			VALUES ($1,$2,$3)
			ON CONFLICT (brand, dock_type) DO UPDATE SET
				service_allowance_min = EXCLUDED.service_allowance_min`,
			brand, dockType, r["service_allowance_min"])
		if err != nil {
			return 0, fmt.Errorf("seed service_allowance %s/%s: %w", brand, dockType, err)
		}
		n++
	}
	return n, nil
}

func seedCalendar(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := readCSV("data/calendar.csv")
	if err != nil {
		return 0, err
	}

	n := 0
	for _, r := range rows {
		date, err := time.Parse("2006-01-02", r["date"])
		if err != nil {
			return 0, fmt.Errorf("calendar date %q: %w", r["date"], err)
		}
		var festival *string
		if f := strings.TrimSpace(r["festival"]); f != "" {
			festival = &f
		}
		var ramp *string
		if v := strings.TrimSpace(r["festival_ramp"]); v != "" {
			ramp = &v
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO calendar_day (
				date, dow, dow_name, is_weekend, iso_year, iso_week,
				is_payday, festival, festival_ramp, is_holiday, monsoon, is_operating
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (date) DO UPDATE SET
				dow = EXCLUDED.dow, dow_name = EXCLUDED.dow_name,
				is_weekend = EXCLUDED.is_weekend, iso_year = EXCLUDED.iso_year,
				iso_week = EXCLUDED.iso_week, is_payday = EXCLUDED.is_payday,
				festival = EXCLUDED.festival, festival_ramp = EXCLUDED.festival_ramp,
				is_holiday = EXCLUDED.is_holiday, monsoon = EXCLUDED.monsoon,
				is_operating = EXCLUDED.is_operating`,
			date, r["dow"], r["dow_name"], boolCSV(r["is_weekend"]),
			r["iso_year"], r["iso_week"], boolCSV(r["is_payday"]),
			festival, ramp, boolCSV(r["is_holiday"]),
			boolCSV(r["monsoon"]), boolCSV(r["is_operating"]))
		if err != nil {
			return 0, fmt.Errorf("seed calendar %s: %w", r["date"], err)
		}
		n++
	}
	return n, nil
}

// seedDemoCalendar guarantees the demo day is operable even though the supplied
// calendar.csv ends on 2026-06-28. It does not edit the supplied file; it
// derives the missing operating-day facts from the weekday using the same
// convention the file records (Monday–Saturday operating, Sunday not) and
// inserts them only when the date is absent, so a supplied row always wins.
//
// The demo window is the S1 order day and delivery day, plus the surrounding
// days so a late order rolled to the next operating day, or a follow-up
// planning run, still finds operating calendar facts.
func seedDemoCalendar(ctx context.Context, tx pgx.Tx) error {
	orderDate, err := time.Parse("2006-01-02", demoOrderDate)
	if err != nil {
		return fmt.Errorf("demo order date: %w", err)
	}
	deliveryDate, err := time.Parse("2006-01-02", demoDeliveryDate)
	if err != nil {
		return fmt.Errorf("demo delivery date: %w", err)
	}

	// Seed from the order day through the following week so the demo's "next
	// operating day" resolution always lands on a real calendar row.
	days := make([]time.Time, 0, 9)
	for d := orderDate; !d.After(deliveryDate.AddDate(0, 0, 7)); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}

	for _, d := range days {
		isoYear, isoWeek := d.ISOWeek()
		isWeekend := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
		// The supplied file marks Saturday operating and Sunday not: operations
		// run Monday–Saturday. Derive the same rule, never a weekday assumption
		// baked into business logic (the calendar row remains the authority).
		isOperating := demoOperatingDay(d)
		_, err := tx.Exec(ctx, `
			INSERT INTO calendar_day (
				date, dow, dow_name, is_weekend, iso_year, iso_week,
				is_payday, festival, festival_ramp, is_holiday, monsoon, is_operating
			) VALUES ($1,$2,$3,$4,$5,$6,FALSE,NULL,NULL,FALSE,FALSE,$7)
			ON CONFLICT (date) DO NOTHING`,
			d.Format("2006-01-02"), int(d.Weekday()), d.Weekday().String()[:3],
			isWeekend, isoYear, isoWeek, isOperating)
		if err != nil {
			return fmt.Errorf("seed demo calendar %s: %w", d.Format("2006-01-02"), err)
		}
	}
	return nil
}

// demoOperatingDay reports whether a derived demo-window date is an operating
// day. Operations run Monday–Saturday, matching the convention the supplied
// calendar.csv records (every Sunday is is_operating=0). Kept as one named
// function so the rule is unit-tested and never scattered.
func demoOperatingDay(d time.Time) bool {
	return d.Weekday() != time.Sunday
}

// readCSV reads an embedded CSV into a slice of header-keyed maps, dropping a
// leading UTF-8 BOM if present.
func readCSV(name string) ([]map[string]string, error) {
	f, err := dataFS.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open embedded %s: %w", name, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header of %s: %w", name, err)
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}

	var out []map[string]string
	line := 1
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", name, line, err)
		}
		if len(rec) != len(header) {
			return nil, fmt.Errorf("%s line %d: expected %d fields, got %d", name, line, len(header), len(rec))
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			row[h] = rec[i]
		}
		out = append(out, row)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%s contains no data rows", name)
	}
	return out, nil
}

// enum converts a CSV lower_snake value to its UPPER_SNAKE wire form.
func enum(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("empty enum value")
	}
	return strings.ToUpper(v), nil
}

// boolCSV interprets a CSV flag that may be 0/1 or true/false.
func boolCSV(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes":
		return true
	default:
		return false
	}
}

// parseClock parses an HH:MM wall-clock time into a time.Time anchored to the
// zero date; only the clock portion is stored.
func parseClock(v string) (time.Time, error) {
	return time.Parse("15:04", strings.TrimSpace(v))
}

// parseWindow splits a "HH:MM-HH:MM" mall window into its open and close times.
func parseWindow(v string) (time.Time, time.Time, error) {
	parts := strings.SplitN(strings.TrimSpace(v), "-", 2)
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("expected HH:MM-HH:MM")
	}
	open, err := parseClock(parts[0])
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	close_, err := parseClock(parts[1])
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return open, close_, nil
}
