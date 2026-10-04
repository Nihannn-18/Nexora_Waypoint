package useradmin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the Dispatcher account-management surface and the public
// self-service password-reset endpoints.
//
//	GET    /api/v1/users                      -> list operational accounts
//	POST   /api/v1/users                      -> create an operational account
//	GET    /api/v1/users/{id}                 -> one account
//	PATCH  /api/v1/users/{id}                 -> edit permitted fields
//	POST   /api/v1/users/{id}/deactivate      -> deactivate (revokes sessions)
//	POST   /api/v1/users/{id}/activate        -> reactivate
//	POST   /api/v1/auth/forgot-password       -> request a reset (generic reply)
//	POST   /api/v1/auth/reset-password        -> consume a reset token
//
// Every /users route is Dispatcher-only. The reset endpoints are public: they
// are a self-service authentication flow and must not require dispatcher
// privileges.
type Handler struct {
	service *Service
	auth    *auth.Middleware
	// devResetLog, when true, logs the raw reset token so a demo/judge run can
	// complete the reset without an email provider. It is enabled only under the
	// explicit DEMO_MODE flag (off for real operation), and the token is never
	// returned in a response body.
	devResetLog bool
}

// NewHandler builds the handler. devResetLog should be true only under the
// explicit DEMO_MODE flag; it is off for real operation.
func NewHandler(service *Service, authMiddleware *auth.Middleware, devResetLog bool) *Handler {
	return &Handler{service: service, auth: authMiddleware, devResetLog: devResetLog}
}

// RegisterRoutes mounts the account-management and password-reset endpoints.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	d := domain.RoleDispatcher
	mux.Handle("GET /api/v1/users", h.auth.RequireRole(d, http.HandlerFunc(h.ListUsers)))
	mux.Handle("POST /api/v1/users", h.auth.RequireRole(d, http.HandlerFunc(h.CreateUser)))
	mux.Handle("GET /api/v1/users/{id}", h.auth.RequireRole(d, http.HandlerFunc(h.GetUser)))
	mux.Handle("PATCH /api/v1/users/{id}", h.auth.RequireRole(d, http.HandlerFunc(h.UpdateUser)))
	mux.Handle("POST /api/v1/users/{id}/deactivate", h.auth.RequireRole(d, http.HandlerFunc(h.DeactivateUser)))
	mux.Handle("POST /api/v1/users/{id}/activate", h.auth.RequireRole(d, http.HandlerFunc(h.ActivateUser)))

	// Public: a self-service flow, not a dispatcher action.
	mux.HandleFunc("POST /api/v1/auth/forgot-password", h.ForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", h.ResetPassword)
}

// --- wire types ------------------------------------------------------------

// userResponse mirrors ManagedUser in libs/shared-types/src/lib/entities.ts.
// It never carries a password, password hash, session token or reset token.
type userResponse struct {
	UserID      string  `json:"userId"`
	Email       string  `json:"email"`
	DisplayName string  `json:"displayName"`
	Role        string  `json:"role"`
	DepotID     *string `json:"depotId"`
	OutletID    *string `json:"outletId"`
	Active      bool    `json:"active"`
	CreatedAt   string  `json:"createdAt"`
}

type createUserRequest struct {
	Email           string `json:"email"`
	DisplayName     string `json:"displayName"`
	Role            string `json:"role"`
	DepotID         string `json:"depotId"`
	OutletID        string `json:"outletId"`
	InitialPassword string `json:"initialPassword"`
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	DepotID     *string `json:"depotId"`
	OutletID    *string `json:"outletId"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token           string `json:"token"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

// genericForgotMessage is returned whether or not the email exists, so the
// endpoint cannot be used to enumerate accounts.
const genericForgotMessage = "If the account exists, password reset instructions have been provided."

func toUserResponse(a Account) userResponse {
	return userResponse{
		UserID:      a.UserID,
		Email:       a.Email,
		DisplayName: a.DisplayName,
		Role:        string(a.Role),
		DepotID:     optionalID(a.DepotID),
		OutletID:    optionalID(a.OutletID),
		Active:      a.Active,
		CreatedAt:   a.CreatedAt,
	}
}

func optionalID(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// --- handlers --------------------------------------------------------------

// ListUsers handles GET /api/v1/users. Optional `role` and `active` filters.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	role := domain.Role(strings.TrimSpace(r.URL.Query().Get("role")))
	var active *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("active")); raw != "" {
		value := raw == "true"
		active = &value
	}
	accounts, err := h.service.ListAccounts(r.Context(), role, active)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]userResponse, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, toUserResponse(a))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": out})
}

