package catalog

import (
	"net/http"
	"strings"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Dispatcher master-data endpoints. Vehicles and outlets are planning inputs, so
// create/update are Dispatcher-only and every field is validated server-side.
// The reads keep their existing role rules (outlets: dispatcher + store manager;
// vehicles: dispatcher). Identity is generated on create and never changed on
// update; there is no delete.

// RegisterMasterDataRoutes mounts the vehicle/outlet mutation endpoints behind
// dispatcher authorization, plus the single-resource reads. It is separate from
// RegisterRoutes so the read surface stays unchanged and the write surface is
// explicit.
func (h *NetworkHandler) RegisterMasterDataRoutes(mux *http.ServeMux) {
	d := domain.RoleDispatcher
	mux.Handle("GET /api/v1/vehicles/{id}", h.auth.RequireRole(d, http.HandlerFunc(h.GetVehicle)))
	mux.Handle("POST /api/v1/vehicles", h.auth.RequireRole(d, http.HandlerFunc(h.CreateVehicle)))
	mux.Handle("PATCH /api/v1/vehicles/{id}", h.auth.RequireRole(d, http.HandlerFunc(h.UpdateVehicle)))
	// Outlet read is dispatcher + store manager (scoped in ListOutlets); the
	// single-outlet read applies the same scope. Mutations are dispatcher-only.
	mux.Handle("GET /api/v1/outlets/{id}", h.auth.RequireAnyRole(
		[]domain.Role{d, domain.RoleStoreManager}, http.HandlerFunc(h.GetOutlet)))
	mux.Handle("POST /api/v1/outlets", h.auth.RequireRole(d, http.HandlerFunc(h.CreateOutlet)))
	mux.Handle("PATCH /api/v1/outlets/{id}", h.auth.RequireRole(d, http.HandlerFunc(h.UpdateOutlet)))
}

// --- wire types ------------------------------------------------------------

type vehicleWriteRequest struct {
	Type             string  `json:"type"`
	TempClass        string  `json:"tempClass"`
	WeightCapKg      float64 `json:"weightCapKg"`
	VolumeCapM3      float64 `json:"volumeCapM3"`
	FuelType         string  `json:"fuelType"`
	KmPerL           float64 `json:"kmPerL"`
	WeeklyFuelQuotaL float64 `json:"weeklyFuelQuotaL"`
	DepotID          string  `json:"depotId"`
}

type outletWriteRequest struct {
	Name              string `json:"name"`
	Brand             string `json:"brand"`
	District          string `json:"district"`
	DepotID           string `json:"depotId"`
	DockType          string `json:"dockType"`
	ParkingConstraint string `json:"parkingConstraint"`
	WindowOpenTime    string `json:"windowOpenTime"`
	WindowCloseTime   string `json:"windowCloseTime"`
	MallWindowOpen    string `json:"mallWindowOpen,omitempty"`
	MallWindowClose   string `json:"mallWindowClose,omitempty"`
}

// --- vehicles --------------------------------------------------------------// GetVehicle handles GET /api/v1/vehicles/{id}.
func (h *NetworkHandler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	detail, err := h.writer.GetVehicle(r.Context(), r.PathValue("id"))
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toVehicleDetailResponse(detail))
}

// CreateVehicle handles POST /api/v1/vehicles. Identity is generated server-side.
func (h *NetworkHandler) CreateVehicle(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req vehicleWriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	id, err := h.writer.NextVehicleID(r.Context())
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	v := vehicleWriteFromRequest(req)
	v.VehicleID = id
	if err := ValidateVehicle(v, true); err != nil {
		writeNetworkError(w, err)
		return
	}
	if ok, err := h.writer.DepotExists(r.Context(), v.DepotID); err != nil {
		writeNetworkError(w, err)
		return
	} else if !ok {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "depotId", Message: "is not a known depot"}})
		return
	}
	detail, err := h.writer.CreateVehicle(r.Context(), v, identity.UserID)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toVehicleDetailResponse(detail))
}

// UpdateVehicle handles PATCH /api/v1/vehicles/{id}. The id is immutable.
func (h *NetworkHandler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req vehicleWriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	v := vehicleWriteFromRequest(req)
	v.VehicleID = r.PathValue("id")
	if err := ValidateVehicle(v, false); err != nil {
		writeNetworkError(w, err)
		return
	}
	if ok, err := h.writer.DepotExists(r.Context(), v.DepotID); err != nil {
		writeNetworkError(w, err)
		return
	} else if !ok {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "depotId", Message: "is not a known depot"}})
		return
	}
	detail, err := h.writer.UpdateVehicle(r.Context(), v, identity.UserID)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toVehicleDetailResponse(detail))
}

