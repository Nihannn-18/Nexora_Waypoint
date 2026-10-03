// Package domain holds the vocabulary shared with the web client.
//
// Every identifier here has a counterpart in libs/shared-types. The wire format
// is the UPPER_SNAKE string in both directions — never an ordinal — so a
// mismatch fails loudly in an integration test instead of silently shifting
// meaning. If you rename something here, rename it there in the same commit.
package domain

// ConstraintCode identifies one hard feasibility rule.
//
// These are the only reasons an allocation may be rejected. Mirrors
// CONSTRAINT_CODES in libs/shared-types/src/lib/constraints.ts.
type ConstraintCode string

const (
	ConstraintVehicleUnavailable ConstraintCode = "VEHICLE_UNAVAILABLE"
	ConstraintDepotMismatch      ConstraintCode = "DEPOT_MISMATCH"
	ConstraintBrandDistrictMix   ConstraintCode = "BRAND_DISTRICT_MIX"
	ConstraintReeferRequired     ConstraintCode = "REEFER_REQUIRED"
	ConstraintVanOnlyAccess      ConstraintCode = "VAN_ONLY_ACCESS"
	ConstraintOrderSplit         ConstraintCode = "ORDER_SPLIT_FORBIDDEN"
	ConstraintTripNumberInvalid  ConstraintCode = "TRIP_NUMBER_INVALID"
	ConstraintTripLimitExceeded  ConstraintCode = "TRIP_LIMIT_EXCEEDED"
	ConstraintWeightExceeded     ConstraintCode = "WEIGHT_CAPACITY_EXCEEDED"
	ConstraintVolumeExceeded     ConstraintCode = "VOLUME_CAPACITY_EXCEEDED"
	ConstraintFreshTimeBudget    ConstraintCode = "FRESH_TIME_BUDGET"
	ConstraintStyleTechBudget    ConstraintCode = "STYLE_TECH_TIME_BUDGET"
	ConstraintWindowMissed       ConstraintCode = "DELIVERY_WINDOW_MISSED"
	ConstraintMallWindowMissed   ConstraintCode = "MALL_WINDOW_MISSED"
	ConstraintFuelQuotaExceeded  ConstraintCode = "FUEL_QUOTA_EXCEEDED"
	ConstraintDuplicateAssign    ConstraintCode = "DUPLICATE_ASSIGNMENT"
	ConstraintNonOperatingDay    ConstraintCode = "NON_OPERATING_DAY"
)

// DesignRuleID is the E-0x group shown in the dispatcher's D-03 rule panel.
// Kept because the Hackathon is scored partly on fidelity to the Day 5 design.
type DesignRuleID string

// DesignRuleOf maps a constraint to the rule-panel group it appears under.
var DesignRuleOf = map[ConstraintCode]DesignRuleID{
	ConstraintWeightExceeded:     "E-01",
	ConstraintVolumeExceeded:     "E-01",
	ConstraintOrderSplit:         "E-01",
	ConstraintDuplicateAssign:    "E-01",
	ConstraintReeferRequired:     "E-02",
	ConstraintVanOnlyAccess:      "E-03",
	ConstraintDepotMismatch:      "E-04",
	ConstraintTripLimitExceeded:  "E-05",
	ConstraintTripNumberInvalid:  "E-05",
	ConstraintFreshTimeBudget:    "E-05",
	ConstraintStyleTechBudget:    "E-05",
	ConstraintBrandDistrictMix:   "E-05",
	ConstraintVehicleUnavailable: "E-05",
	ConstraintNonOperatingDay:    "E-05",
	ConstraintWindowMissed:       "E-06",
	ConstraintMallWindowMissed:   "E-06",
	ConstraintFuelQuotaExceeded:  "E-07",
}