// GetUser handles GET /api/v1/users/{id}.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	account, err := h.service.Account(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserResponse(account))
}

// CreateUser handles POST /api/v1/users. The role is validated server-side
// against the allowed operational set; DISPATCHER is refused.
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req createUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	account, err := h.service.CreateAccount(r.Context(), CreateInput{
		Email:           req.Email,
		DisplayName:     req.DisplayName,
		Role:            domain.Role(strings.TrimSpace(req.Role)),
		DepotID:         strings.TrimSpace(req.DepotID),
		OutletID:        strings.TrimSpace(req.OutletID),
		InitialPassword: req.InitialPassword,
	}, identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toUserResponse(account))
}

// UpdateUser handles PATCH /api/v1/users/{id}. Only permitted profile/assignment
// fields are editable; role and password are not.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req updateUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	account, err := h.service.UpdateAccount(r.Context(), r.PathValue("id"), UpdateInput{
		DisplayName: req.DisplayName,
		DepotID:     req.DepotID,
		OutletID:    req.OutletID,
	}, identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserResponse(account))
}

// DeactivateUser handles POST /api/v1/users/{id}/deactivate.
func (h *Handler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	h.setActive(w, r, false)
}

// ActivateUser handles POST /api/v1/users/{id}/activate.
func (h *Handler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	h.setActive(w, r, true)
}

func (h *Handler) setActive(w http.ResponseWriter, r *http.Request, active bool) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	account, err := h.service.SetActive(r.Context(), r.PathValue("id"), active, identity.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserResponse(account))
}

// ForgotPassword handles POST /api/v1/auth/forgot-password. The response is
// always the same generic success, so it cannot be used to enumerate accounts.
//
// This project has no email provider, so a reset link cannot be delivered by
// mail. Under the explicit DEMO_MODE flag the raw token is logged so a demo/judge
// run can complete the flow; with DEMO_MODE off it is discarded and only the
// generic reply is returned. The token is NEVER placed in the response body.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	rawToken, matched, err := h.service.ForgotPassword(r.Context(), req.Email)
	if err != nil {
		// A malformed email is still answered generically so account shape cannot
		// be probed; only a genuine server fault is surfaced.
		if errors.Is(err, ErrInvalid) {
			respondForgot(w, h.devResetLog, "", false)
			return
		}
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
		return
	}
	respondForgot(w, h.devResetLog, rawToken, matched)
}

// respondForgot emits the generic reply. If devLog is on and a token was
// minted, the token is written to the log only (never the response).
func respondForgot(w http.ResponseWriter, devLog bool, rawToken string, matched bool) {
	if devLog && matched && rawToken != "" {
		// Demo only (DEMO_MODE): the raw token is logged so the reset flow can be
		// completed without an email provider. It is never returned to a client.
		slog.Info("demo password reset token issued", "resetToken", rawToken)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"message": genericForgotMessage})
}

// ResetPassword handles POST /api/v1/auth/reset-password. An invalid, expired,
// already-used or inactive-account token is a generic 400 with a single message;
// the cases are indistinguishable to the client.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	if err := h.service.ResetPassword(r.Context(), ResetInput{
		Token:           req.Token,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	}); err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "Your password has been reset. Sign in with your new password.",
	})
}

// writeError maps useradmin errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrDuplicateEmail):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "An account with that email already exists")
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Account not found")
	case errors.Is(err, ErrInvalidResetToken):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "token", Message: "is invalid or has expired"}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
