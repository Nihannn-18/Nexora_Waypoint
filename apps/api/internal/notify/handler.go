package notify

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the notification endpoints. Every endpoint is scoped to the
// authenticated recipient: a user sees only notifications addressed to them (or
// to their outlet).
//
//	GET  /api/v1/notifications                 -> list (limit, offset)
//	GET  /api/v1/notifications/unread-count    -> unread count
//	POST /api/v1/notifications/{id}/read       -> mark one read
//	POST /api/v1/notifications/read-all        -> mark all read
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the notification handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the notification endpoints behind authentication. Any
// authenticated role may manage its own notifications.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/notifications", h.auth.RequireAuthenticated(http.HandlerFunc(h.List)))
	mux.Handle("GET /api/v1/notifications/unread-count", h.auth.RequireAuthenticated(http.HandlerFunc(h.UnreadCount)))
	mux.Handle("POST /api/v1/notifications/{id}/read", h.auth.RequireAuthenticated(http.HandlerFunc(h.MarkRead)))
	mux.Handle("POST /api/v1/notifications/read-all", h.auth.RequireAuthenticated(http.HandlerFunc(h.MarkAllRead)))
}

type notificationResponse struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Reference string `json:"reference,omitempty"`
	OutletID  string `json:"outletId,omitempty"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"createdAt"`
	ReadAt    string `json:"readAt,omitempty"`
}

// List handles GET /api/v1/notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.service.List(r.Context(), recipient(identity), limit, offset)
	if err != nil {
		writeNotifyError(w, err)
		return
	}
	out := make([]notificationResponse, 0, len(items))
	for _, n := range items {
		row := notificationResponse{
			ID: n.ID, Type: n.Type, Title: n.Title, Message: n.Message,
			Reference: n.Reference, OutletID: n.OutletID, Read: n.Read(),
			CreatedAt: n.CreatedAt.Format(time.RFC3339),
		}
		if n.ReadAt != nil {
			row.ReadAt = n.ReadAt.Format(time.RFC3339)
		}
		out = append(out, row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"notifications": out})
}

// UnreadCount handles GET /api/v1/notifications/unread-count.
func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	n, err := h.service.UnreadCount(r.Context(), recipient(identity))
	if err != nil {
		writeNotifyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"unread": n})
}

// MarkRead handles POST /api/v1/notifications/{id}/read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	if err := h.service.MarkRead(r.Context(), recipient(identity), r.PathValue("id")); err != nil {
		writeNotifyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"read": true})
}

// MarkAllRead handles POST /api/v1/notifications/read-all.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	n, err := h.service.MarkAllRead(r.Context(), recipient(identity))
	if err != nil {
		writeNotifyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"markedRead": n})
}

// recipient narrows an authenticated identity to the notification recipient: the
// user id, plus their outlet for an outlet-scoped account.
func recipient(id auth.Identity) Recipient {
	return Recipient{UserID: id.UserID, OutletID: id.OutletID}
}

func writeNotifyError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Notification not found")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
