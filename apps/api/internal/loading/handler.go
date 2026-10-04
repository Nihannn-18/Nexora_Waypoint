package loading

import (
	"errors"
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the loader endpoints. Loading is the loader's action; reads
// and writes are scoped to the caller's depot (a loader's routes are its depot's
// routes).
//
//	GET  /api/v1/loading/routes?date=   -> 200 the depot's confirmed routes
//	GET  /api/v1/routes/{id}/loading    -> 200 picking list with load state
//	POST /api/v1/routes/{id}/shortfalls -> 200 route load state after recording
//
// The POST path matches docs/api.md. The GET path is named "loading" rather than
// "legs" so it does not collide with the dispatcher-facing
// GET /routes/{id}/legs; it is the loader's picking list (order lines, expected
// quantity, current loaded/damaged/missing).
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the loading handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the loader endpoints behind loader authorization.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	loader := domain.RoleLoader
	mux.Handle("GET /api/v1/loading/routes",
		h.auth.RequireRole(loader, http.HandlerFunc(h.Routes)))
	mux.Handle("GET /api/v1/routes/{id}/loading",
		h.auth.RequireRole(loader, http.HandlerFunc(h.PickingList)))
	mux.Handle("POST /api/v1/routes/{id}/shortfalls",
		h.auth.RequireRole(loader, http.HandlerFunc(h.RecordShortfalls)))
}