func vehicleWriteFromRequest(req vehicleWriteRequest) VehicleWrite {
	return VehicleWrite{
		Type:             domain.VehicleType(strings.TrimSpace(req.Type)),
		TempClass:        domain.VehicleTempClass(strings.TrimSpace(req.TempClass)),
		WeightCapKg:      req.WeightCapKg,
		VolumeCapM3:      req.VolumeCapM3,
		FuelType:         req.FuelType,
		KmPerL:           req.KmPerL,
		WeeklyFuelQuotaL: req.WeeklyFuelQuotaL,
		DepotID:          strings.TrimSpace(req.DepotID),
	}
}

// --- outlets ---------------------------------------------------------------

// GetOutlet handles GET /api/v1/outlets/{id}. A store manager may read only
// their own outlet; anything else is reported as not found, exactly as the list
// endpoint pins scope.
func (h *NetworkHandler) GetOutlet(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	outletID := r.PathValue("id")
	if identity.Role == domain.RoleStoreManager && !identity.CanAccessOutlet(outletID) {
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Outlet not found")
		return
	}
	detail, err := h.writer.GetOutlet(r.Context(), outletID)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOutletDetailResponse(detail))
}

// CreateOutlet handles POST /api/v1/outlets. Identity is generated server-side.
func (h *NetworkHandler) CreateOutlet(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req outletWriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	id, err := h.writer.NextOutletID(r.Context())
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	o := outletWriteFromRequest(req)
	o.OutletID = id
	if err := ValidateOutlet(o, true); err != nil {
		writeNetworkError(w, err)
		return
	}
	if ok, err := h.writer.DepotExists(r.Context(), o.DepotID); err != nil {
		writeNetworkError(w, err)
		return
	} else if !ok {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "depotId", Message: "is not a known depot"}})
		return
	}
	detail, err := h.writer.CreateOutlet(r.Context(), o, identity.UserID)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toOutletDetailResponse(detail))
}

// UpdateOutlet handles PATCH /api/v1/outlets/{id}. The id is immutable.
func (h *NetworkHandler) UpdateOutlet(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req outletWriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	o := outletWriteFromRequest(req)
	o.OutletID = r.PathValue("id")
	if err := ValidateOutlet(o, false); err != nil {
		writeNetworkError(w, err)
		return
	}
	if ok, err := h.writer.DepotExists(r.Context(), o.DepotID); err != nil {
		writeNetworkError(w, err)
		return
	} else if !ok {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "depotId", Message: "is not a known depot"}})
		return
	}
	detail, err := h.writer.UpdateOutlet(r.Context(), o, identity.UserID)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOutletDetailResponse(detail))
}

func outletWriteFromRequest(req outletWriteRequest) OutletWrite {
	return OutletWrite{
		Name:              req.Name,
		Brand:             domain.Brand(strings.TrimSpace(req.Brand)),
		District:          req.District,
		DepotID:           strings.TrimSpace(req.DepotID),
		DockType:          domain.DockType(strings.TrimSpace(req.DockType)),
		ParkingConstraint: domain.ParkingConstraint(strings.TrimSpace(req.ParkingConstraint)),
		WindowOpenTime:    strings.TrimSpace(req.WindowOpenTime),
		WindowCloseTime:   strings.TrimSpace(req.WindowCloseTime),
		MallWindowOpen:    strings.TrimSpace(req.MallWindowOpen),
		MallWindowClose:   strings.TrimSpace(req.MallWindowClose),
	}
}

// --- response mapping ------------------------------------------------------

func toVehicleDetailResponse(v VehicleDetail) vehicleResponse {
	return vehicleResponse{
		VehicleID: v.VehicleID, Type: string(v.Type), TempClass: string(v.TempClass),
		WeightCapKg: v.WeightCapKg, VolumeCapM3: v.VolumeCapM3, FuelType: v.FuelType,
		KmPerL: v.KmPerL, WeeklyFuelQuotaL: v.WeeklyFuelQuotaL, DepotID: v.DepotID,
		Status: wireVehicleStatus(v.Status),
	}
}

func toOutletDetailResponse(o OutletDetail) outletResponse {
	row := outletResponse{
		OutletID: o.OutletID, Name: o.Name, Brand: string(o.Brand), District: o.District,
		DepotID: o.DepotID, DockType: string(o.DockType), ParkingConstraint: string(o.ParkingConstraint),
		WindowOpenTime: o.WindowOpenTime, WindowCloseTime: o.WindowCloseTime,
	}
	if o.MallWindowOpen != "" && o.MallWindowClose != "" {
		row.MallWindow = &clockWindow{Open: o.MallWindowOpen, Close: o.MallWindowClose}
	}
	return row
}
