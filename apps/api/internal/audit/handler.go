package audit

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the audit query endpoint. Audit is dispatcher-only (there is no
// admin role), and a dispatcher sees both depots, optionally narrowed by
// depotId.
//
//	GET /api/v1/audit?actor=&action=&entityType=&entityId=&depotId=&from=&to=&limit=&offset=
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the audit handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the audit endpoint behind dispatcher authorization.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/audit",
		h.auth.RequireRole(domain.RoleDispatcher, http.HandlerFunc(h.List)))
}

type recordResponse struct {
	ID         string         `json:"id"`
	Actor      string         `json:"actor,omitempty"`
	Role       string         `json:"role,omitempty"`
	Action     string         `json:"action"`
	EntityType string         `json:"entityType"`
	EntityID   string         `json:"entityId,omitempty"`
	DepotID    string         `json:"depotId,omitempty"`
	OutletID   string         `json:"outletId,omitempty"`
	Result     string         `json:"result,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	OccurredAt string         `json:"occurredAt"`
}

// List handles GET /api/v1/audit.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	q := r.URL.Query()
	filter := Filter{
		Actor:      q.Get("actor"),
		Action:     q.Get("action"),
		EntityType: q.Get("entityType"),
		EntityID:   q.Get("entityId"),
		DepotID:    q.Get("depotId"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = t
		} else {
			httpx.WriteValidation(w, []httpx.FieldError{{Field: "from", Message: "must be RFC 3339"}})
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = t
		} else {
			httpx.WriteValidation(w, []httpx.FieldError{{Field: "to", Message: "must be RFC 3339"}})
			return
		}
	}

	records, err := h.service.List(r.Context(), Identity{UserID: identity.UserID, Role: identity.Role}, filter)
	if err != nil {
		writeAuditError(w, err)
		return
	}
	out := make([]recordResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, recordResponse{
			ID: rec.ID, Actor: rec.Actor, Role: rec.Role, Action: rec.Action,
			EntityType: rec.EntityType, EntityID: rec.EntityID, DepotID: rec.DepotID,
			OutletID: rec.OutletID, Result: rec.Result, Detail: rec.Detail,
			OccurredAt: rec.OccurredAt.Format(time.RFC3339),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"records": out})
}

func writeAuditError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