// ConstraintResult is one rule's verdict. The dispatcher's rule panel renders
// every result, passed or not, so the plan is explainable rather than opaque.
type ConstraintResult struct {
	Code   ConstraintCode `json:"code"`
	Passed bool           `json:"passed"`
	// Detail names the binding quantity when Passed is false,
	// e.g. "2840 / 2500 kg". Empty otherwise.
	Detail string `json:"detail,omitempty"`
}

// Brand is one of the three retail brands sharing the fleet.
type Brand string

const (
	BrandFresh Brand = "FRESH"
	BrandStyle Brand = "STYLE"
	BrandTech  Brand = "TECH"
)

// BrandValues lists the three canonical brands.
var BrandValues = []Brand{BrandFresh, BrandStyle, BrandTech}

// Valid reports whether b is one of the three canonical brands.
func (b Brand) Valid() bool {
	for _, known := range BrandValues {
		if b == known {
			return true
		}
	}
	return false
}

// Role is one of the four user roles a judge must be able to sign in as.
type Role string

const (
	RoleDispatcher   Role = "DISPATCHER"
	RoleLoader       Role = "LOADER"
	RoleDriver       Role = "DRIVER"
	RoleStoreManager Role = "STORE_MANAGER"
)

// RoleValues lists the four canonical roles. Used to validate data loaded from
// the database or, in future, a verification bridge.
var RoleValues = []Role{RoleDispatcher, RoleLoader, RoleDriver, RoleStoreManager}

// Valid reports whether r is one of the four canonical roles. A zero or unknown
// role is never valid, so an identity with no role fails authentication rather
// than being treated as some implicit default.
func (r Role) Valid() bool {
	for _, known := range RoleValues {
		if r == known {
			return true
		}
	}
	return false
}

// OrderStatus tracks an order through its lifecycle:
//
//	PLACED → CONFIRMED → ALLOCATED → LOADED → IN_TRANSIT → DELIVERED → RECEIVED
//	                           └──────────────→ DEFERRED → next planning run
type OrderStatus string

const (
	OrderPlaced    OrderStatus = "PLACED"
	OrderConfirmed OrderStatus = "CONFIRMED"
	OrderAllocated OrderStatus = "ALLOCATED"
	OrderLoaded    OrderStatus = "LOADED"
	OrderInTransit OrderStatus = "IN_TRANSIT"
	OrderDelivered OrderStatus = "DELIVERED"
	OrderFailed    OrderStatus = "FAILED"
	OrderReceived  OrderStatus = "RECEIVED"
	OrderDeferred  OrderStatus = "DEFERRED"
)

// TempRequirement is what an order needs; CHILLED and FROZEN both demand a reefer.
type TempRequirement string

const (
	TempAmbient TempRequirement = "AMBIENT"
	TempChilled TempRequirement = "CHILLED"
	TempFrozen  TempRequirement = "FROZEN"
)

// TempRequirementValues lists the three canonical temperature requirements.
var TempRequirementValues = []TempRequirement{TempAmbient, TempChilled, TempFrozen}

// Valid reports whether t is one of the three canonical requirements.
func (t TempRequirement) Valid() bool {
	for _, known := range TempRequirementValues {
		if t == known {
			return true
		}
	}
	return false
}

// RequiresReefer reports whether this requirement can only be met by a
// refrigerated vehicle. A reefer may also carry ambient; the reverse is never true.
func (t TempRequirement) RequiresReefer() bool {
	return t == TempChilled || t == TempFrozen
}

// ParkingConstraint mirrors the parking_constraint column of outlets.csv.
// It is one column with three values, not two independent flags.
type ParkingConstraint string

const (
	// ParkingNormal: any vehicle may serve the outlet.
	ParkingNormal ParkingConstraint = "NORMAL"
	// ParkingVanOnly: trucks cannot reach the outlet; only a van may be allocated.
	ParkingVanOnly ParkingConstraint = "VAN_ONLY"
	// ParkingMallDock: access is limited to the mall's fixed delivery window.
	ParkingMallDock ParkingConstraint = "MALL_DOCK"
)
