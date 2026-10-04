package demo

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the demo-mode controls (docs/api.md, "Demo mode"):
//
//	POST /api/v1/demo/clock  { stage }  — jump the API clock (dispatcher)
//	POST /api/v1/demo/reset             — clear operational data, re-seed (dispatcher)
//
// The composition root mounts it only when DEMO_MODE is on; with demo mode off
// both paths fall through to the API's 404.
type Handler struct {
	service  *Service
	auth     *auth.Middleware
	timezone string
}

// NewHandler builds the demo handler. timezone is reported back with the new
// clock so the client can update its view without a second /meta call.
func NewHandler(service *Service, authMiddleware *auth.Middleware, timezone string) *Handler {
	return &Handler{service: service, auth: authMiddleware, timezone: timezone}
}

// RegisterRoutes mounts the demo endpoints. Both are dispatcher-only: the
// dispatcher drives the walkthrough.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/demo/clock", h.auth.RequireRole(domain.RoleDispatcher, http.HandlerFunc(h.SetClock)))
	mux.Handle("POST /api/v1/demo/reset", h.auth.RequireRole(domain.RoleDispatcher, http.HandlerFunc(h.Reset)))
}

type setClockRequest struct {
	Stage string `json:"stage"`
}

// clockResponse carries the GET /meta fields plus the stage jumped to, so the
// web app can re-sync its clock offset from this response alone.
type clockResponse struct {
	Stage    string `json:"stage"`
	Now      string `json:"now"`
	DemoMode bool   `json:"demoMode"`
	Timezone string `json:"timezone"`
}

type resetResponse struct {
	Now             string           `json:"now"`
	DemoMode        bool             `json:"demoMode"`
	Timezone        string           `json:"timezone"`
	Cleared         map[string]int64 `json:"cleared"`
	DemoOrders      int              `json:"demoOrders"`
	DemoVehicleDays int              `json:"demoVehicleDays"`
}

// SetClock handles POST /api/v1/demo/clock.
func (h *Handler) SetClock(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req setClockRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}

	stage := Stage(strings.TrimSpace(req.Stage))
	if errs := validateStage(stage); errs != nil {
		httpx.WriteValidation(w, errs)
		return
	}

	now, err := h.service.JumpTo(r.Context(), identity.UserID, identity.DepotID, stage)
	if errors.Is(err, ErrUnknownStage) {
		httpx.WriteValidation(w, validateStage(stage))
		return
	}
	if err != nil {
		slog.Error("demo clock jump failed", "error", err)
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Could not move the demo clock")
		return
	}

	slog.Info("demo clock moved", "stage", stage, "now", now.Format(time.RFC3339), "actor", identity.UserID)
	httpx.WriteJSON(w, http.StatusOK, clockResponse{
		Stage:    string(stage),
		Now:      now.Format(time.RFC3339),
		DemoMode: true,
		Timezone: h.timezone,
	})
}

// validateStage names every known stage when stage is missing or unknown, and
// returns nil when it is valid.
func validateStage(stage Stage) []httpx.FieldError {
	var v httpx.Validation
	allowed := make([]string, 0, len(stageTimes))
	for _, s := range Stages() {
		allowed = append(allowed, string(s))
	}
	v.Required("stage", string(stage))
	v.OneOf("stage", string(stage), allowed...)
	return v.Errors()
}

// Reset handles POST /api/v1/demo/reset. It takes no body.
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}

	sum, now, err := h.service.Reset(r.Context(), identity.UserID, identity.DepotID)
	if err != nil {
		slog.Error("demo reset failed", "error", err)
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Could not reset the demo data; nothing was changed")
		return
	}

	slog.Info("demo data reset", "actor", identity.UserID, "demoOrders", sum.DemoOrders, "now", now.Format(time.RFC3339))
	httpx.WriteJSON(w, http.StatusOK, resetResponse{
		Now:             now.Format(time.RFC3339),
		DemoMode:        true,
		Timezone:        h.timezone,
		Cleared:         sum.Cleared,
		DemoOrders:      sum.DemoOrders,
		DemoVehicleDays: sum.DemoVehicleDays,
	})
}
