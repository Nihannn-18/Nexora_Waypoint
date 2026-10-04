package receipts

import (
	"errors"
	"net/http"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the GRN endpoints (S-06).
//
//	GET  /api/v1/orders/{id}/receipt -> expected lines, loader flags, POD, GRN
//	POST /api/v1/orders/{id}/receipt -> record the GRN (store manager)
//
// The read is open to the store manager for their own outlet and to the
// dispatcher; recording a receipt is the store manager's action only.
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the receipts handler.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// RegisterRoutes mounts the receipt endpoints.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/orders/{id}/receipt", h.auth.RequireAnyRole(
		[]domain.Role{domain.RoleStoreManager, domain.RoleDispatcher}, http.HandlerFunc(h.Get)))
	mux.Handle("POST /api/v1/orders/{id}/receipt", h.auth.RequireRole(
		domain.RoleStoreManager, http.HandlerFunc(h.Create)))
}

// --- wire types ------------------------------------------------------------

type loaderFlagResponse struct {
	MissingQty int    `json:"missingQty"`
	DamagedQty int    `json:"damagedQty"`
	PhotoRef   string `json:"photoRef,omitempty"`
}

type expectedLineResponse struct {
	OrderItemID    string              `json:"orderItemId"`
	SKU            string              `json:"sku"`
	Name           string              `json:"name"`
	OrderedQty     int                 `json:"orderedQty"`
	ExpectedQty    int                 `json:"expectedQty"`
	ExpectedSource string              `json:"expectedSource"`
	LoaderFlag     *loaderFlagResponse `json:"loaderFlag,omitempty"`
}

type podResponse struct {
	Outcome      string `json:"outcome"`
	ReceiverName string `json:"receiverName"`
	Signature    string `json:"signature,omitempty"`
	FileRef      string `json:"fileRef,omitempty"`
	OccurredAt   string `json:"occurredAt"`
}

type receiptLineResponse struct {
	OrderItemID string `json:"orderItemId"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	OrderedQty  int    `json:"orderedQty"`
	ExpectedQty int    `json:"expectedQty"`
	ReceivedQty int    `json:"receivedQty"`
	DamagedQty  int    `json:"damagedQty"`
	ShortQty    int    `json:"shortQty"`
	Condition   string `json:"condition"`
}

type issueResponse struct {
	Type        string `json:"type"`
	OrderItemID string `json:"orderItemId"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Quantity    int    `json:"quantity"`
}

type receiptResponse struct {
	ReceiptID      string                `json:"receiptId"`
	OrderID        string                `json:"orderId"`
	Status         string                `json:"status"`
	ReceivedAt     string                `json:"receivedAt"`
	ReceivedBy     string                `json:"receivedBy"`
	ReceivedByName string                `json:"receivedByName,omitempty"`
	Notes          string                `json:"notes,omitempty"`
	Lines          []receiptLineResponse `json:"lines"`
	Issues         []issueResponse       `json:"issues"`
}

type viewResponse struct {
	OrderID         string                 `json:"orderId"`
	OrderNumber     string                 `json:"orderNumber"`
	OrderStatus     string                 `json:"orderStatus"`
	Lines           []expectedLineResponse `json:"lines"`
	ProofOfDelivery *podResponse           `json:"proofOfDelivery,omitempty"`
	Receipt         *receiptResponse       `json:"receipt,omitempty"`
}

type lineRequest struct {
	OrderItemID string `json:"orderItemId"`
	ReceivedQty int    `json:"receivedQty"`
	DamagedQty  int    `json:"damagedQty"`
}

type createRequest struct {
	Lines []lineRequest `json:"lines"`
	Notes string        `json:"notes,omitempty"`
}

// --- handlers --------------------------------------------------------------

// Get handles GET /api/v1/orders/{id}/receipt.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	view, err := h.service.View(r.Context(), r.PathValue("id"), scopeOf(identity))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toViewResponse(view))
}

// Create handles POST /api/v1/orders/{id}/receipt.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	identity, err := auth.MustIdentity(r.Context())
	if err != nil {
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
		return
	}
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteBadRequest(w, err.Error())
		return
	}
	in := Input{Notes: req.Notes, Lines: make([]LineInput, 0, len(req.Lines))}
	for _, l := range req.Lines {
		in.Lines = append(in.Lines, LineInput(l))
	}
	rec, err := h.service.Submit(r.Context(), r.PathValue("id"), identity.UserID, scopeOf(identity), in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toReceiptResponse(rec))
}

func scopeOf(id auth.Identity) Scope { return Scope{Role: id.Role, OutletID: id.OutletID} }

func toViewResponse(v View) viewResponse {
	out := viewResponse{
		OrderID: v.Order.OrderID, OrderNumber: v.Order.OrderNumber, OrderStatus: v.Order.Status,
		Lines: make([]expectedLineResponse, 0, len(v.Order.Lines)),
	}
	for _, l := range v.Order.Lines {
		line := expectedLineResponse{
			OrderItemID: l.OrderItemID, SKU: l.SKU, Name: l.Name,
			OrderedQty: l.OrderedQty, ExpectedQty: l.ExpectedQty(), ExpectedSource: l.ExpectedSource(),
		}
		if l.LoaderFlag != nil {
			flag := loaderFlagResponse(*l.LoaderFlag)
			line.LoaderFlag = &flag
		}
		out.Lines = append(out.Lines, line)
	}
	if v.Pod != nil {
		out.ProofOfDelivery = &podResponse{
			Outcome: v.Pod.Outcome, ReceiverName: v.Pod.ReceiverName,
			Signature: v.Pod.SignatureRef, FileRef: v.Pod.PhotoRef,
			OccurredAt: v.Pod.OccurredAt.Format(time.RFC3339),
		}
	}
	if v.Receipt != nil {
		rec := toReceiptResponse(*v.Receipt)
		out.Receipt = &rec
	}
	return out
}

func toReceiptResponse(r Receipt) receiptResponse {
	out := receiptResponse{
		ReceiptID: r.ReceiptID, OrderID: r.OrderID, Status: r.Status(),
		ReceivedAt: r.ReceivedAt.Format(time.RFC3339), ReceivedBy: r.ReceivedBy,
		ReceivedByName: r.ReceivedByName, Notes: r.Notes,
		Lines:  make([]receiptLineResponse, 0, len(r.Lines)),
		Issues: []issueResponse{},
	}
	for _, l := range r.Lines {
		out.Lines = append(out.Lines, receiptLineResponse{
			OrderItemID: l.OrderItemID, SKU: l.SKU, Name: l.Name,
			OrderedQty: l.OrderedQty, ExpectedQty: l.ExpectedQty,
			ReceivedQty: l.ReceivedQty, DamagedQty: l.DamagedQty,
			ShortQty: l.ShortQty(), Condition: l.Condition(),
		})
	}
	for _, i := range r.Issues() {
		out.Issues = append(out.Issues, issueResponse(i))
	}
	return out
}

// writeError maps receipt errors onto the shared HTTP error contract.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Order not found")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	case errors.Is(err, ErrAlreadyReceived):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "A receipt has already been recorded for this order")
	case errors.Is(err, ErrNotDelivered):
		httpx.WriteErrorCode(w, http.StatusConflict, httpx.CodeConflict, "This order has not been delivered yet, so there is nothing to receive")
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
