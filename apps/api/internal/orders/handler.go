package orders

import (
	"errors"
	"net/http"
	"strconv"
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
//	GET  /api/v1/orders            — filtered, paged list (dispatcher, store manager)
//	POST /api/v1/orders/close      — close the planning queue (dispatcher)
//	GET  /api/v1/orders/queue      — confirmed orders in the closed queue (dispatcher)
//	GET  /api/v1/orders/{id}       — read one, scope-enforced
//	POST /api/v1/orders/{id}/confirm — confirm before the cutoff
//
// GET/POST /orders/{id}/receipt live in internal/receipts, which owns the GRN.
// GET /orders/{id}/eta is not implemented.
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
	// Only a store manager (for their own outlet) or a dispatcher (acting
	// across outlets, per docs/api.md) may create an order. Loaders and drivers
	// have no order-intake role, so they are refused here before any handler
	// runs — the outlet check inside Create is not the only guard.
	mux.Handle("POST /api/v1/orders", h.auth.RequireAnyRole(
		[]domain.Role{domain.RoleStoreManager, domain.RoleDispatcher}, http.HandlerFunc(h.Create)))
	// Listing is for the roles that work with orders as orders. Loaders and
	// drivers reach their orders through their routes instead.
	mux.Handle("GET /api/v1/orders", h.auth.RequireAnyRole(
		[]domain.Role{domain.RoleDispatcher, domain.RoleStoreManager}, http.HandlerFunc(h.List)))
	mux.Handle("GET /api/v1/orders/{id}", h.auth.RequireAuthenticated(http.HandlerFunc(h.Get)))
	mux.Handle("POST /api/v1/orders/{id}/confirm", h.auth.RequireAuthenticated(http.HandlerFunc(h.Confirm)))
	// Closing the planning queue is a dispatcher action; the queue read is for
	// the dispatcher's planning board.
	mux.Handle("POST /api/v1/orders/close", h.auth.RequireRole(
		domain.RoleDispatcher, http.HandlerFunc(h.CloseQueue)))
	mux.Handle("GET /api/v1/orders/queue", h.auth.RequireRole(
		domain.RoleDispatcher, http.HandlerFunc(h.Queue)))
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
	SKU                  string  `json:"sku"`
	Name                 string  `json:"name"`
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
	Deferral               *deferralResponse   `json:"deferral,omitempty"`
}

// deferralResponse is the store-facing reason an order was deferred, from the
// latest deferral_log row.
type deferralResponse struct {
	ReasonText     string `json:"reasonText"`
	ConstraintCode string `json:"constraintCode,omitempty"`
	DecidedAt      string `json:"decidedAt"`
	DeferredToDate string `json:"deferredToDate,omitempty"`
}

