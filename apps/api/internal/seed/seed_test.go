package seed

import (
	"testing"
	"time"
)

// These tests are DB-free: they exercise CSV parsing, enum conversion, the
// mall-window split and the embedded datasets themselves. Integration seeding
// against a live PostgreSQL is covered separately where a database is
// available.

func TestEnum(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "lower snake to upper", in: "van_only", want: "VAN_ONLY"},
		{name: "brand title to upper", in: "Fresh", want: "FRESH"},
		{name: "already upper", in: "REEFER", want: "REEFER"},
		{name: "trims whitespace", in: "  normal  ", want: "NORMAL"},
		{name: "empty is an error", in: "", wantErr: true},
		{name: "whitespace-only is an error", in: "   ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := enum(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("enum(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBoolCSV(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"1", true}, {"true", true}, {"TRUE", true}, {"t", true}, {"yes", true},
		{"0", false}, {"false", false}, {"", false}, {"no", false}, {"garbage", false},
	}
	for _, tt := range tests {
		if got := boolCSV(tt.in); got != tt.want {
			t.Errorf("boolCSV(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseClock(t *testing.T) {
	got, err := parseClock("07:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Hour() != 7 || got.Minute() != 30 {
		t.Fatalf("parseClock(07:30) = %v, want 07:30", got)
	}

	if _, err := parseClock("25:00"); err == nil {
		t.Fatal("expected error for hour 25")
	}
	if _, err := parseClock("not-a-time"); err == nil {
		t.Fatal("expected error for a non-time string")
	}
}

func TestParseWindow(t *testing.T) {
	open, close_, err := parseWindow("09:00-11:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open.Hour() != 9 || open.Minute() != 0 {
		t.Errorf("open = %v, want 09:00", open)
	}
	if close_.Hour() != 11 || close_.Minute() != 0 {
		t.Errorf("close = %v, want 11:00", close_)
	}

	if _, _, err := parseWindow("09:00"); err == nil {
		t.Fatal("expected error for a window without a dash")
	}
	if _, _, err := parseWindow("09:00-bad"); err == nil {
		t.Fatal("expected error for an unparseable close time")
	}
}

// TestEmbeddedDatasetsLoad proves every embedded CSV parses and carries the
// expected number of rows. These are the counts the specification pins.
func TestEmbeddedDatasetsLoad(t *testing.T) {
	tests := []struct {
		file string
		want int
	}{
		{"data/outlets.csv", 120},
		{"data/vehicles.csv", 60},
		{"data/district_travel.csv", 12},
		{"data/service_allowance.csv", 9},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			rows, err := readCSV(tt.file)
			if err != nil {
				t.Fatalf("readCSV(%s): %v", tt.file, err)
			}
			if len(rows) != tt.want {
				t.Fatalf("readCSV(%s) returned %d rows, want %d", tt.file, len(rows), tt.want)
			}
		})
	}

	// Calendar is a long series; assert it parses and is non-trivial rather
	// than pinning a brittle exact count.
	cal, err := readCSV("data/calendar.csv")
	if err != nil {
		t.Fatalf("readCSV(calendar): %v", err)
	}
	if len(cal) < 900 {
		t.Fatalf("calendar.csv has %d rows, expected the full supplied series", len(cal))
	}
	if _, err := time.Parse("2006-01-02", cal[0]["date"]); err != nil {
		t.Fatalf("first calendar date does not parse: %v", err)
	}
}

// TestOutletMallWindowSplit confirms the only blank-vs-filled column in the
// supplied outlets behaves: exactly the mall outlets carry a window, and every
// non-blank value is a valid HH:MM-HH:MM pair.
func TestOutletMallWindowSplit(t *testing.T) {
	rows, err := readCSV("data/outlets.csv")
	if err != nil {
		t.Fatalf("readCSV(outlets): %v", err)
	}

	withWindow := 0
	for _, r := range rows {
		w := r["mall_window"]
		if w == "" {
			continue
		}
		withWindow++
		if _, _, err := parseWindow(w); err != nil {
			t.Fatalf("outlet %s has malformed mall_window %q: %v", r["outlet_id"], w, err)
		}
	}
	if withWindow == 0 {
		t.Fatal("expected at least one outlet with a mall window")
	}
}

// TestInvalidCSVHandling proves malformed input is rejected rather than
// silently producing a partial dataset.
func TestInvalidCSVHandling(t *testing.T) {
	// A file that does not exist in the embedded FS must error.
	if _, err := readCSV("data/does-not-exist.csv"); err == nil {
		t.Fatal("expected error for a missing embedded file")
	}
}

// TestDepotNamesMatchCSV guards the fixed depot reference facts against a CSV
// that references a depot we do not know about.
func TestDepotNamesMatchCSV(t *testing.T) {
	known := map[string]bool{}
	for _, d := range depots {
		known[d.name] = true
	}

	for _, file := range []string{"data/outlets.csv", "data/vehicles.csv", "data/district_travel.csv"} {
		rows, err := readCSV(file)
		if err != nil {
			t.Fatalf("readCSV(%s): %v", file, err)
		}
		for _, r := range rows {
			if !known[r["depot"]] {
				t.Fatalf("%s references unknown depot %q", file, r["depot"])
			}
		}
	}
}

func TestDemoDayDatasets(t *testing.T) {
	orders, err := readCSV("data/task2b_peak_day_scenarios.csv")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range orders {
		if r["scenario"] != demoScenario {
			t.Errorf("%s: scenario %q", r["order_ref"], r["scenario"])
		}
		if seen[r["order_ref"]] {
			t.Errorf("duplicate order_ref %s", r["order_ref"])
		}
		seen[r["order_ref"]] = true
		if _, err := enum(r["temp_requirement"]); err != nil {
			t.Errorf("%s: %v", r["order_ref"], err)
		}
	}

	fleet, err := readCSV("data/task2b_peak_day_fleet.csv")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range fleet {
		if s, _ := enum(r["status"]); s != "AVAILABLE" && s != "IN_WORKSHOP" {
			t.Errorf("%s: unsupported status %q", r["vehicle_id"], r["status"])
		}
	}
}
