package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// allowAll is a test-only resolver/authorizer that authenticates everyone and
// authorises everything, so the storage paths can be exercised without the
// real session-based resolver.
type allowAll struct{}

func (allowAll) Resolve(*http.Request) (Principal, error) {
	return Principal{UserID: "U1", Role: "LOADER"}, nil
}
func (allowAll) AuthorizeUpload(context.Context, Principal, Purpose, string) error { return nil }
func (allowAll) AuthorizeRead(context.Context, Principal, string) error            { return nil }

func newTestHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	h := NewHandler(store, allowAll{}, allowAll{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestCreateUploadRequiresAuth(t *testing.T) {
	store, _ := NewLocalStorage(t.TempDir())
	// Default (deny) resolver: every call must be 401.
	h := NewHandler(store, UnimplementedResolver{}, DenyAuthorizer{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := strings.NewReader(`{"purpose":"POD","legId":"LEG1","contentType":"image/jpeg"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCreateUploadValidatesPurpose(t *testing.T) {
	_, mux := newTestHandler(t)

	cases := []struct {
		name string
		body string
	}{
		{"missing purpose", `{"contentType":"image/jpeg"}`},
		{"pod without leg", `{"purpose":"POD","contentType":"image/jpeg"}`},
		{"shortfall without orderItem", `{"purpose":"SHORTFALL","contentType":"image/jpeg"}`},
		{"non-image", `{"purpose":"POD","legId":"L1","contentType":"application/pdf"}`},
		{"unknown field", `{"purpose":"POD","legId":"L1","contentType":"image/jpeg","evil":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUploadAndRetrieveRoundTrip(t *testing.T) {
	_, mux := newTestHandler(t)

	// 1. Request an upload.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads",
		strings.NewReader(`{"purpose":"POD","legId":"LEG1","contentType":"image/jpeg"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created createUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.FileRef == "" || !strings.HasPrefix(created.FileRef, "pod/LEG1/") {
		t.Fatalf("unexpected fileRef %q", created.FileRef)
	}
	if created.UploadMode != "inline" {
		t.Fatalf("uploadMode = %q, want inline for local backend", created.UploadMode)
	}

	// 2. Upload the bytes to the server-generated key.
	img := []byte("fake-jpeg-bytes")
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/media/"+created.FileRef, bytes.NewReader(img))
	putReq.Header.Set("Content-Type", "image/jpeg")
	putRec := httptest.NewRecorder()
	mux.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusCreated {
		t.Fatalf("put status = %d, want 201: %s", putRec.Code, putRec.Body.String())
	}

	// 3. Retrieve it back.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+created.FileRef, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if ct := getRec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", ct)
	}
	if !bytes.Equal(getRec.Body.Bytes(), img) {
		t.Fatalf("round-tripped body mismatch")
	}
}

func TestUploadRejectsUnknownKey(t *testing.T) {
	_, mux := newTestHandler(t)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/media/pod/LEG1/../../etc/passwd", strings.NewReader("x"))
	putReq.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, putReq)
	if rec.Code == http.StatusCreated {
		t.Fatal("traversal key was accepted")
	}
}

func TestGetMissingReturns404(t *testing.T) {
	_, mux := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/pod/LEG1/missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// denyRead authorises uploads but denies reads, to prove the read path is
// gated independently.
type denyRead struct{ allowAll }

func (denyRead) AuthorizeRead(context.Context, Principal, string) error {
	return errors.New("out of scope")
}

func TestReadAuthorizationEnforced(t *testing.T) {
	store, _ := NewLocalStorage(t.TempDir())
	if err := store.Put(context.Background(), "pod/LEG1/obj", strings.NewReader("x"), "image/jpeg"); err != nil {
		t.Fatalf("seed object: %v", err)
	}
	h := NewHandler(store, allowAll{}, denyRead{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/pod/LEG1/obj", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
