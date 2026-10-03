package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"waypoint.lk/api/internal/httpx"
)

// maxUploadBytes caps a single media upload. A phone photo is a few megabytes;
// 8 MiB leaves headroom without letting a client exhaust the disk.
const maxUploadBytes = 8 << 20

// presignTTL is how long an upload or download URL stays valid, in seconds.
const presignTTL = 300

// Principal is the authenticated caller, reduced to what media authorisation
// needs. It is produced by the auth layer (internal/auth) once Better Auth
// integration lands; until then the resolver returns an error and every media
// request is 401.
//
// This is a local, intentionally narrow view: the media package must not import
// internal/auth's full Identity, so AuthResolver adapts one to the other at the
// composition root.
type Principal struct {
	UserID   string
	Role     string
	DepotID  string
	OutletID string
}

// Resolver turns an authenticated HTTP request into a Principal.
//
// It is implemented by AuthResolver, which delegates to internal/auth. Kept as
// an interface so the media handler does not depend on the auth package.
type Resolver interface {
	Resolve(r *http.Request) (Principal, error)
}

// AuthResolver adapts internal/auth to the media Resolver interface. It is
// wired in main; the media package never imports auth directly.
//
// Load is auth's identity load wrapped to the minimal surface media needs:
// user id, role and the depot/outlet scope the authorizer checks.
type AuthResolver struct {
	Load func(r *http.Request) (userID, role, depotID, outletID string, err error)
}

// Resolve delegates to the injected loader.
func (a AuthResolver) Resolve(r *http.Request) (Principal, error) {
	if a.Load == nil {
		return Principal{}, ErrUnauthenticated
	}
	userID, role, depotID, outletID, err := a.Load(r)
	if err != nil {
		return Principal{}, err
	}
	return Principal{UserID: userID, Role: role, DepotID: depotID, OutletID: outletID}, nil
}

// UnimplementedResolver is the placeholder wired in until Better Auth lands. It
// refuses every request, so no media can be read or written without
// authorisation — the safe default.
type UnimplementedResolver struct{}

// Resolve always returns ErrUnauthenticated.
func (UnimplementedResolver) Resolve(*http.Request) (Principal, error) {
	return Principal{}, ErrUnauthenticated
}

// ErrUnauthenticated is returned when no valid session is present.
var ErrUnauthenticated = errors.New("unauthenticated")

// ErrForbidden is returned when the caller is authenticated but out of scope
// for the requested media object.
var ErrForbidden = errors.New("forbidden")

// Authorizer decides whether a principal may touch a given media purpose and
// owner. The real implementation checks depot/outlet/route scope. The default
// denies everything until auth exists.
type Authorizer interface {
	// AuthorizeUpload reports whether p may upload for purpose/owner, and the
	// content-type/extension policy is the caller's concern.
	AuthorizeUpload(ctx context.Context, p Principal, purpose Purpose, ownerID string) error
	// AuthorizeRead reports whether p may read an existing object key.
	AuthorizeRead(ctx context.Context, p Principal, key string) error
}

// DenyAuthorizer refuses everything, matching UnimplementedResolver.
type DenyAuthorizer struct{}

// AuthorizeUpload always denies.
func (DenyAuthorizer) AuthorizeUpload(context.Context, Principal, Purpose, string) error {
	return ErrUnauthenticated
}

// AuthorizeRead always denies.
func (DenyAuthorizer) AuthorizeRead(context.Context, Principal, string) error {
	return ErrUnauthenticated
}

// OwnerScope is the depot/outlet a media owner (order line or leg) belongs to.
type OwnerScope struct {
	DepotID  string
	OutletID string
	Found    bool
}

// OwnerReader resolves the business scope of a media owner. The database-backed
// implementation is wired in main, so this package stays free of SQL.
type OwnerReader interface {
	// OrderItemScope resolves the depot/outlet of a shortfall's order line.
	OrderItemScope(ctx context.Context, orderItemID string) (OwnerScope, error)
	// LegScope resolves the depot/outlet of a POD's route leg.
	LegScope(ctx context.Context, legID string) (OwnerScope, error)
}

