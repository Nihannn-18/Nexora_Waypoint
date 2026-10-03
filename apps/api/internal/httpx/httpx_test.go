package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"waypoint.lk/api/internal/clock"
	"waypoint.lk/api/internal/config"
)

// fixedClock is a Clock that always reads t, so /meta is deterministic.
type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func TestMeta(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 25, 16, 5, 0, 0, loc)

	for _, demoMode := range []bool{true, false} {
		cfg := config.Config{Timezone: "Asia/Colombo", DemoMode: demoMode}
		rec := get(t, Router(cfg, fixedClock{t: at}, time.Now(), nil), "/api/v1/meta")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"now": "2026-09-25T16:05:00+05:30", "demoMode": demoMode, "timezone": "Asia/Colombo"}
		if len(body) != len(want) {
			t.Errorf("demoMode=%v: body = %v, want exactly the documented fields %v", demoMode, body, want)
		}
		for k, v := range want {
			if body[k] != v {
				t.Errorf("demoMode=%v: %s = %v, want %v", demoMode, k, body[k], v)
			}
		}
	}
}

func TestMetaTicksWithDemoClock(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 25, 15, 40, 0, 0, loc)
	base := &stepClock{t: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)}
	h := Router(config.Config{Timezone: "Asia/Colombo", DemoMode: true}, clock.NewDemo(start, base), time.Now(), nil)

	nowOf := func() string {
		var body MetaResponse
		if err := json.Unmarshal(get(t, h, "/api/v1/meta").Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Now
	}
	if got := nowOf(); got != "2026-09-25T15:40:00+05:30" {
		t.Fatalf("now = %s, want the demo start", got)
	}
	base.t = base.t.Add(25 * time.Minute)
	if got := nowOf(); got != "2026-09-25T16:05:00+05:30" {
		t.Fatalf("now = %s, want 25 minutes after the demo start", got)
	}
}

// stepClock is a Clock the test advances by hand.
type stepClock struct{ t time.Time }

func (s *stepClock) Now() time.Time { return s.t }

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestReadyz(t *testing.T) {
	up := Check{Name: "database", Fn: func(context.Context) error { return nil }}
	down := Check{Name: "queue", Fn: func(context.Context) error { return errors.New("refused") }}

	t.Run("all up is 200", func(t *testing.T) {
		rec := get(t, Router(config.Config{}, fixedClock{}, time.Now(), []Check{up}), "/readyz")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("a failing dependency is 503 and named", func(t *testing.T) {
		rec := get(t, Router(config.Config{}, fixedClock{}, time.Now(), []Check{up, down}), "/readyz")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
		var body struct {
			Dependencies map[string]string `json:"dependencies"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Dependencies["queue"] != "down" || body.Dependencies["database"] != "up" {
			t.Fatalf("dependencies = %v", body.Dependencies)
		}
	})

	t.Run("liveness ignores dependencies", func(t *testing.T) {
		rec := get(t, Router(config.Config{}, fixedClock{}, time.Now(), []Check{down}), "/healthz")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})
}

func TestUnknownRouteUsesErrorShape(t *testing.T) {
	rec := get(t, Router(config.Config{}, fixedClock{}, time.Now(), nil), "/nope")
	var body ErrorBody
	if rec.Code != http.StatusNotFound || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Message == "" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

func TestPanicBecomes500(t *testing.T) {
	h := withRecover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := get(t, h, "/")
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusInternalServerError || body.Code != CodeInternal {
		t.Fatalf("status %d body %+v", rec.Code, body)
	}
	if body.Message == "boom" {
		t.Fatal("panic value leaked to the client")
	}
}

func TestValidation(t *testing.T) {
	var v Validation
	v.Required("name", "  ")
	v.OneOf("temp", "WARM", "CHILLED", "AMBIENT")
	v.OneOf("optional", "", "A")
	v.Positive("weightKg", 0)
	if got := len(v.Errors()); got != 3 {
		t.Fatalf("errors = %v, want 3 (empty OneOf must be left to Required)", v.Errors())
	}

	rec := httptest.NewRecorder()
	WriteValidation(rec, v.Errors())
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body.Code != CodeValidationFailed || len(body.FieldErrors) != 3 {
		t.Fatalf("status %d body %+v", rec.Code, body)
	}

	var ok Validation
	ok.Required("name", "x")
	if ok.Errors() != nil {
		t.Fatal("valid input produced errors")
	}
}
