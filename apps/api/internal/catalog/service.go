package catalog

import (
	"context"
	"fmt"

	"waypoint.lk/api/internal/domain"
)

// Service is the catalogue's business surface. It validates lookups, applies
// the read rules and returns domain items; it never reaches into HTTP types.
//
// Downstream packages (orders, planning) depend on Service, not on the
// repository or SQL, so catalogue access stays in one place.
type Service struct {
	repo Repository
}

// NewService builds a catalogue service over the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Get returns the item with itemID, or ErrNotFound.
func (s *Service) Get(ctx context.Context, itemID string) (Item, error) {
	if itemID == "" {
		return Item{}, ValidationError{Field: "itemId", Message: "is required"}
	}
	return s.repo.GetByID(ctx, itemID)
}

// GetBySKU returns the item with the given SKU, or ErrNotFound.
func (s *Service) GetBySKU(ctx context.Context, sku string) (Item, error) {
	if sku == "" {
		return Item{}, ValidationError{Field: "sku", Message: "is required"}
	}
	return s.repo.GetBySKU(ctx, sku)
}

// List returns items matching filter. A filter with an unknown enum value is
// rejected before the query runs, so a typo is a clear error rather than an
// empty result.
func (s *Service) List(ctx context.Context, filter Filter) ([]Item, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, filter)
}

// ResolveMany fetches the items for a set of ids in one pass and reports any
// that are missing. Order creation needs the whole set or none: an order may
// not reference an unknown SKU, so this fails with ErrNotFound naming the first
// missing id rather than silently dropping it.
//
// It is here — not in the order service — so the "items must exist" check lives
// with the catalogue, matching docs/api.md ("check items exist and brands
// match").
func (s *Service) ResolveMany(ctx context.Context, itemIDs []string) (map[string]Item, error) {
	resolved := make(map[string]Item, len(itemIDs))
	for _, id := range itemIDs {
		if _, seen := resolved[id]; seen {
			continue
		}
		item, err := s.repo.GetByID(ctx, id)
		if err != nil {
			if isNotFound(err) {
				return nil, fmt.Errorf("%w: item %s", ErrNotFound, id)
			}
			return nil, err
		}
		resolved[id] = item
	}
	return resolved, nil
}

// BrandsMatch reports whether every listed item has the expected brand. Order
// lines must belong to the order's brand, so this is a catalogue-owned check
// the order service can call rather than re-implementing.
func BrandsMatch(items []Item, want domain.Brand) bool {
	for _, it := range items {
		if it.Brand != want {
			return false
		}
	}
	return true
}
