// Package catalog owns the product/item side of Waypoint: the SKUs a store
// manager orders, with the dimensions and temperature requirements that the
// capacity rules are checked against.
//
// It mirrors the `item` table in docs/data-model.md and the `Item` read model
// in libs/shared-types. The package is deliberately dependency-light: it uses
// internal/domain for shared vocabulary and internal/httpx for transport, and
// depends on nothing in planning, routes, loading or delivery, so downstream
// order and planning code can import it without a cycle.
//
// Derived or client-supplied totals never live here: an order snapshots a
// line's unit weight/volume at order time (see internal/orders), so a catalog
// edit cannot retroactively change a historical order.
package catalog

import (
	"errors"
	"strings"

	"waypoint.lk/api/internal/domain"
)

// Item is one SKU. IDs are server-generated UUIDs; `SKU` is the human key and
// is unique across the catalogue.
//
// This is the domain type; persistence structs never leave the repository.
type Item struct {
	ItemID string
	SKU    string
	Name   string
	Brand  domain.Brand
	// Category is optional free-text grouping; empty when the catalogue has none.
	Category string
	// UnitWeightKg and UnitVolumeM3 are per-unit dimensions, in kilograms and
	// cubic metres respectively — the same units the capacity constraints use.
	UnitWeightKg float64
	UnitVolumeM3 float64
	// TemperatureRequirement is AMBIENT, CHILLED or FROZEN. CHILLED and FROZEN
	// both demand a reefer.
	TemperatureRequirement domain.TempRequirement
}

// RequiresReefer reports whether the item can only travel in a refrigerated
// vehicle. It delegates to the domain rule so the definition lives in one place.
func (i Item) RequiresReefer() bool { return i.TemperatureRequirement.RequiresReefer() }

// Sentinel errors. Handlers map these to the shared HTTP error contract; they
// are not error codes in their own right.
var (
	// ErrNotFound means no catalogue item matches the lookup.
	ErrNotFound = errors.New("catalog: item not found")
	// ErrInvalid means the item or a lookup filter failed validation.
	ErrInvalid = errors.New("catalog: invalid input")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// isNotFound reports whether err wraps ErrNotFound.
func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// Validate checks the invariants the schema enforces, so the domain rejects the
// same bad data the database would — before a round trip. Boundary values are
// accepted (a zero-weight item is permitted by the schema's `>= 0` check).
func (i Item) Validate() error {
	if strings.TrimSpace(i.SKU) == "" {
		return ValidationError{Field: "sku", Message: "is required"}
	}
	if strings.TrimSpace(i.Name) == "" {
		return ValidationError{Field: "name", Message: "is required"}
	}
	if !i.Brand.Valid() {
		return ValidationError{Field: "brand", Message: "must be one of FRESH, STYLE, TECH"}
	}
	if !i.TemperatureRequirement.Valid() {
		return ValidationError{Field: "temperatureRequirement", Message: "must be one of AMBIENT, CHILLED, FROZEN"}
	}
	if i.UnitWeightKg < 0 {
		return ValidationError{Field: "unitWeightKg", Message: "must not be negative"}
	}
	if i.UnitVolumeM3 < 0 {
		return ValidationError{Field: "unitVolumeM3", Message: "must not be negative"}
	}
	return nil
}

// Filter narrows a catalogue listing. Zero values mean "no filter", so an empty
// Filter lists everything. It mirrors the query parameters the read endpoint
// accepts and nothing more.
type Filter struct {
	// Brand restricts to one brand when set.
	Brand domain.Brand
	// Temperature restricts to one requirement when set.
	Temperature domain.TempRequirement
	// Search is a case-insensitive substring match against SKU or name.
	Search string
}

// Validate rejects a filter whose enum values are present but unknown, so a
// typo returns a clear 400 rather than an empty list.
func (f Filter) Validate() error {
	if f.Brand != "" && !f.Brand.Valid() {
		return ValidationError{Field: "brand", Message: "must be one of FRESH, STYLE, TECH"}
	}
	if f.Temperature != "" && !f.Temperature.Valid() {
		return ValidationError{Field: "temperature", Message: "must be one of AMBIENT, CHILLED, FROZEN"}
	}
	return nil
}
