package assignment

import (
	"errors"
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the Dispatcher assignment surface plus the driver's own
// assignment read.
//
//	GET    /api/v1/drivers                          -> active drivers for the picker
//	GET    /api/v1/vehicles/{id}/assignment         -> { assignment | null }
//	PUT    /api/v1/vehicles/{id}/assignment         -> assign / change driver
//	DELETE /api/v1/vehicles/{id}/assignment?date=   -> remove driver
//	GET    /api/v1/outlets/{id}/manager             -> { manager | null }
//	PUT    /api/v1/outlets/{id}/manager             -> assign / change manager
//	DELETE /api/v1/outlets/{id}/manager             -> remove manager
//	GET    /api/v1/driver/assignment?date=          -> the caller's own assignment
//
// Every mutation is Dispatcher-only. The driver read is self-scoped: the driver
// id comes from the authenticated identity, never the request.
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the assignment handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the assignment endpoints.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	d := domain.RoleDispatcher
	mux.Handle("GET /api/v1/drivers", h.auth.RequireRole(d, http.HandlerFunc(h.ListDrivers)))
	mux.Handle("GET /api/v1/store-managers", h.auth.RequireRole(d, http.HandlerFunc(h.ListStoreManagers)))
	mux.Handle("GET /api/v1/vehicles/{id}/assignment", h.auth.RequireRole(d, http.HandlerFunc(h.GetVehicleAssignment)))
	mux.Handle("PUT /api/v1/vehicles/{id}/assignment", h.auth.RequireRole(d, http.HandlerFunc(h.PutVehicleAssignment)))
	mux.Handle("DELETE /api/v1/vehicles/{id}/assignment", h.auth.RequireRole(d, http.HandlerFunc(h.DeleteVehicleAssignment)))
	mux.Handle("GET /api/v1/outlets/{id}/manager", h.auth.RequireRole(d, http.HandlerFunc(h.GetOutletManager)))
	mux.Handle("PUT /api/v1/outlets/{id}/manager", h.auth.RequireRole(d, http.HandlerFunc(h.PutOutletManager)))
	mux.Handle("DELETE /api/v1/outlets/{id}/manager", h.auth.RequireRole(d, http.HandlerFunc(h.DeleteOutletManager)))
	mux.Handle("GET /api/v1/driver/assignment", h.auth.RequireRole(
		domain.RoleDriver, http.HandlerFunc(h.GetOwnAssignment)))
}

// --- wire types ------------------------------------------------------------

type driverResponse struct {
	UserID  string `json:"userId"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	DepotID string `json:"depotId"`
}

type storeManagerResponse struct {
	UserID   string `json:"userId"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	OutletID string `json:"outletId"`
}

type vehicleAssignmentResponse struct {
	VehicleID  string `json:"vehicleId"`
	DriverID   string `json:"driverId"`
	DriverName string `json:"driverName"`
	Email      string `json:"driverEmail"`
	Date       string `json:"date"`
	DepotID    string `json:"depotId"`
}

type outletManagerResponse struct {
	OutletID string `json:"outletId"`
	UserID   string `json:"userId"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	DepotID  string `json:"depotId"`
}

type assignDriverRequest struct {
	DriverID string `json:"driverId"`
	Date     string `json:"date"`
}

type assignManagerRequest struct {
	UserID string `json:"userId"`
}

// --- handlers --------------------------------------------------------------

// ListDrivers handles GET /api/v1/drivers.
func (h *Handler) ListDrivers(w http.ResponseWriter, r *http.Request) {
	drivers, err := h.service.ListDrivers(r.Context(), r.URL.Query().Get("depotId"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]driverResponse, 0, len(drivers))
	for _, d := range drivers {
		out = append(out, driverResponse{UserID: d.UserID, Name: d.Name, Email: d.Email, DepotID: d.DepotID})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"drivers": out})
}

// ListStoreManagers handles GET /api/v1/store-managers.
func (h *Handler) ListStoreManagers(w http.ResponseWriter, r *http.Request) {
	managers, err := h.service.ListStoreManagers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]storeManagerResponse, 0, len(managers))
	for _, m := range managers {
		out = append(out, storeManagerResponse{UserID: m.UserID, Name: m.Name, Email: m.Email, OutletID: m.OutletID})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"storeManagers": out})
}

// GetVehicleAssignment handles GET /api/v1/vehicles/{id}/assignment.
func (h *Handler) GetVehicleAssignment(w http.ResponseWriter, r *http.Request) {
	a, ok, err := h.service.VehicleAssignment(r.Context(), r.PathValue("id"), r.URL.Query().Get("date"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"assignment": nil})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"assignment": toVehicleAssignmentResponse(a)})
}

// PutVehicleAssignment handles PUT /api/v1/vehicles/{id}/assignment.
func (h *Handler) PutVehicleAssignment(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req assignDriverRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	a, err := h.service.AssignDriver(r.Context(), AssignDriverInput{
		VehicleID: r.PathValue("id"), DriverID: req.DriverID, Date: req.Date,
	}, identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toVehicleAssignmentResponse(a))
}

// DeleteVehicleAssignment handles DELETE /api/v1/vehicles/{id}/assignment?date=.
func (h *Handler) DeleteVehicleAssignment(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	a, err := h.service.UnassignDriver(r.Context(), r.PathValue("id"), r.URL.Query().Get("date"), identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toVehicleAssignmentResponse(a))
}

// GetOutletManager handles GET /api/v1/outlets/{id}/manager.
func (h *Handler) GetOutletManager(w http.ResponseWriter, r *http.Request) {
	m, ok, err := h.service.OutletManager(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"manager": nil})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"manager": toOutletManagerResponse(m)})
}

// PutOutletManager handles PUT /api/v1/outlets/{id}/manager.
func (h *Handler) PutOutletManager(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req assignManagerRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	m, err := h.service.AssignManager(r.Context(), AssignManagerInput{
		OutletID: r.PathValue("id"), UserID: req.UserID,
	}, identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOutletManagerResponse(m))
}

// DeleteOutletManager handles DELETE /api/v1/outlets/{id}/manager.
func (h *Handler) DeleteOutletManager(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	m, err := h.service.UnassignManager(r.Context(), r.PathValue("id"), identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOutletManagerResponse(m))
}

// GetOwnAssignment handles GET /api/v1/driver/assignment. The driver id is the
// authenticated caller; a client cannot query another driver's assignment.
func (h *Handler) GetOwnAssignment(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	a, ok, err := h.service.DriverAssignment(r.Context(), identity.UserID, r.URL.Query().Get("date"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"assignment": nil})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"assignment": toVehicleAssignmentResponse(a)})
}

// --- mapping ---------------------------------------------------------------

func toVehicleAssignmentResponse(a VehicleAssignment) vehicleAssignmentResponse {
	return vehicleAssignmentResponse{
		VehicleID: a.VehicleID, DriverID: a.Driver.UserID, DriverName: a.Driver.Name,
		Email: a.Driver.Email, Date: a.AssignmentDate, DepotID: a.DepotID,
	}
}

func toOutletManagerResponse(m OutletManager) outletManagerResponse {
	return outletManagerResponse{
		OutletID: m.OutletID, UserID: m.UserID, Name: m.Name, Email: m.Email, DepotID: m.DepotID,
	}
}

// writeError maps assignment errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Not found")
	case errors.Is(err, ErrConflict):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, err.Error())
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
