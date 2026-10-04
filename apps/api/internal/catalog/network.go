package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// This file serves the network reference the dispatcher plans against: depots,
// outlets and vehicles. It is read-only reference data seeded from the supplied
// files; nothing here writes, and no feasibility rule is evaluated here — the
// planning engine and the validator stay the only authorities on that.

// Depot is one of the two depots. DepotID is the stable internal id the
// planning, route and audit endpoints filter by; Code is the display key.
type Depot struct {
	DepotID string
	Code    string
	Name    string
}

// Outlet mirrors outlets.csv plus the seeded display name.
type Outlet struct {
	OutletID          string
	Name              string
	Brand             string
	District          string
	DepotID           string
	DockType          string
	ParkingConstraint string
	// MallWindowOpen/Close are "HH:MM", empty unless the outlet is in a mall.
	MallWindowOpen  string
	MallWindowClose string
	WindowOpenTime  string
	WindowCloseTime string
}

// Vehicle mirrors vehicles.csv plus its availability status on a date.
type Vehicle struct {
	VehicleID        string
	Type             string
	TempClass        string
	WeightCapKg      float64
	VolumeCapM3      float64
	FuelType         string
	KmPerL           float64
	WeeklyFuelQuotaL float64
	DepotID          string
	// Status is the day's availability (AVAILABLE, IN_WORKSHOP, BREAKDOWN). A
	// vehicle with no availability row for the date is available, the same
	// convention the planning loader uses.
	Status string
}

// NetworkReader loads the network reference. Filters left empty mean "all".
type NetworkReader interface {
	Depots(ctx context.Context) ([]Depot, error)
	Outlets(ctx context.Context, depotID string) ([]Outlet, error)
	Vehicles(ctx context.Context, depotID string, date time.Time) ([]Vehicle, error)
}

// PGNetworkReader is the PostgreSQL-backed NetworkReader.
type PGNetworkReader struct {
	pool *pgxpool.Pool
}

// NewPGNetworkReader builds a reader over the given pool.
func NewPGNetworkReader(pool *pgxpool.Pool) *PGNetworkReader {
	return &PGNetworkReader{pool: pool}
}

