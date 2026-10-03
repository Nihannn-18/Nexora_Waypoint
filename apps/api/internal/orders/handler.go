package orders

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the order endpoints. It depends on a Service and the auth
// Middleware, both injected, so the HTTP layer stays thin.
//
// Routes implemented (docs/api.md "Store manager"):
//
//	POST /api/v1/orders            — create (store manager owns the outlet)
//	GET  /api/v1/orders/{id}       — read one, scope-enforced
//	POST /api/v1/orders/{id}/confirm — confirm before the cutoff
//
// GET /orders/{id}/eta and POST /orders/{id}/receipt are deliberately absent:
// they need route state and delivery outcomes owned by later agents.
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the order handler. auth must be non-nil so every route is
// authenticated.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the order endpoints on the API mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/orders", h.auth.RequireAuthenticated(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/v1/orders/{id}", h.auth.RequireAuthenticated(http.HandlerFunc(h.Get)))
	mux.Handle("POST /api/v1/orders/{id}/confirm", h.auth.RequireAuthenticated(http.HandlerFunc(h.Confirm)))
}

type createOrderRequest struct {
	OutletID              string        `json:"outletId"`
	RequestedDeliveryDate string        `json:"requestedDeliveryDate"`
	Items                 []lineRequest `json:"items"`
	Notes                 string        `json:"notes,omitempty"`
}

type lineRequest struct {
	ItemID   string `json:"itemId"`
	Quantity int    `json:"quantity"`
}

type orderLineResponse struct {
	OrderItemID          string  `json:"orderItemId"`
	ItemID               string  `json:"itemId"`
	Quantity             int     `json:"quantity"`
	UnitWeightKgSnapshot float64 `json:"unitWeightKgSnapshot"`
	UnitVolumeM3Snapshot float64 `json:"unitVolumeM3Snapshot"`
	TotalWeightKg        float64 `json:"totalWeightKg"`
	TotalVolumeM3        float64 `json:"totalVolumeM3"`
}

type orderResponse struct {
	OrderID                string              `json:"orderId"`
	OrderNumber            string              `json:"orderNumber"`
	OutletID               string              `json:"outletId"`
	Brand                  string              `json:"brand"`
	OrderDate              string              `json:"orderDate"`
	RequestedDeliveryDate  string              `json:"requestedDeliveryDate"`
	TotalUnits             int                 `json:"totalUnits"`
	TotalWeightKg          float64             `json:"totalWeightKg"`
	TotalVolumeM3          float64             `json:"totalVolumeM3"`
	TemperatureRequirement string              `json:"temperatureRequirement"`
	Status                 string              `json:"status"`
	AfterCutoff            bool                `json:"afterCutoff"`
	Notes                  string              `json:"notes,omitempty"`
	Lines                  []orderLineResponse `json:"lines"`
}

func toResponse(o Order) orderResponse {
	lines := make([]orderLineResponse, 0, len(o.Lines))
	for _, ln := range o.Lines {
		lines = append(lines, orderLineResponse{
			OrderItemID:          ln.OrderItemID,
			ItemID:               ln.ItemID,
			Quantity:             ln.Quantity,
			UnitWeightKgSnapshot: ln.UnitWeightKgSnapshot,
			UnitVolumeM3Snapshot: ln.UnitVolumeM3Snapshot,
			TotalWeightKg:        ln.TotalWeightKg,
			TotalVolumeM3:        ln.TotalVolumeM3,
		})
	}
	return orderResponse{
		OrderID:                o.OrderID,
		OrderNumber:            o.OrderNumber,
		OutletID:               o.OutletID,
		Brand:                  string(o.Brand),
		OrderDate:              o.OrderDate.Format("2006-01-02"),
		RequestedDeliveryDate:  o.RequestedDeliveryDate.Format("2006-01-02"),
		TotalUnits:             o.TotalUnits,
		TotalWeightKg:          o.TotalWeightKg,
		TotalVolumeM3:          o.TotalVolumeM3,
		TemperatureRequirement: string(o.TempRequirement),
		Status:                 string(o.Status),
		AfterCutoff:            o.AfterCutoff,
		Notes:                  o.Notes,
		Lines:                  lines,
	}
}

// Create handles POST /api/v1/orders.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}

	var req createOrderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}

	delivery, err := parseDate(req.RequestedDeliveryDate)
	if err != nil {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "requestedDeliveryDate", Message: "must be YYYY-MM-DD"}})
		return
	}

	// A store manager may only order for their own outlet; a dispatcher may
	// place for any outlet. This mirrors the scope rules the rest of the API uses.
	if identity.Role == domain.RoleStoreManager && identity.OutletID != req.OutletID {
		httpx.WriteErrorCode(w, http.StatusForbidden, httpx.CodeForbidden, "You can only order for your own outlet")
		return
	}

	lines := make([]LineRequest, 0, len(req.Items))
	for _, it := range req.Items {
		lines = append(lines, LineRequest{ItemID: it.ItemID, Quantity: it.Quantity})
	}

	order, err := h.service.Create(r.Context(), CreateInput{
		OutletID:              req.OutletID,
		RequestedDeliveryDate: delivery,
		Lines:                 lines,
		Notes:                 req.Notes,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toResponse(order))
}

// Get handles GET /api/v1/orders/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	order, err := h.service.Get(r.Context(), r.PathValue("id"), scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(order))
}

// Confirm handles POST /api/v1/orders/{id}/confirm.
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	order, err := h.service.Confirm(r.Context(), r.PathValue("id"), scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(order))
}

// scopeOf narrows an authenticated identity to the order scope.
func scopeOf(id auth.Identity) Scope {
	return Scope{Role: id.Role, OutletID: id.OutletID, DepotID: id.DepotID}
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}

// writeError maps order errors onto the shared HTTP error contract:
//
//	not found          -> 404 NOT_FOUND
//	invalid input      -> 400 VALIDATION_FAILED (with the offending field)
//	illegal transition -> 409 CONFLICT
//	anything else      -> 500 INTERNAL_ERROR
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Order not found")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrConflict):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "That action is not allowed from the order's current status")
	case errors.Is(err, ErrInvalid):
		// Domain-level invalid input without a field (e.g. an unknown outlet id).
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "order", Message: "the order is not valid"}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
