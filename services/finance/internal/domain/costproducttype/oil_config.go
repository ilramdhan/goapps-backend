package costproducttype

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Oil class values mirror cost_product_type.cpt_oil_class (CHECK chk_cpt_oil_class).
const (
	OilClassPTY     = "PTY"
	OilClassPOY     = "POY"
	OilClassSuperba = "SUPERBA"
)

var (
	// ErrInvalidOilClass is returned for an oil class outside PTY/POY/SUPERBA/"".
	ErrInvalidOilClass = errors.New("oil_class must be one of PTY, POY, SUPERBA or empty")
	// ErrOilConfigNoGroups is returned when a type with an oil class has no allowed group.
	ErrOilConfigNoGroups = errors.New("a product type with an oil class needs at least one allowed oil group")
	// ErrOilConfigDefaultCount is returned when the allowed groups do not have exactly one default.
	ErrOilConfigDefaultCount = errors.New("exactly one allowed oil group must be the default")
	// ErrOilConfigGroupsWithoutClass is returned when groups are sent for a type without oil class.
	ErrOilConfigGroupsWithoutClass = errors.New("oil groups can only be set when oil_class is set")
	// ErrOilConfigDuplicateGroup is returned when the same group code appears twice.
	ErrOilConfigDuplicateGroup = errors.New("duplicate oil group code")
	// ErrOilConfigGroupNotOil is returned when a group code is not an active oil RM group.
	ErrOilConfigGroupNotOil = errors.New("group is not an active oil RM group")
)

// OilGroupEntry is one allowed oil RM group of a product type.
type OilGroupEntry struct {
	GroupCode string
	GroupName string
	IsDefault bool
}

// OilConfig is the oil class + allowed oil groups of a product type.
type OilConfig struct {
	TypeID   int32
	OilClass string
	Groups   []OilGroupEntry
}

// ResolvedOilGroup is an active oil RM group resolved from its code.
type ResolvedOilGroup struct {
	GroupHeadID string
	GroupCode   string
	GroupName   string
}

// OilConfigRepository persists the product-type oil configuration
// (cost_product_type.cpt_oil_class + cost_product_type_oil_group).
type OilConfigRepository interface {
	// GetOilConfig returns the oil class and mapped groups of a type (ErrNotFound when absent).
	GetOilConfig(ctx context.Context, typeID int32) (*OilConfig, error)
	// ResolveOilGroups returns the active oil RM groups (is_oil_group, active, not
	// deleted) among codes, keyed by code. Unknown / non-oil codes are absent.
	ResolveOilGroups(ctx context.Context, codes []string) (map[string]ResolvedOilGroup, error)
	// ReplaceOilConfig atomically sets the class and replaces all mapped groups.
	ReplaceOilConfig(ctx context.Context, typeID int32, oilClass string, groups []ReplaceOilGroup, actor string) error
}

// ReplaceOilGroup is one group row written by ReplaceOilConfig.
type ReplaceOilGroup struct {
	GroupHeadID string
	IsDefault   bool
}

// NormalizeOilConfig trims/uppercases the class, trims group codes, and
// validates the shape rules of spec §4.7 (class set ⇒ ≥1 group and exactly one
// default; class empty ⇒ no groups; no duplicate codes). Existence of the
// groups as oil groups is checked by the application layer.
func NormalizeOilConfig(oilClass string, groups []OilGroupEntry) (string, []OilGroupEntry, error) {
	class := strings.ToUpper(strings.TrimSpace(oilClass))
	switch class {
	case "", OilClassPTY, OilClassPOY, OilClassSuperba:
	default:
		return "", nil, ErrInvalidOilClass
	}
	out := make([]OilGroupEntry, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	defaults := 0
	for _, g := range groups {
		code := strings.TrimSpace(g.GroupCode)
		if seen[code] {
			return "", nil, fmt.Errorf("%w: %s", ErrOilConfigDuplicateGroup, code)
		}
		seen[code] = true
		if g.IsDefault {
			defaults++
		}
		out = append(out, OilGroupEntry{GroupCode: code, GroupName: g.GroupName, IsDefault: g.IsDefault})
	}
	if class == "" {
		if len(out) > 0 {
			return "", nil, ErrOilConfigGroupsWithoutClass
		}
		return class, out, nil
	}
	if len(out) == 0 {
		return "", nil, ErrOilConfigNoGroups
	}
	if defaults != 1 {
		return "", nil, ErrOilConfigDefaultCount
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GroupCode < out[j].GroupCode })
	return class, out, nil
}
