package receipts

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

func muxFor(t *testing.T, id auth.Identity, repo *fakeRepo) *http.ServeMux {
	t.Helper()
	mw := auth.NewMiddleware(auth.NewIdentityLoader(verifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
	mux := http.NewServeMux()
	NewHandler(NewService(repo, fixedClock{demoNow}), mw).RegisterRoutes(mux)
	return mux
}

func storeIdentity() auth.Identity {
	return auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT014"}
}

func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	return rec
}

func TestHandlerGetView(t *testing.T) {
	mux := muxFor(t, storeIdentity(), newFakeRepo())
	rec := do(t, mux, http.MethodGet, "/api/v1/orders/O1/receipt", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body)
	}
	var body viewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.OrderStatus != "DELIVERED" || len(body.Lines) != 2 {
		t.Fatalf("body = %+v", body)
	}
	l1 := body.Lines[0]
	if l1.ExpectedQty != 8 || l1.ExpectedSource != ExpectedFromLoader || l1.LoaderFlag == nil || l1.LoaderFlag.MissingQty != 2 {
		t.Errorf("L1 = %+v, want 8 expected from the loader with 2 missing", l1)
	}
	if body.Lines[1].ExpectedSource != ExpectedFromOrder {
		t.Errorf("L2 source = %q, want ORDER", body.Lines[1].ExpectedSource)
	}
	if body.ProofOfDelivery == nil || body.ProofOfDelivery.FileRef != "pod/LEG1/abc" {
		t.Errorf("pod = %+v", body.ProofOfDelivery)
	}
	if body.Receipt != nil {
		t.Errorf("receipt = %+v, want none yet", body.Receipt)
	}
}

func TestHandlerCreate(t *testing.T) {
	repo := newFakeRepo()
	mux := muxFor(t, storeIdentity(), repo)
	rec := do(t, mux, http.MethodPost, "/api/v1/orders/O1/receipt", map[string]any{
		"lines": []map[string]any{
			{"orderItemId": "L1", "receivedQty": 7, "damagedQty": 1},
			{"orderItemId": "L2", "receivedQty": 5, "damagedQty": 0},
		},
		"notes": "one carton crushed",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body)
	}
	var body receiptResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != StatusReceivedWithIssue || len(body.Issues) != 1 || body.Issues[0].Type != IssueDamaged {
		t.Fatalf("body = %+v, want one damaged issue", body)
	}
	if body.Lines[0].Condition != ConditionDamaged || body.Lines[0].ShortQty != 0 {
		t.Errorf("L1 = %+v", body.Lines[0])
	}
	if body.ReceivedAt != "2026-09-26T07:50:00+05:30" {
		t.Errorf("receivedAt = %q, want the API clock", body.ReceivedAt)
	}

	// A second submission is a conflict, never a second GRN.
	again := do(t, mux, http.MethodPost, "/api/v1/orders/O1/receipt", map[string]any{
		"lines": []map[string]any{{"orderItemId": "L1", "receivedQty": 8}, {"orderItemId": "L2", "receivedQty": 5}},
	})
	if again.Code != http.StatusConflict {
		t.Fatalf("second submit status = %d, want 409", again.Code)
	}
}

func TestHandlerCreateErrors(t *testing.T) {
	tests := []struct {
		name     string
		identity auth.Identity
		body     any
		wantCode int
		wantErr  string
	}{
		{
			name: "over-receiving is a validation error", identity: storeIdentity(),
			body:     map[string]any{"lines": []map[string]any{{"orderItemId": "L1", "receivedQty": 9}, {"orderItemId": "L2", "receivedQty": 5}}},
			wantCode: http.StatusBadRequest, wantErr: httpx.CodeValidationFailed,
		},
		{
			name: "unknown fields are refused", identity: storeIdentity(),
			body:     map[string]any{"status": "RECEIVED", "lines": []map[string]any{}},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "another outlet is not found", identity: auth.Identity{UserID: "u2", Role: domain.RoleStoreManager, OutletID: "OUT015"},
			body:     map[string]any{"lines": []map[string]any{{"orderItemId": "L1", "receivedQty": 8}, {"orderItemId": "L2", "receivedQty": 5}}},
			wantCode: http.StatusNotFound, wantErr: httpx.CodeNotFound,
		},
		{
			name: "a dispatcher cannot record a GRN", identity: auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher},
			body:     map[string]any{"lines": []map[string]any{{"orderItemId": "L1", "receivedQty": 8}, {"orderItemId": "L2", "receivedQty": 5}}},
			wantCode: http.StatusForbidden,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := muxFor(t, tc.identity, newFakeRepo())
			rec := do(t, mux, http.MethodPost, "/api/v1/orders/O1/receipt", tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantCode, rec.Body)
			}
			if tc.wantErr == "" {
				return
			}
			var body httpx.ErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != tc.wantErr {
				t.Errorf("code = %q, want %q", body.Code, tc.wantErr)
			}
		})
	}
}

func TestHandlerCreateNotDelivered(t *testing.T) {
	repo := newFakeRepo()
	o := repo.orders["O1"]
	o.Status = "IN_TRANSIT"
	repo.orders["O1"] = o
	mux := muxFor(t, storeIdentity(), repo)
	rec := do(t, mux, http.MethodPost, "/api/v1/orders/O1/receipt", map[string]any{
		"lines": []map[string]any{{"orderItemId": "L1", "receivedQty": 8}, {"orderItemId": "L2", "receivedQty": 5}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", rec.Code, rec.Body)
	}
}