func toResponse(o Order) orderResponse {
	lines := make([]orderLineResponse, 0, len(o.Lines))
	for _, ln := range o.Lines {
		lines = append(lines, orderLineResponse{
			OrderItemID:          ln.OrderItemID,
			ItemID:               ln.ItemID,
			SKU:                  ln.SKU,
			Name:                 ln.Name,
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
		Deferral:               toDeferralResponse(o.Deferral),
	}
}

func toDeferralResponse(d *Deferral) *deferralResponse {
	if d == nil {
		return nil
	}
	return &deferralResponse{
		ReasonText:     d.ReasonText,
		ConstraintCode: d.ConstraintCode,
		DecidedAt:      d.DecidedAt.Format(time.RFC3339),
		DeferredToDate: d.DeferredToDate,
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

type listResponse struct {
	Orders []orderResponse `json:"orders"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// List handles GET /api/v1/orders. Every filter is applied in SQL, so a page
// and its total always describe the same filtered set.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	q := r.URL.Query()
	filter := Filter{
		OutletID: strings.TrimSpace(q.Get("outletId")),
		Status:   strings.TrimSpace(q.Get("status")),
		Brand:    strings.TrimSpace(q.Get("brand")),
		DepotID:  strings.TrimSpace(q.Get("depotId")),
		District: strings.TrimSpace(q.Get("district")),
		Search:   q.Get("search"),
	}
	if v := q.Get("deliveryDate"); v != "" {
		d, err := parseDate(v)
		if err != nil {
			httpx.WriteValidation(w, []httpx.FieldError{{Field: "deliveryDate", Message: "must be YYYY-MM-DD"}})
			return
		}
		filter.DeliveryDate = &d
	}
	for _, p := range []struct {
		name string
		dst  *int
	}{{"limit", &filter.Limit}, {"offset", &filter.Offset}} {
		if v := q.Get(p.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				httpx.WriteValidation(w, []httpx.FieldError{{Field: p.name, Message: "must be a whole number"}})
				return
			}
			*p.dst = n
		}
	}

	page, err := h.service.ListPage(r.Context(), filter, scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}
	out := listResponse{Orders: make([]orderResponse, 0, len(page.Orders)), Total: page.Total, Limit: page.Limit, Offset: page.Offset}
	for _, o := range page.Orders {
		out.Orders = append(out.Orders, toResponse(o))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// closeQueueRequest is the body of POST /orders/close. `brands` narrows the
// close to the brands the dispatcher confirmed; empty means every brand.
type closeQueueRequest struct {
	Date    string   `json:"date"`
	DepotID string   `json:"depotId"`
	Brands  []string `json:"brands,omitempty"`
}

type closeQueueResponse struct {
	Date          string       `json:"date"`
	DepotID       string       `json:"depotId"`
	Brands        []string     `json:"brands"`
	Closed        int          `json:"closed"`
	AlreadyClosed []string     `json:"alreadyClosed"`
	Queue         listResponse `json:"queue"`
}

// queueResponse is the closed-queue read: the confirmed orders for a date and
// depot plus the brands whose queue is already closed.
type queueResponse struct {
	Orders       []orderResponse `json:"orders"`
	Total        int             `json:"total"`
	Limit        int             `json:"limit"`
	Offset       int             `json:"offset"`
	ClosedBrands []string        `json:"closedBrands"`
}

// CloseQueue handles POST /api/v1/orders/close. It freezes the queue for the
// given delivery date and depot, per brand when `brands` is supplied and for
// every brand otherwise. Closing is idempotent; the response reports the closure
// state and the resulting queue contents so the board can go read-only.
func (h *Handler) CloseQueue(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req closeQueueRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	date, err := parseDate(req.Date)
	if err != nil {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "date", Message: "must be YYYY-MM-DD"}})
		return
	}
	brands := make([]domain.Brand, 0, len(req.Brands))
	for _, b := range req.Brands {
		brands = append(brands, domain.Brand(strings.TrimSpace(b)))
	}

	result, err := h.service.CloseQueue(r.Context(), CloseQueueInput{
		Date: date, DepotID: req.DepotID, Brands: brands, Actor: identity.UserID,
	}, scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}

	// The closed queue is the CONFIRMED orders for the date at the depot.
	page, err := h.service.ListPage(r.Context(), Filter{
		DeliveryDate: &date,
		DepotID:      req.DepotID,
		Status:       string(domain.OrderConfirmed),
		Limit:        maxPageSize,
	}, scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}
	queue := listResponse{Orders: make([]orderResponse, 0, len(page.Orders)), Total: page.Total, Limit: page.Limit, Offset: page.Offset}
	for _, o := range page.Orders {
		queue.Orders = append(queue.Orders, toResponse(o))
	}

	httpx.WriteJSON(w, http.StatusOK, closeQueueResponse{
		Date: result.Date, DepotID: result.DepotID, Brands: result.Brands,
		Closed: result.Closed, AlreadyClosed: result.AlreadyClosed, Queue: queue,
	})
}

// Queue handles GET /api/v1/orders/queue?date=&depotId=&brand=. It returns the
// confirmed orders in the closed queue for a date and depot, so the planning
// board can list them read-only after the queue is closed.
func (h *Handler) Queue(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	q := r.URL.Query()
	dateStr := q.Get("date")
	if dateStr == "" {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "date", Message: "is required (YYYY-MM-DD)"}})
		return
	}
	date, err := parseDate(dateStr)
	if err != nil {
		httpx.WriteValidation(w, []httpx.FieldError{{Field: "date", Message: "must be YYYY-MM-DD"}})
		return
	}
	page, err := h.service.ListPage(r.Context(), Filter{
		DeliveryDate: &date,
		DepotID:      q.Get("depotId"),
		Brand:        q.Get("brand"),
		District:     q.Get("district"),
		Status:       string(domain.OrderConfirmed),
		Limit:        maxPageSize,
	}, scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}
	closedBrands, err := h.service.ClosedBrands(r.Context(), date, q.Get("depotId"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := queueResponse{
		Orders:       make([]orderResponse, 0, len(page.Orders)),
		Total:        page.Total,
		Limit:        page.Limit,
		Offset:       page.Offset,
		ClosedBrands: closedBrands,
	}
	for _, o := range page.Orders {
		out.Orders = append(out.Orders, toResponse(o))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
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
