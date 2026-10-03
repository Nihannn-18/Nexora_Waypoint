package planning

import (
	"errors"
	"net/http"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the planning suggestion endpoints. Dispatcher-only: planning is
// the dispatcher's action (docs/api.md, "Dispatcher — planning").
//
//	POST /api/v1/allocations/suggest      -> 202 { jobId, status }
//	GET  /api/v1/planning-jobs/{jobId}    -> 200 job status
//	GET  /api/v1/planning-jobs/{jobId}/results -> 200 proposals
//
// Confirmation (POST /allocations/confirm), validation and recalculation are a
// later Routes/Planner concern and are deliberately not implemented here.
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the planning handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the planning endpoints behind dispatcher authorization.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/allocations/suggest",
		h.auth.RequireRole(domain.RoleDispatcher, http.HandlerFunc(h.Suggest)))
	mux.Handle("GET /api/v1/planning-jobs/{jobId}",
		h.auth.RequireRole(domain.RoleDispatcher, http.HandlerFunc(h.Job)))
	mux.Handle("GET /api/v1/planning-jobs/{jobId}/results",
		h.auth.RequireRole(domain.RoleDispatcher, http.HandlerFunc(h.Results)))
}

type suggestRequest struct {
	PlanningDate string `json:"planningDate"`
	DepotID      string `json:"depotId"`
}

type suggestResponse struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

type jobResponse struct {
	JobID        string `json:"jobId"`
	PlanningDate string `json:"planningDate"`
	DepotID      string `json:"depotId"`
	Status       string `json:"status"`
	RequestedBy  string `json:"requestedBy,omitempty"`
	CreatedAt    string `json:"createdAt"`
	CompletedAt  string `json:"completedAt,omitempty"`
	Error        string `json:"errorMessage,omitempty"`
}

type proposalResponse struct {
	OrderID     string  `json:"orderId"`
	Decision    string  `json:"decision"`
	VehicleID   string  `json:"vehicleId,omitempty"`
	TripNo      int     `json:"tripNo,omitempty"`
	Seq         int     `json:"seq,omitempty"`
	TripMinutes float64 `json:"tripMinutes,omitempty"`
	Explanation string  `json:"explanation"`
	Constraint  string  `json:"constraintCode,omitempty"`
}

type resultsResponse struct {
	Job       jobResponse        `json:"job"`
	Proposals []proposalResponse `json:"proposals"`
}

// Suggest handles POST /api/v1/allocations/suggest.
func (h *Handler) Suggest(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req suggestRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	date, err := time.Parse("2006-01-02", req.PlanningDate)
	if err != nil {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "planningDate", Message: "must be YYYY-MM-DD"}})
		return
	}
	job, err := h.service.Suggest(r.Context(), date, req.DepotID, identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	// 202, matching the documented asynchronous contract.
	httpx.WriteJSON(w, http.StatusAccepted, suggestResponse{JobID: job.JobID, Status: job.Status})
}

// Job handles GET /api/v1/planning-jobs/{jobId}.
func (h *Handler) Job(w http.ResponseWriter, r *http.Request) {
	job, err := h.service.Job(r.Context(), r.PathValue("jobId"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toJobResponse(job))
}

// Results handles GET /api/v1/planning-jobs/{jobId}/results.
func (h *Handler) Results(w http.ResponseWriter, r *http.Request) {
	job, proposals, err := h.service.Results(r.Context(), r.PathValue("jobId"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := resultsResponse{Job: toJobResponse(job), Proposals: make([]proposalResponse, 0, len(proposals))}
	for _, p := range proposals {
		out.Proposals = append(out.Proposals, proposalResponse{
			OrderID: p.OrderID, Decision: p.Decision, VehicleID: p.VehicleID,
			TripNo: p.TripNo, Seq: p.Seq, TripMinutes: p.TripMinutes,
			Explanation: p.Explanation, Constraint: string(p.Constraint),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func toJobResponse(j Job) jobResponse {
	out := jobResponse{
		JobID: j.JobID, PlanningDate: j.PlanningDate.Format("2006-01-02"),
		DepotID: j.DepotID, Status: j.Status, RequestedBy: j.RequestedBy,
		CreatedAt: j.CreatedAt.Format(time.RFC3339), Error: j.ErrorMessage,
	}
	if j.CompletedAt != nil {
		out.CompletedAt = j.CompletedAt.Format(time.RFC3339)
	}
	return out
}

// writeError maps planning errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrJobNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Planning job not found")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
