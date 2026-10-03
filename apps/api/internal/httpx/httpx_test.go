package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"waypoint.lk/api/internal/config"
)

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
		rec := get(t, Router(config.Config{}, time.Now(), []Check{up}), "/readyz")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("a failing dependency is 503 and named", func(t *testing.T) {
		rec := get(t, Router(config.Config{}, time.Now(), []Check{up, down}), "/readyz")
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
		rec := get(t, Router(config.Config{}, time.Now(), []Check{down}), "/healthz")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})
}

func TestUnknownRouteUsesErrorShape(t *testing.T) {
	rec := get(t, Router(config.Config{}, time.Now(), nil), "/nope")
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