type lineResponse struct {
	OrderItemID  string `json:"orderItemId"`
	OrderID      string `json:"orderId"`
	ItemID       string `json:"itemId"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	OrderedQty   int    `json:"orderedQty"`
	LoadedQty    int    `json:"loadedQty"`
	DamagedQty   int    `json:"damagedQty"`
	MissingQty   int    `json:"missingQty"`
	ShortfallQty int    `json:"shortfallQty"`
	PhotoRef     string `json:"photoRef,omitempty"`
	RecordedBy   string `json:"recordedBy,omitempty"`
	RecordedAt   string `json:"recordedAt,omitempty"`

	Seq             int     `json:"seq"`
	OutletID        string  `json:"outletId"`
	OutletName      string  `json:"outletName"`
	OrderNumber     string  `json:"orderNumber"`
	DockType        string  `json:"dockType"`
	TempRequirement string  `json:"tempRequirement"`
	WeightKg        float64 `json:"weightKg"`
	VolumeM3        float64 `json:"volumeM3"`
}

type routeSummaryResponse struct {
	RouteID       string `json:"routeId"`
	VehicleID     string `json:"vehicleId"`
	DepotID       string `json:"depotId"`
	RouteDate     string `json:"routeDate"`
	TripNo        int    `json:"tripNo"`
	Brand         string `json:"brand"`
	District      string `json:"district"`
	Status        string `json:"status"`
	Stops         int    `json:"stops"`
	Lines         int    `json:"lines"`
	LinesComplete int    `json:"linesComplete"`
	ShortfallQty  int    `json:"shortfallQty"`
	Ready         bool   `json:"routeReady"`
}

type routeLoadingResponse struct {
	RouteID   string         `json:"routeId"`
	VehicleID string         `json:"vehicleId"`
	DepotID   string         `json:"depotId"`
	RouteDate string         `json:"routeDate"`
	TripNo    int            `json:"tripNo"`
	Brand     string         `json:"brand"`
	District  string         `json:"district"`
	Status    string         `json:"status"`
	Ready     bool           `json:"routeReady"`
	Lines     []lineResponse `json:"lines"`

	VehicleType    string  `json:"vehicleType"`
	VehicleTemp    string  `json:"vehicleTemp"`
	WeightCapKg    float64 `json:"weightCapKg"`
	VolumeCapM3    float64 `json:"volumeCapM3"`
	LoadedWeightKg float64 `json:"loadedWeightKg"`
	LoadedVolumeM3 float64 `json:"loadedVolumeM3"`
	Stops          int     `json:"stops"`
}

type shortfallRequest struct {
	Items []lineUpdateRequest `json:"items"`
}

type lineUpdateRequest struct {
	OrderItemID string `json:"orderItemId"`
	LoadedQty   int    `json:"loadedQty"`
	DamagedQty  int    `json:"damagedQty"`
	MissingQty  int    `json:"missingQty"`
	PhotoRef    string `json:"photoRef,omitempty"`
}

// Routes handles GET /api/v1/loading/routes?date=YYYY-MM-DD.
func (h *Handler) Routes(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	summaries, err := h.service.Routes(r.Context(), identity.DepotID, r.URL.Query().Get("date"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]routeSummaryResponse, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, routeSummaryResponse{
			RouteID: s.RouteID, VehicleID: s.VehicleID, DepotID: s.DepotID,
			RouteDate: s.RouteDate, TripNo: s.TripNo, Brand: string(s.Brand),
			District: s.District, Status: s.Status, Stops: s.Stops, Lines: s.Lines,
			LinesComplete: s.LinesComplete, ShortfallQty: s.ShortfallQty, Ready: s.Ready(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"routes": out})
}

// PickingList handles GET /api/v1/routes/{id}/loading.
func (h *Handler) PickingList(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	rl, err := h.service.PickingList(r.Context(), r.PathValue("id"), identity.DepotID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(rl))
}

// RecordShortfalls handles POST /api/v1/routes/{id}/shortfalls.
func (h *Handler) RecordShortfalls(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req shortfallRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	updates := make([]LineUpdate, 0, len(req.Items))
	for _, it := range req.Items {
		updates = append(updates, LineUpdate{
			OrderItemID: it.OrderItemID, LoadedQty: it.LoadedQty,
			DamagedQty: it.DamagedQty, MissingQty: it.MissingQty, PhotoRef: it.PhotoRef,
		})
	}
	rl, err := h.service.RecordShortfalls(r.Context(), r.PathValue("id"), identity.DepotID, identity.UserID, updates)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(rl))
}

func toResponse(rl RouteLoading) routeLoadingResponse {
	lines := make([]lineResponse, 0, len(rl.Lines))
	for _, l := range rl.Lines {
		lines = append(lines, lineResponse{
			OrderItemID: l.OrderItemID, OrderID: l.OrderID, ItemID: l.ItemID,
			SKU: l.SKU, Name: l.Name, OrderedQty: l.OrderedQty, LoadedQty: l.LoadedQty,
			DamagedQty: l.DamagedQty, MissingQty: l.MissingQty, ShortfallQty: l.ShortfallQty(),
			PhotoRef: l.PhotoRef, RecordedBy: l.RecordedBy, RecordedAt: l.RecordedAt,
			Seq: l.Seq, OutletID: l.OutletID, OutletName: l.OutletName, OrderNumber: l.OrderNumber,
			DockType: l.DockType, TempRequirement: l.TempRequirement,
			WeightKg: l.WeightKg, VolumeM3: l.VolumeM3,
		})
	}
	return routeLoadingResponse{
		RouteID: rl.RouteID, VehicleID: rl.VehicleID, DepotID: rl.DepotID, RouteDate: rl.RouteDate,
		TripNo: rl.TripNo, Brand: string(rl.Brand), District: rl.District, Status: rl.Status,
		Ready: rl.Ready(), Lines: lines,
		VehicleType: rl.VehicleType, VehicleTemp: rl.VehicleTemp,
		WeightCapKg: rl.WeightCapKg, VolumeCapM3: rl.VolumeCapM3,
		LoadedWeightKg: rl.LoadedWeightKg(), LoadedVolumeM3: rl.LoadedVolumeM3(),
		Stops: rl.Stops(),
	}
}

// writeError maps loading errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Not found")
	case errors.Is(err, ErrConflict):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "The route is not in a state that permits loading")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrInvalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "items", Message: "the submission is not valid"}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