// ScopeAuthorizer enforces role, purpose and depot/outlet scope for media.
//
// Uploads are restricted to the role that produces them (LOADER for a shortfall,
// DRIVER for a POD) and to their own depot. Reads are allowed for the dispatcher
// (both depots), the producing role within its depot, and a store manager for an
// object belonging to its outlet. A wrong-purpose, cross-scope or unknown-owner
// request is denied.
type ScopeAuthorizer struct {
	owners OwnerReader
}

// NewScopeAuthorizer builds a scope authorizer over an owner reader.
func NewScopeAuthorizer(owners OwnerReader) ScopeAuthorizer {
	return ScopeAuthorizer{owners: owners}
}

// AuthorizeUpload checks that the principal may upload for purpose/owner.
func (a ScopeAuthorizer) AuthorizeUpload(ctx context.Context, p Principal, purpose Purpose, ownerID string) error {
	scope, err := a.ownerScope(ctx, purpose, ownerID)
	if err != nil {
		return err
	}
	if !scope.Found {
		return ErrForbidden
	}
	if !sameDepot(p, scope) {
		return ErrForbidden
	}
	switch purpose {
	case PurposeShortfall:
		if p.Role != roleLoader {
			return ErrForbidden
		}
	case PurposePOD:
		if p.Role != roleDriver {
			return ErrForbidden
		}
	default:
		return ErrForbidden
	}
	return nil
}

// AuthorizeRead checks that the principal may read the object named by key.
func (a ScopeAuthorizer) AuthorizeRead(ctx context.Context, p Principal, key string) error {
	purpose, ownerID, err := ParseKey(key)
	if err != nil {
		return ErrForbidden
	}
	scope, err := a.ownerScope(ctx, purpose, ownerID)
	if err != nil {
		return err
	}
	if !scope.Found {
		return ErrForbidden
	}
	switch p.Role {
	case roleDispatcher:
		return nil
	case roleStoreManager:
		if p.OutletID != "" && p.OutletID == scope.OutletID {
			return nil
		}
		return ErrForbidden
	case roleLoader, roleDriver:
		if sameDepot(p, scope) {
			return nil
		}
		return ErrForbidden
	default:
		return ErrForbidden
	}
}

func (a ScopeAuthorizer) ownerScope(ctx context.Context, purpose Purpose, ownerID string) (OwnerScope, error) {
	if a.owners == nil {
		return OwnerScope{}, ErrForbidden
	}
	switch purpose {
	case PurposeShortfall:
		return a.owners.OrderItemScope(ctx, ownerID)
	case PurposePOD:
		return a.owners.LegScope(ctx, ownerID)
	default:
		return OwnerScope{}, ErrForbidden
	}
}

// Media roles, mirroring domain.Role. Defined here so the media package does not
// import the domain vocabulary just for four strings.
const (
	roleDispatcher   = "DISPATCHER"
	roleLoader       = "LOADER"
	roleDriver       = "DRIVER"
	roleStoreManager = "STORE_MANAGER"
)

func sameDepot(p Principal, scope OwnerScope) bool {
	return p.DepotID != "" && scope.DepotID != "" && p.DepotID == scope.DepotID
}

// Handler serves the media endpoints. It depends on a Storage, a Resolver and
// an Authorizer, all injected so the HTTP layer stays thin.
type Handler struct {
	storage    Storage
	resolver   Resolver
	authorizer Authorizer
}

// NewHandler builds the media handler.
func NewHandler(storage Storage, resolver Resolver, authorizer Authorizer) *Handler {
	return &Handler{storage: storage, resolver: resolver, authorizer: authorizer}
}

// RegisterRoutes mounts the media endpoints on the API mux. Upload URLs and
// keys are minted server-side; the handlers authorise the caller before any
// byte moves.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/media/uploads", h.CreateUpload)
	mux.HandleFunc("PUT /api/v1/media/{key...}", h.PutBytes)
	mux.HandleFunc("GET /api/v1/media/{key...}", h.GetBytes)
}

type createUploadRequest struct {
	// Purpose is the workflow: "SHORTFALL" or "POD".
	Purpose string `json:"purpose"`
	// OrderItemID is required for SHORTFALL.
	OrderItemID string `json:"orderItemId,omitempty"`
	// LegID is required for POD.
	LegID string `json:"legId,omitempty"`
	// ContentType is the upload's media type, e.g. "image/jpeg".
	ContentType string `json:"contentType"`
}

