package catalog

import (
	"errors"
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Handler serves the read-only catalogue endpoints. It depends on a Service and
// the auth Middleware, both injected, so the HTTP layer stays thin: parse the
// request, authorise, call the service, write the response.
type Handler struct {
	service *Service
	auth    *auth.Middleware
}

// NewHandler builds the catalogue handler. auth must be non-nil so every route
// is authenticated; the catalogue has no anonymous surface.
func NewHandler(service *Service, authMiddleware *auth.Middleware) *Handler {
	return &Handler{service: service, auth: authMiddleware}
}

// itemResponse is the wire shape for one item. Camelcase field names match the
// `Item` read model in libs/shared-types.
type itemResponse struct {
	ItemID                 string  `json:"itemId"`
	SKU                    string  `json:"sku"`
	Name                   string  `json:"name"`
	Brand                  string  `json:"brand"`
	Category               string  `json:"category,omitempty"`
	UnitWeightKg           float64 `json:"unitWeightKg"`
	UnitVolumeM3           float64 `json:"unitVolumeM3"`
	TemperatureRequirement string  `json:"temperatureRequirement"`
}

type listItemsResponse struct {
	Items []itemResponse `json:"items"`
}

func toResponse(i Item) itemResponse {
	return itemResponse{
		ItemID:                 i.ItemID,
		SKU:                    i.SKU,
		Name:                   i.Name,
		Brand:                  string(i.Brand),
		Category:               i.Category,
		UnitWeightKg:           i.UnitWeightKg,
		UnitVolumeM3:           i.UnitVolumeM3,
		TemperatureRequirement: string(i.TemperatureRequirement),
	}
}

// RegisterRoutes mounts the catalogue endpoints on the API mux.
//
// Both routes require an authenticated caller. Any role may read the catalogue:
// a store manager needs it to place an order, the dispatcher and loader need the
// dimensions, and there is no sensitive data in a SKU. Role-specific rules, if
// ever needed, belong here rather than duplicated in each feature handler.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/items", h.auth.RequireAuthenticated(http.HandlerFunc(h.List)))
	mux.Handle("GET /api/v1/items/{id}", h.auth.RequireAuthenticated(http.HandlerFunc(h.Get)))
}

// List handles GET /api/v1/items with optional brand, temperature and search
// query parameters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := Filter{
		Brand:       domain.Brand(q.Get("brand")),
		Temperature: domain.TempRequirement(q.Get("temperature")),
		Search:      q.Get("search"),
	}

	items, err := h.service.List(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}

	out := listItemsResponse{Items: make([]itemResponse, 0, len(items))}
	for _, it := range items {
		out.Items = append(out.Items, toResponse(it))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Get handles GET /api/v1/items/{id}. `id` is the item UUID; a SKU lookup is
// available with ?sku= so callers can resolve either identifier.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if sku := r.URL.Query().Get("sku"); sku != "" {
		item, err := h.service.GetBySKU(r.Context(), sku)
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toResponse(item))
		return
	}

	id := r.PathValue("id")
	item, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(item))
}

// writeError maps catalogue errors onto the shared HTTP error contract:
//
//	not found       -> 404 NOT_FOUND
//	invalid input   -> 400 VALIDATION_FAILED (with the offending field)
//	anything else   -> 500 INTERNAL_ERROR
//
// No SQL or internal error text is returned to the client.
func writeError(w http.ResponseWriter, err error) {
	var invalid ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteErrorCode(w, http.StatusNotFound, httpx.CodeNotFound, "Catalogue item not found")
	case errors.As(err, &invalid):
		httpx.WriteValidation(w, []httpx.FieldError{{Field: invalid.Field, Message: invalid.Message}})
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
