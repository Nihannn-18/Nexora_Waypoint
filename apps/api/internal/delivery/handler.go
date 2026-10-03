package delivery

import (
	"errors"
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the driver delivery endpoints. Delivery is the driver's
// action; every route requires DRIVER authorization and depot scope.
//
//	POST /api/v1/legs/{id}/events  -> record one outcome with POD
//	POST /api/v1/sync/events       -> batch offline events, reconciled per event
//	GET  /api/v1/sync/status       -> the driver's synced/conflict counts
//	GET  /api/v1/legs/{id}         -> the leg's delivery context
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the delivery handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the delivery endpoints behind driver authorization.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	driver := domain.RoleDriver
	mux.Handle("POST /api/v1/legs/{id}/events",
		h.auth.RequireRole(driver, http.HandlerFunc(h.RecordEvent)))
	mux.Handle("GET /api/v1/legs/{id}",
		h.auth.RequireRole(driver, http.HandlerFunc(h.GetLeg)))
	mux.Handle("POST /api/v1/sync/events",
		h.auth.RequireRole(driver, http.HandlerFunc(h.SyncEvents)))
	mux.Handle("GET /api/v1/sync/status",
		h.auth.RequireRole(driver, http.HandlerFunc(h.SyncStatus)))
}

// --- wire types ------------------------------------------------------------

type podRequest struct {
	Type         string `json:"type,omitempty"`
	ReceiverName string `json:"receiverName,omitempty"`
	Signature    string `json:"signature,omitempty"`
	FileRef      string `json:"fileRef,omitempty"`
}

type itemRequest struct {
	OrderItemID string `json:"orderItemId"`
	Quantity    int    `json:"quantity"`
	DamagedQty  int    `json:"damagedQty,omitempty"`
	ShortQty    int    `json:"shortQty,omitempty"`
}

type eventRequest struct {
	ClientEventID  string        `json:"clientEventId"`
	Outcome        string        `json:"outcome"`
	OccurredAt     string        `json:"occurredAt"`
	CreatedOffline bool          `json:"createdOffline"`
	DeliveredItems []itemRequest `json:"deliveredItems,omitempty"`
	ReasonCode     string        `json:"reasonCode,omitempty"`
	Notes          string        `json:"notes,omitempty"`
	Pod            *podRequest   `json:"proofOfDelivery,omitempty"`
}

type syncEventsRequest struct {
	Events []syncEventRequest `json:"events"`
}

type syncEventRequest struct {
	LegID string `json:"legId"`
	eventRequest
}

type eventResultResponse struct {
	ClientEventID string `json:"clientEventId"`
	Status        string `json:"status"`
	ServerEventID string `json:"serverEventId,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type syncEventsResponse struct {
	Results []eventResultResponse `json:"results"`
}

type syncStatusResponse struct {
	Synced    int `json:"synced"`
	Conflicts int `json:"conflicts"`
}

// --- handlers --------------------------------------------------------------

// RecordEvent handles POST /api/v1/legs/{id}/events.
func (h *Handler) RecordEvent(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req eventRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	in := eventInput(r.PathValue("id"), req)
	res, err := h.service.RecordOne(r.Context(), identity.UserID, identity.DepotID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, eventResultResponse{
		ClientEventID: res.ClientEventID, Status: res.Status, ServerEventID: res.ServerEventID, Reason: res.Reason,
	})
}

// SyncEvents handles POST /api/v1/sync/events.
func (h *Handler) SyncEvents(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req syncEventsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	events := make([]EventInput, 0, len(req.Events))
	for _, e := range req.Events {
		events = append(events, eventInput(e.LegID, e.eventRequest))
	}
	results, err := h.service.SyncBatch(r.Context(), identity.UserID, identity.DepotID, events)
	if err != nil {
		writeError(w, err)
		return
	}
	out := syncEventsResponse{Results: make([]eventResultResponse, 0, len(results))}
	for _, res := range results {
		out.Results = append(out.Results, eventResultResponse{
			ClientEventID: res.ClientEventID, Status: res.Status, ServerEventID: res.ServerEventID, Reason: res.Reason,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// SyncStatus handles GET /api/v1/sync/status.
func (h *Handler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	s, err := h.service.SyncStatus(r.Context(), identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, syncStatusResponse{Synced: s.Synced, Conflicts: s.Conflicts})
}

// GetLeg handles GET /api/v1/legs/{id}.
func (h *Handler) GetLeg(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	leg, err := h.service.LegContext(r.Context(), r.PathValue("id"), identity.DepotID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"legId": leg.LegID, "routeId": leg.RouteID, "depotId": leg.DepotID,
		"routeDate": leg.RouteDate, "toOutletId": leg.ToOutlet, "status": leg.Status,
		"orderIds": leg.OrderIDs,
	})
}

func eventInput(legID string, req eventRequest) EventInput {
	in := EventInput{
		LegID: legID, ClientEventID: req.ClientEventID, Outcome: req.Outcome,
		OccurredAt: req.OccurredAt, CreatedOffline: req.CreatedOffline,
		ReasonCode: req.ReasonCode, Notes: req.Notes,
	}
	for _, it := range req.DeliveredItems {
		in.Items = append(in.Items, ItemDelivery{
			OrderItemID: it.OrderItemID, DeliveredQty: it.Quantity,
			DamagedQty: it.DamagedQty, ShortQty: it.ShortQty,
		})
	}
	if req.Pod != nil {
		in.Pod = Pod{
			Type: NormalizePodType(req.Pod.Type), ReceiverName: req.Pod.ReceiverName,
			SignatureRef: req.Pod.Signature, PhotoRef: req.Pod.FileRef,
		}
	}
	return in
}

// writeError maps delivery errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Not found")
	case errors.Is(err, ErrConflict):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "The leg changed; reload and retry")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrInvalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "event", Message: "the event is not valid"}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