type createUploadResponse struct {
	FileRef    string            `json:"fileRef"`
	UploadMode string            `json:"uploadMode"`
	UploadURL  string            `json:"uploadUrl"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// CreateUpload registers an intent to upload and returns a server-generated
// key plus a URL to put bytes at. It never accepts a client-supplied key.
func (h *Handler) CreateUpload(w http.ResponseWriter, r *http.Request) {
	principal, err := h.resolver.Resolve(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req createUploadRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	purpose, ownerID, err := purposeAndOwner(req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !isImageContentType(req.ContentType) {
		httpx.WriteError(w, http.StatusBadRequest, "Only image uploads are accepted")
		return
	}
	if err := h.authorizer.AuthorizeUpload(r.Context(), principal, purpose, ownerID); err != nil {
		writeAuthzError(w, err)
		return
	}

	key, err := KeyFor(purpose, ownerID, newObjectID())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Could not allocate media key")
		return
	}

	url, headers, err := h.storage.PresignPut(r.Context(), key, req.ContentType, presignTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Could not prepare upload")
		return
	}

	mode := "presigned"
	if _, ok := h.storage.(*LocalStorage); ok {
		mode = "inline"
	}

	httpx.WriteJSON(w, http.StatusCreated, createUploadResponse{
		FileRef:    key,
		UploadMode: mode,
		UploadURL:  url,
		Headers:    headers,
	})
}

// PutBytes stores the uploaded object for a key minted by CreateUpload. It is
// the local-backend upload route; with S3 the client uploads directly to the
// presigned URL and never calls this.
func (h *Handler) PutBytes(w http.ResponseWriter, r *http.Request) {
	principal, err := h.resolver.Resolve(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	key := r.PathValue("key")
	purpose, ownerID, err := ParseKey(key)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid media key")
		return
	}
	// This is an upload: authorise it as one. The key was minted by CreateUpload,
	// which already authorised the intent; this re-checks the principal still has
	// upload scope for the object it names.
	if err := h.authorizer.AuthorizeUpload(r.Context(), principal, purpose, ownerID); err != nil {
		writeAuthzError(w, err)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if !isImageContentType(contentType) {
		httpx.WriteError(w, http.StatusBadRequest, "Only image uploads are accepted")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := h.storage.Put(r.Context(), key, body, contentType); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "Image exceeds the size limit")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "Could not store image")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"fileRef": key})
}

// GetBytes streams an authorised object for the local backend. With S3 the
// client follows the presigned URL returned by the create step instead.
func (h *Handler) GetBytes(w http.ResponseWriter, r *http.Request) {
	principal, err := h.resolver.Resolve(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	key := r.PathValue("key")
	if _, err := safeKey(key); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid media key")
		return
	}
	if err := h.authorizer.AuthorizeRead(r.Context(), principal, key); err != nil {
		writeAuthzError(w, err)
		return
	}

	rc, contentType, err := h.storage.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			httpx.WriteError(w, http.StatusNotFound, "Media not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "Could not read media")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func purposeAndOwner(req createUploadRequest) (Purpose, string, error) {
	switch strings.ToUpper(strings.TrimSpace(req.Purpose)) {
	case "SHORTFALL":
		if strings.TrimSpace(req.OrderItemID) == "" {
			return "", "", errors.New("orderItemId is required for a shortfall photo")
		}
		return PurposeShortfall, req.OrderItemID, nil
	case "POD":
		if strings.TrimSpace(req.LegID) == "" {
			return "", "", errors.New("legId is required for a POD photo")
		}
		return PurposePOD, req.LegID, nil
	default:
		return "", "", errors.New(`purpose must be "SHORTFALL" or "POD"`)
	}
}

func isImageContentType(ct string) bool {
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "image/jpeg", "image/png", "image/webp", "image/heic":
		return true
	default:
		return false
	}
}

// newObjectID returns a random hex identifier for an object key.
func newObjectID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("media: random id: %v", err))
	}
	return hex.EncodeToString(b[:])
}

func writeAuthzError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnauthenticated) {
		httpx.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	httpx.WriteError(w, http.StatusForbidden, "Out of scope for this record")
}
