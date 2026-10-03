package authapi

import (
	"errors"
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the authentication endpoints. Login is public; logout and /me
// sit behind the same RequireAuthenticated middleware as every other protected
// endpoint.
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the endpoints on the API mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.Handle("POST /api/v1/auth/logout", h.auth.RequireAuthenticated(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /api/v1/me", h.auth.RequireAuthenticated(http.HandlerFunc(h.Me)))
}

// --- wire types ------------------------------------------------------------

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// userResponse mirrors AuthenticatedUser in
// libs/shared-types/src/lib/entities.ts. depotId/outletId are null when the
// account is not scoped to one.
type userResponse struct {
	UserID   string  `json:"userId"`
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Role     string  `json:"role"`
	DepotID  *string `json:"depotId"`
	OutletID *string `json:"outletId"`
}

type loginResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

func toUserResponse(id auth.Identity) userResponse {
	return userResponse{
		UserID:   id.UserID,
		Name:     id.Name,
		Email:    id.Email,
		Role:     string(id.Role),
		DepotID:  optionalID(id.DepotID),
		OutletID: optionalID(id.OutletID),
	}
}

func optionalID(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// --- handlers --------------------------------------------------------------

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}

	validation := &httpx.Validation{}
	validation.Required("email", req.Email)
	validation.Required("password", req.Password)
	if errs := validation.Errors(); len(errs) > 0 {
		httpx.WriteValidation(w, errs)
		return
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeLoginError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, loginResponse{
		Token: result.Token,
		User:  toUserResponse(result.User),
	})
}

// Logout handles POST /api/v1/auth/logout. It revokes the session named by the
// request's bearer token; the caller cannot name another user's session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	rawToken, ok := auth.BearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	if err := h.service.Logout(r.Context(), rawToken); err != nil {
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/v1/me: the authenticated caller's effective identity.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserResponse(identity))
}

// writeLoginError maps a login failure to a generic 401. Bad credentials and an
// inactive account are deliberately indistinguishable so a client cannot probe
// for valid emails or account state. A database fault is a real 500.
func writeLoginError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrUnauthenticated) || errors.Is(err, auth.ErrForbidden) {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Invalid email or password")
		return
	}
	httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
}
