package main

import (
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/httpx"
)

// meHandler serves GET /api/v1/me: the authenticated caller's effective
// identity — user id, role and depot/outlet scope — as resolved by the real
// Better Auth session verification path. It is the single place the frontend
// learns who it is; it never reads a client-supplied identity header.
type meHandler struct {
	auth *auth.Middleware
}

// meResponse mirrors AuthenticatedUser in
// libs/shared-types/src/lib/entities.ts. depotId/outletId are null when the
// account is not scoped to one.
type meResponse struct {
	UserID   string  `json:"userId"`
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Role     string  `json:"role"`
	DepotID  *string `json:"depotId"`
	OutletID *string `json:"outletId"`
}

// RegisterRoutes mounts /me behind the same RequireAuthenticated middleware as
// every other protected endpoint.
func (h meHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/me", h.auth.RequireAuthenticated(http.HandlerFunc(h.get)))
}

func (h meHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, meResponse{
		UserID:   id.UserID,
		Name:     id.Name,
		Email:    id.Email,
		Role:     string(id.Role),
		DepotID:  optionalID(id.DepotID),
		OutletID: optionalID(id.OutletID),
	})
}

func optionalID(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