// Depots implements NetworkReader.
func (r *PGNetworkReader) Depots(ctx context.Context) ([]Depot, error) {
	rows, err := r.pool.Query(ctx, `SELECT depot_id::text, code, name FROM depot WHERE is_active ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list depots: %w", err)
	}
	defer rows.Close()
	out := make([]Depot, 0, 2)
	for rows.Next() {
		var d Depot
		if err := rows.Scan(&d.DepotID, &d.Code, &d.Name); err != nil {
			return nil, fmt.Errorf("scan depot: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Outlets implements NetworkReader. depot_id is compared as text so a malformed
// id matches nothing rather than failing the UUID cast.
func (r *PGNetworkReader) Outlets(ctx context.Context, depotID string) ([]Outlet, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT outlet_id, name, brand, district, depot_id::text, dock_type, parking_constraint,
		       COALESCE(to_char(mall_window_open, 'HH24:MI'), ''),
		       COALESCE(to_char(mall_window_close, 'HH24:MI'), ''),
		       to_char(window_open_time, 'HH24:MI'), to_char(window_close_time, 'HH24:MI')
		FROM outlet
		WHERE $1 = '' OR depot_id::text = $1
		ORDER BY outlet_id`, depotID)
	if err != nil {
		return nil, fmt.Errorf("list outlets: %w", err)
	}
	defer rows.Close()
	out := make([]Outlet, 0)
	for rows.Next() {
		var o Outlet
		if err := rows.Scan(&o.OutletID, &o.Name, &o.Brand, &o.District, &o.DepotID, &o.DockType,
			&o.ParkingConstraint, &o.MallWindowOpen, &o.MallWindowClose, &o.WindowOpenTime,
			&o.WindowCloseTime); err != nil {
			return nil, fmt.Errorf("scan outlet: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Vehicles implements NetworkReader.
func (r *PGNetworkReader) Vehicles(ctx context.Context, depotID string, date time.Time) ([]Vehicle, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT v.vehicle_id, v.type, v.temp, v.weight_cap_kg, v.volume_cap_m3, v.fuel_type,
		       v.km_per_l, v.weekly_fuel_quota_l, v.depot_id::text, COALESCE(a.status, 'AVAILABLE')
		FROM vehicle v
		LEFT JOIN vehicle_daily_availability a ON a.vehicle_id = v.vehicle_id AND a.date = $2
		WHERE $1 = '' OR v.depot_id::text = $1
		ORDER BY v.vehicle_id`, depotID, date)
	if err != nil {
		return nil, fmt.Errorf("list vehicles: %w", err)
	}
	defer rows.Close()
	out := make([]Vehicle, 0)
	for rows.Next() {
		var v Vehicle
		if err := rows.Scan(&v.VehicleID, &v.Type, &v.TempClass, &v.WeightCapKg, &v.VolumeCapM3,
			&v.FuelType, &v.KmPerL, &v.WeeklyFuelQuotaL, &v.DepotID, &v.Status); err != nil {
			return nil, fmt.Errorf("scan vehicle: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Clock reports the API's current business-time instant. Vehicle availability
// defaults to "today" on the API clock, never the wall clock, because the
// seeded demo day is in the past.
type Clock interface{ Now() time.Time }

// NetworkHandler serves the dispatcher's network reference endpoints.
//
//	GET /api/v1/depots                       -> { depots }
//	GET /api/v1/outlets?depotId=             -> { outlets }
//	GET /api/v1/vehicles?depotId=&date=      -> { vehicles } with the date's status
//
// It also serves the Dispatcher master-data mutations (see
// RegisterMasterDataRoutes): vehicle/outlet create and update, validated
// server-side and audited. writer is required for those routes.
type NetworkHandler struct {
	reader NetworkReader
	writer MasterDataWriter
	clock  Clock
	auth   *auth.Middleware
}

// MasterDataWriter is the write surface the handler needs. It is satisfied by
// *PGNetworkWriter and by test fakes.
type MasterDataWriter interface {
	VehicleWriter
	OutletWriter
}

// NewNetworkHandler builds the handler for the read endpoints.
func NewNetworkHandler(reader NetworkReader, clock Clock, authMiddleware *auth.Middleware) *NetworkHandler {
	return &NetworkHandler{reader: reader, clock: clock, auth: authMiddleware}
}

// WithWriter attaches the master-data writer, enabling the mutation routes. It
// returns the handler for chaining at the composition root.
func (h *NetworkHandler) WithWriter(writer MasterDataWriter) *NetworkHandler {
	h.writer = writer
	return h
}

// RegisterRoutes mounts the endpoints behind role authorization. A dispatcher
// plans both depots, so the whole network is theirs to read. A store manager
// reads outlets too, but the handler pins the result to the caller's own outlet
// (app_user.outlet_id) regardless of any query parameter — the scope comes from
// the authenticated identity, never the request.
func (h *NetworkHandler) RegisterRoutes(mux *http.ServeMux) {
	d := domain.RoleDispatcher
	mux.Handle("GET /api/v1/depots", h.auth.RequireRole(d, http.HandlerFunc(h.ListDepots)))
	mux.Handle("GET /api/v1/outlets", h.auth.RequireAnyRole(
		[]domain.Role{d, domain.RoleStoreManager}, http.HandlerFunc(h.ListOutlets)))
	mux.Handle("GET /api/v1/vehicles", h.auth.RequireRole(d, http.HandlerFunc(h.ListVehicles)))
}

type depotResponse struct {
	DepotID string `json:"depotId"`
	Code    string `json:"code"`
	Name    string `json:"name"`
}

type clockWindow struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

type outletResponse struct {
	OutletID          string       `json:"outletId"`
	Name              string       `json:"name"`
	Brand             string       `json:"brand"`
	District          string       `json:"district"`
	DepotID           string       `json:"depotId"`
	DockType          string       `json:"dockType"`
	ParkingConstraint string       `json:"parkingConstraint"`
	MallWindow        *clockWindow `json:"mallWindow"`
	WindowOpenTime    string       `json:"windowOpenTime"`
	WindowCloseTime   string       `json:"windowCloseTime"`
}

type vehicleResponse struct {
	VehicleID        string  `json:"vehicleId"`
	Type             string  `json:"type"`
	TempClass        string  `json:"tempClass"`
	WeightCapKg      float64 `json:"weightCapKg"`
	VolumeCapM3      float64 `json:"volumeCapM3"`
	FuelType         string  `json:"fuelType"`
	KmPerL           float64 `json:"kmPerL"`
	WeeklyFuelQuotaL float64 `json:"weeklyFuelQuotaL"`
	DepotID          string  `json:"depotId"`
	Status           string  `json:"status"`
}

// ListDepots handles GET /api/v1/depots.
func (h *NetworkHandler) ListDepots(w http.ResponseWriter, r *http.Request) {
	depots, err := h.reader.Depots(r.Context())
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	out := make([]depotResponse, 0, len(depots))
	for _, d := range depots {
		out = append(out, depotResponse{DepotID: d.DepotID, Code: d.Code, Name: d.Name})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"depots": out})
}

// ListOutlets handles GET /api/v1/outlets.
//
// A dispatcher reads the whole network, optionally narrowed by ?depotId. A
// store manager is pinned to the outlet on their app_user record: the handler
// ignores any client-supplied depotId, queries without that filter, and returns
// only outlets the authenticated identity may access. A store manager can never
// widen their scope — or request another outlet — through the query string.
func (h *NetworkHandler) ListOutlets(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}

	depotFilter := strings.TrimSpace(r.URL.Query().Get("depotId"))
	if identity.Role == domain.RoleStoreManager {
		// Scope comes from the identity, never the request: drop any depot filter
		// a store manager supplied and select only their own outlet below.
		depotFilter = ""
	}

	outlets, err := h.reader.Outlets(r.Context(), depotFilter)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	out := make([]outletResponse, 0, len(outlets))
	for _, o := range outlets {
		if !identity.CanAccessOutlet(o.OutletID) {
			continue
		}
		row := outletResponse{
			OutletID: o.OutletID, Name: o.Name, Brand: o.Brand, District: o.District,
			DepotID: o.DepotID, DockType: o.DockType, ParkingConstraint: o.ParkingConstraint,
			WindowOpenTime: o.WindowOpenTime, WindowCloseTime: o.WindowCloseTime,
		}
		if o.MallWindowOpen != "" && o.MallWindowClose != "" {
			row.MallWindow = &clockWindow{Open: o.MallWindowOpen, Close: o.MallWindowClose}
		}
		out = append(out, row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlets": out})
}

// ListVehicles handles GET /api/v1/vehicles. date defaults to today on the API
// clock.
func (h *NetworkHandler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	date := h.clock.Now()
	if v := strings.TrimSpace(q.Get("date")); v != "" {
		d, err := time.Parse("2006-01-02", v)
		if err != nil {
			httpx.WriteValidation(w, []httpx.FieldError{{Field: "date", Message: "must be YYYY-MM-DD"}})
			return
		}
		date = d
	}
	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	vehicles, err := h.reader.Vehicles(r.Context(), strings.TrimSpace(q.Get("depotId")), day)
	if err != nil {
		writeNetworkError(w, err)
		return
	}
	out := make([]vehicleResponse, 0, len(vehicles))
	for _, v := range vehicles {
		out = append(out, vehicleResponse{
			VehicleID: v.VehicleID, Type: v.Type, TempClass: v.TempClass, WeightCapKg: v.WeightCapKg,
			VolumeCapM3: v.VolumeCapM3, FuelType: v.FuelType, KmPerL: v.KmPerL,
			WeeklyFuelQuotaL: v.WeeklyFuelQuotaL, DepotID: v.DepotID, Status: wireVehicleStatus(v.Status),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicles": out})
}

// wireVehicleStatus maps the stored availability status to the wire enum in
// libs/shared-types (VEHICLE_STATUSES). The table spells a breakdown
// BREAKDOWN; the contract spells it BROKEN_DOWN.
func wireVehicleStatus(s string) string {
	if s == "BREAKDOWN" {
		return "BROKEN_DOWN"
	}
	return s
}

func writeNetworkError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrDuplicate):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "That identifier already exists")
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Not found")
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
