package routes

import (
	"errors"
	"net/http"
	"strings"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the operational route/allocation endpoints. Confirmation is
// the dispatcher's action; reads are dispatcher-scoped.
//
//	POST /api/v1/allocations/confirm   -> 200 confirmation result
//	GET  /api/v1/routes/{id}           -> 200 route with legs
//	GET  /api/v1/routes               -> 200 routes for ?date=&depotId=
//	GET  /api/v1/routes/{id}/legs      -> 200 ordered legs
//	GET  /api/v1/deferrals             -> 200 deferral history (?outletId=)
type Handler struct {
	confirmation *Confirmation
	repo         Repository
	auth         *auth.Middleware
}

// NewHandler builds the routes handler.
func NewHandler(confirmation *Confirmation, repo Repository, authMiddleware *auth.Middleware) *Handler {
	return &Handler{confirmation: confirmation, repo: repo, auth: authMiddleware}
}

// RegisterRoutes mounts the routes endpoints behind dispatcher authorization.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	dispatcher := domain.RoleDispatcher
	mux.Handle("POST /api/v1/allocations/confirm",
		h.auth.RequireRole(dispatcher, http.HandlerFunc(h.Confirm)))
	mux.Handle("GET /api/v1/routes",
		h.auth.RequireRole(dispatcher, http.HandlerFunc(h.ListRoutes)))
	mux.Handle("GET /api/v1/routes/{id}",
		h.auth.RequireRole(dispatcher, http.HandlerFunc(h.GetRoute)))
	mux.Handle("GET /api/v1/routes/{id}/legs",
		h.auth.RequireRole(dispatcher, http.HandlerFunc(h.GetLegs)))
	mux.Handle("GET /api/v1/deferrals",
		h.auth.RequireRole(dispatcher, http.HandlerFunc(h.ListDeferrals)))
}

// --- wire types ------------------------------------------------------------

type confirmRequest struct {
	JobID     string               `json:"jobId"`
	Routes    []routeChoiceRequest `json:"routes"`
	Deferrals []deferralRequest    `json:"deferrals"`
}

type routeChoiceRequest struct {
	VehicleID string   `json:"vehicleId"`
	TripNo    int      `json:"tripNo"`
	OrderIDs  []string `json:"orderIds"`
}

type deferralRequest struct {
	OrderID        string `json:"orderId"`
	ReasonType     string `json:"reasonType"`
	ConstraintCode string `json:"constraintCode,omitempty"`
	ReasonText     string `json:"reasonText"`
	DeferredToDate string `json:"deferredToDate,omitempty"`
}

type confirmResponse struct {
	RouteIDs        []string `json:"routeIds"`
	AllocatedOrders []string `json:"allocatedOrders"`
	DeferredOrders  []string `json:"deferredOrders"`
}

type routeResponse struct {
	RouteID       string        `json:"routeId"`
	VehicleID     string        `json:"vehicleId"`
	DepotID       string        `json:"depotId"`
	RouteDate     string        `json:"routeDate"`
	TripNo        int           `json:"tripNo"`
	Brand         string        `json:"brand"`
	District      string        `json:"district"`
	Status        string        `json:"status"`
	RouteVersion  int           `json:"routeVersion"`
	OutboundMin   int           `json:"outboundMin"`
	InterStopMin  int           `json:"interStopMin"`
	HandlingMin   int           `json:"handlingMin"`
	TotalTripMin  int           `json:"totalTripMin"`
	TotalWeightKg float64       `json:"totalWeightKg"`
	TotalVolumeM3 float64       `json:"totalVolumeM3"`
	DistanceKm    float64       `json:"distanceKm"`
	Legs          []legResponse `json:"legs"`
}

type legResponse struct {
	LegID      string  `json:"legId"`
	OrderID    string  `json:"orderId"`
	Seq        int     `json:"seq"`
	FromPoint  string  `json:"fromPoint"`
	ToOutletID string  `json:"toOutletId"`
	DistanceKm float64 `json:"distanceKm"`
	Status     string  `json:"status"`
}

type deferralResponse struct {
	DeferralID     string `json:"deferralId"`
	OrderID        string `json:"orderId"`
	OrderNumber    string `json:"orderNumber"`
	OutletID       string `json:"outletId"`
	ReasonType     string `json:"reasonType"`
	ReasonText     string `json:"reasonText"`
	ConstraintCode string `json:"constraintCode,omitempty"`
	DecidedBy      string `json:"decidedBy"`
	DecidedAt      string `json:"decidedAt"`
	DeferredToDate string `json:"deferredToDate,omitempty"`
}

