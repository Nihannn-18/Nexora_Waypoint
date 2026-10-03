package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

type verifier struct{ id auth.Identity }

func (v verifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

func middlewareFor(id auth.Identity) *auth.Middleware {
	return auth.NewMiddleware(auth.NewIdentityLoader(verifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
}

func driver() auth.Identity {
	return auth.Identity{UserID: "u-driver", Role: domain.RoleDriver, DepotID: "d-peli"}
}
func loader() auth.Identity {
	return auth.Identity{UserID: "u-loader", Role: domain.RoleLoader, DepotID: "d-peli"}
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf)))
	return rec
}

func TestHandlerRecordEvent(t *testing.T) {
	svc, _ := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{
		"clientEventId": "EV1", "outcome": "DELIVERED",
		"occurredAt": "2026-09-26T07:42:00+05:30", "createdOffline": true,
		"proofOfDelivery": map[string]any{"type": "PHOTO", "receiverName": "Nimal", "fileRef": "pod/LEG1/abc"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body)
	}
	var body eventResultResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != SyncAccepted {
		t.Fatalf("status = %q, want ACCEPTED", body.Status)
	}
}

func TestHandlerRecordEventValidation(t *testing.T) {
	svc, _ := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{
		"clientEventId": "EV1", "outcome": "DELIVERED",
		"occurredAt": "2026-09-26T07:42:00+05:30",
		// no receiver / no POD -> invalid
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != httpx.CodeValidationFailed {
		t.Fatalf("code = %q, want %q", body.Code, httpx.CodeValidationFailed)
	}
}

func TestHandlerSyncEvents(t *testing.T) {
	svc, repo := fixture()
	repo.results["EV2"] = EventResult{ClientEventID: "EV2", Status: SyncDuplicate, ServerEventID: "E2"}
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/sync/events", map[string]any{
		"events": []map[string]any{
			{"legId": "LEG1", "clientEventId": "EV1", "outcome": "DELIVERED", "occurredAt": "2026-09-26T07:42:00+05:30",
				"proofOfDelivery": map[string]any{"type": "PHOTO", "receiverName": "N", "fileRef": "pod/LEG1/a"}},
			{"legId": "LEG1", "clientEventId": "EV2", "outcome": "FAILED", "occurredAt": "2026-09-26T07:50:00+05:30"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body)
	}
	var body syncEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 2 || body.Results[0].Status != SyncAccepted || body.Results[1].Status != SyncDuplicate {
		t.Fatalf("body = %+v", body.Results)
	}
}

func TestHandlerRequiresDriver(t *testing.T) {
	svc, _ := fixture()

	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux)
	if rec := postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{}); rec.Code != http.StatusForbidden {
		t.Fatalf("loader status = %d, want 403", rec.Code)
	}

	mux2 := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(svc, failClosed).RegisterRoutes(mux2)
	if rec := postJSON(t, mux2, "/api/v1/legs/LEG1/events", map[string]any{}); rec.Code == http.StatusOK {
		t.Fatal("delivery served an unverifiable request")
	}
}

func TestHandlerSyncStatus(t *testing.T) {
	svc, repo := fixture()
	repo.status = SyncStatus{Synced: 3}
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sync/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
