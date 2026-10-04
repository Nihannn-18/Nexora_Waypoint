package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The demo day is Task 2B scenario S1: orders are placed on Fri 25 Sep 2026 for
// delivery on Sat 26 Sep 2026. These are facts of the supplied scenario, not
// configuration.
const (
	demoScenario     = "S1"
	demoOrderDate    = "2026-09-25"
	demoDeliveryDate = "2026-09-26"
)

// seedDemoDay loads the S1 fleet availability and orders. Orders arrive as
// aggregates, so each gets one line whose totals are the CSV values verbatim.
func seedDemoDay(ctx context.Context, tx pgx.Tx) (orders, availability int, err error) {
	if availability, err = seedDemoAvailability(ctx, tx); err != nil {
		return 0, 0, err
	}
	if orders, err = seedDemoOrders(ctx, tx); err != nil {
		return 0, 0, err
	}
	return orders, availability, nil
}

// seedDemoAvailability writes one row per vehicle for the delivery day. The
// fleet file lists only some vehicles; any vehicle it omits is available, since
// the file records exceptions to a fully available fleet.
func seedDemoAvailability(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := readCSV("data/task2b_peak_day_fleet.csv")
	if err != nil {
		return 0, err
	}

	status := make(map[string]string, len(rows))
	for _, r := range rows {
		if r["scenario"] != demoScenario {
			return 0, fmt.Errorf("fleet row for %s has scenario %q, want %s", r["vehicle_id"], r["scenario"], demoScenario)
		}
		s, err := enum(r["status"])
		if err != nil {
			return 0, fmt.Errorf("fleet %s status: %w", r["vehicle_id"], err)
		}
		if s != "AVAILABLE" && s != "IN_WORKSHOP" {
			return 0, fmt.Errorf("fleet %s has unsupported status %q", r["vehicle_id"], r["status"])
		}
		status[r["vehicle_id"]] = s
	}

	// Insert for every vehicle; an unknown vehicle_id in the file is caught by
	// the check below rather than silently ignored.
	tag, err := tx.Exec(ctx, `
		INSERT INTO vehicle_daily_availability (vehicle_id, date, available, status, reason)
		SELECT v.vehicle_id, $1::date, COALESCE(f.status, 'AVAILABLE') = 'AVAILABLE',
		       COALESCE(f.status, 'AVAILABLE'),
		       CASE WHEN f.status = 'IN_WORKSHOP' THEN 'In workshop (Task 2B S1)' END
		FROM vehicle v
		LEFT JOIN (SELECT key AS vehicle_id, value AS status FROM jsonb_each_text($2::jsonb)) f
		       ON f.vehicle_id = v.vehicle_id
		ON CONFLICT (vehicle_id, date) DO UPDATE
		SET available = EXCLUDED.available, status = EXCLUDED.status, reason = EXCLUDED.reason`,
		demoDeliveryDate, jsonMap(status))
	if err != nil {
		return 0, fmt.Errorf("seed vehicle availability: %w", err)
	}

	var known int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM vehicle WHERE vehicle_id = ANY($1)`, keys(status)).Scan(&known); err != nil {
		return 0, fmt.Errorf("check fleet vehicle ids: %w", err)
	}
	if known != len(status) {
		return 0, fmt.Errorf("fleet file names %d vehicles, only %d exist", len(status), known)
	}
	return int(tag.RowsAffected()), nil
}

// seedDemoOrders inserts each scenario order once. An existing order number is
// left untouched, so re-seeding never rewinds an order that has since moved
// through the lifecycle.
//
// It returns the number of S1 rows read (the scenario's order count), not the
// number newly inserted: on a re-run every row conflicts and nothing is written,
// yet the count stays stable so callers and the idempotency test can compare
// runs. The database row count is asserted separately where it matters.
func seedDemoOrders(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := readCSV("data/task2b_peak_day_scenarios.csv")
	if err != nil {
		return 0, err
	}

	itemIDs := map[string]string{}
	for _, r := range rows {
		if r["scenario"] != demoScenario {
			return 0, fmt.Errorf("order %s has scenario %q, want %s", r["order_ref"], r["scenario"], demoScenario)
		}
		brand, err := enum(r["brand"])
		if err != nil {
			return 0, fmt.Errorf("order %s brand: %w", r["order_ref"], err)
		}
		temp, err := enum(r["temp_requirement"])
		if err != nil {
			return 0, fmt.Errorf("order %s temperature: %w", r["order_ref"], err)
		}

		itemID, err := demoItem(ctx, tx, itemIDs, brand, temp)
		if err != nil {
			return 0, err
		}

		var orderID string
		err = tx.QueryRow(ctx, `
			INSERT INTO customer_order (
				order_number, outlet_id, brand, order_date, requested_delivery_date,
				total_units, total_weight_kg, total_volume_m3, temp_requirement,
				status, deferred_yesterday, days_since_last_served, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'CONFIRMED',$10,$11,
				($4::date)::timestamp AT TIME ZONE 'UTC', ($4::date)::timestamp AT TIME ZONE 'UTC')
			ON CONFLICT (order_number) DO NOTHING
			RETURNING order_id`,
			r["order_ref"], r["outlet_id"], brand, demoOrderDate, demoDeliveryDate,
			r["order_units"], r["order_weight_kg"], r["order_volume_m3"], temp,
			boolCSV(r["deferred_yesterday"]), r["days_since_last_served"]).Scan(&orderID)
		if err == pgx.ErrNoRows {
			continue // already seeded
		}
		if err != nil {
			return 0, fmt.Errorf("seed order %s: %w", r["order_ref"], err)
		}

		// The snapshot unit values are derived from the order totals; the line
		// totals are the CSV values, never recomputed from them.
		_, err = tx.Exec(ctx, `
			INSERT INTO order_item (
				order_id, item_id, quantity, unit_weight_kg_snapshot,
				unit_volume_m3_snapshot, total_weight_kg, total_volume_m3
			) VALUES ($1, $2, $3::int, round($4::numeric / $3::int, 3), round($5::numeric / $3::int, 4), $4::numeric, $5::numeric)`,
			orderID, itemID, r["order_units"], r["order_weight_kg"], r["order_volume_m3"])
		if err != nil {
			return 0, fmt.Errorf("seed order line for %s: %w", r["order_ref"], err)
		}
	}
	return len(rows), nil
}

// demoItem returns the aggregate SKU for a brand and temperature, creating it
// on first use. These are stand-ins: the scenario carries no real SKUs.
func demoItem(ctx context.Context, tx pgx.Tx, cache map[string]string, brand, temp string) (string, error) {
	sku := "DEMO-" + brand + "-" + temp
	if id, ok := cache[sku]; ok {
		return id, nil
	}
	name := fmt.Sprintf("Demo aggregate (%s, %s)", brand[:1]+strings.ToLower(brand[1:]), strings.ToLower(temp))
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO item (sku, name, brand, unit_weight_kg, unit_volume_m3, temperature_requirement)
		VALUES ($1, $2, $3, 1, 0.01, $4)
		ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name
		RETURNING item_id`, sku, name, brand, temp).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("seed demo item %s: %w", sku, err)
	}
	cache[sku] = id
	return id, nil
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func jsonMap(m map[string]string) string {
	b, _ := json.Marshal(m) // a map of strings always marshals
	return string(b)
}