// --- handlers --------------------------------------------------------------

// Confirm handles POST /api/v1/allocations/confirm.
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req confirmRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}

	in := ConfirmInput{JobID: req.JobID, Actor: identity.UserID}
	for _, rc := range req.Routes {
		in.Routes = append(in.Routes, RouteChoice{VehicleID: rc.VehicleID, TripNo: rc.TripNo, OrderIDs: rc.OrderIDs})
	}
	for _, d := range req.Deferrals {
		in.Deferrals = append(in.Deferrals, DeferralChoice{
			OrderID: d.OrderID, ReasonType: d.ReasonType,
			ConstraintCode: domain.ConstraintCode(strings.TrimSpace(d.ConstraintCode)),
			ReasonText:     d.ReasonText, DeferredToDate: d.DeferredToDate,
		})
	}

	result, err := h.confirmation.Confirm(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confirmResponse{
		RouteIDs: result.RouteIDs, AllocatedOrders: result.AllocatedOrders, DeferredOrders: result.DeferredOrders,
	})
}

// GetRoute handles GET /api/v1/routes/{id}.
func (h *Handler) GetRoute(w http.ResponseWriter, r *http.Request) {
	route, err := h.repo.GetRoute(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRouteResponse(route))
}

// GetLegs handles GET /api/v1/routes/{id}/legs.
func (h *Handler) GetLegs(w http.ResponseWriter, r *http.Request) {
	route, err := h.repo.GetRoute(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	legs := make([]legResponse, 0, len(route.Legs))
	for _, l := range route.Legs {
		legs = append(legs, toLegResponse(l))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"routeId": route.RouteID, "legs": legs})
}

// ListRoutes handles GET /api/v1/routes?date=&depotId=.
func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "date", Message: "is required (YYYY-MM-DD)"}})
		return
	}
	routes, err := h.repo.ListRoutes(r.Context(), date, r.URL.Query().Get("depotId"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]routeResponse, 0, len(routes))
	for _, rt := range routes {
		out = append(out, toRouteResponse(rt))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"routes": out})
}

// ListDeferrals handles GET /api/v1/deferrals?outletId=.
func (h *Handler) ListDeferrals(w http.ResponseWriter, r *http.Request) {
	entries, err := h.repo.ListDeferrals(r.Context(), r.URL.Query().Get("outletId"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]deferralResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, deferralResponse{
			DeferralID: e.DeferralID, OrderID: e.OrderID, OrderNumber: e.OrderNumber,
			OutletID: e.OutletID, ReasonType: e.ReasonType, ReasonText: e.Reason,
			ConstraintCode: e.ConstraintCode, DecidedBy: e.DecidedBy,
			DecidedAt: e.DecidedAt.Format("2006-01-02T15:04:05Z07:00"), DeferredToDate: e.DeferredToDate,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deferrals": out})
}

func toRouteResponse(rt Route) routeResponse {
	legs := make([]legResponse, 0, len(rt.Legs))
	for _, l := range rt.Legs {
		legs = append(legs, toLegResponse(l))
	}
	return routeResponse{
		RouteID: rt.RouteID, VehicleID: rt.VehicleID, DepotID: rt.DepotID, RouteDate: rt.RouteDate,
		TripNo: rt.TripNo, Brand: string(rt.Brand), District: rt.District, Status: rt.Status,
		RouteVersion: rt.RouteVersion, OutboundMin: rt.OutboundMin, InterStopMin: rt.InterStopMin,
		HandlingMin: rt.HandlingMin, TotalTripMin: rt.TotalTripMin, TotalWeightKg: rt.TotalWeightKg,
		TotalVolumeM3: rt.TotalVolumeM3, DistanceKm: rt.DistanceKm, Legs: legs,
	}
}

func toLegResponse(l RouteLeg) legResponse {
	return legResponse{
		LegID: l.LegID, OrderID: l.OrderID, Seq: l.Seq, FromPoint: l.FromPoint,
		ToOutletID: l.ToOutlet, DistanceKm: l.DistanceKm, Status: l.Status,
	}
}

// writeError maps routes errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	var violation ConstraintViolationError
	switch {
	case errors.As(err, &violation):
		// A plan that violates a hard constraint is infeasible, not a malformed
		// request: 422 with every rule verdict, matching POST /allocations/validate.
		httpx.WriteConstraintViolation(w, violation.Results)
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Not found")
	case errors.Is(err, ErrConflict):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "The plan conflicts with current state; reload and retry")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrInvalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "confirmation", Message: "the confirmation is not valid"}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
