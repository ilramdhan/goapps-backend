package costcalc

import (
	"fmt"
	"slices"
	"strings"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// Oil class values. They mirror cost_product_type.cpt_oil_class (migration
// 000521) and select which arm of F_YARN_OIL_COST / F_YARN_OIL_GAIN applies.
const (
	// OilClassPTY is the PTY oil class (coning oil, OIL_GAIN computed from OPU).
	OilClassPTY = "PTY"
	// OilClassPOY is the POY oil class (spin finish oil, OIL_GAIN from the
	// OIL_GAIN_POY_DEFAULT constant formula).
	OilClassPOY = "POY"
	// OilClassSuperba is the Superba oil class (TCS/TPS/TTS).
	OilClassSuperba = "SUPERBA"
)

// Reserved scope keys the engine injects for the oil formulas (migration
// 000524). Like the SPIN_* keys they are deliberately NOT mst_parameter rows,
// so they never appear on a product's CAPP form.
const (
	// ScopeKeyIsPTY is 1 for a PTY-class product, 0 otherwise.
	ScopeKeyIsPTY = "IS_PTY"
	// ScopeKeyIsPOY is 1 for a POY-class product, 0 otherwise.
	ScopeKeyIsPOY = "IS_POY"
	// ScopeKeyIsSuperba is 1 for a Superba-class product, 0 otherwise.
	ScopeKeyIsSuperba = "IS_SUPERBA"
	// ScopeKeyOilRate is the OIL_RATE param. For an oil-class product the
	// engine overwrites whatever CAPP carried with the RM group's rate for the
	// calc period (CR -> SR -> PR by default, see resolveOilRate).
	ScopeKeyOilRate = "OIL_RATE"
)

// OilInput is the per-product oil context loaded by LoadOilContext. A nil
// *OilInput means the product's type has no oil class: the IS_* flags are all
// 0 and OIL_RATE is left exactly as CAPP supplied it.
type OilInput struct {
	// Class is the product type's oil class: PTY, POY or SUPERBA.
	Class string
	// TypeCode is the product type code (e.g. "TCS"), used in messages only.
	TypeCode string
	// GroupCode is the stored OIL_NAME value (trimmed); "" when empty.
	GroupCode string
	// DefaultGroup is the product type's default oil RM group code; "" when
	// the type has no default mapping.
	DefaultGroup string
	// Allowed is the product type's allowed oil RM group codes. An empty set
	// disables the allowed-set check (only existence of the rate row matters).
	Allowed map[string]bool
}

// injectProductClassFlags writes IS_PTY / IS_POY / IS_SUPERBA into scope as
// float64 1/0 and removes them from zeroFilled, so cpc_param_snapshot records
// the real flag the oil formulas branched on. It always writes all three keys:
// a product with no oil class (oil == nil) gets three zeros, which makes the
// oil formulas evaluate to 0 for it.
func injectProductClassFlags(scope map[string]any, zeroFilled map[string]bool, oil *OilInput) {
	class := ""
	if oil != nil {
		class = oil.Class
	}
	flags := map[string]bool{
		ScopeKeyIsPTY:     class == OilClassPTY,
		ScopeKeyIsPOY:     class == OilClassPOY,
		ScopeKeyIsSuperba: class == OilClassSuperba,
	}
	for key, on := range flags {
		v := float64(0)
		if on {
			v = 1
		}
		scope[key] = v
		delete(zeroFilled, key)
	}
}

// resolveOilRate resolves OIL_RATE for an oil-class product from the oil RM
// group's cst_rm_cost row for the calc period, using the same GROUP-RM cascade
// (in.RMRateOrder, default CR -> SR -> PR) as resolveRMUnitCost.
//
// applied is false when the product has no oil class (in.Oil == nil); the
// caller then leaves OIL_RATE untouched. Every failure wraps
// costcalcdom.ErrMissingRMCost so the chunk processor marks the product
// BLOCKED / MISSING_RM_COST:
//   - no stored OIL_NAME and no type default;
//   - the stored OIL_NAME is not allowed for the product type (legacy data);
//   - no cst_rm_cost row for the group in the period;
//   - the row exists but every cascade rate is zero (user decision D17 —
//     deliberately stricter than resolveRMUnitCost, which treats all-zero as
//     a valid "no rate yet" state).
func resolveOilRate(in ComputeInput) (rate float64, label string, applied bool, err error) {
	oil := in.Oil
	if oil == nil {
		return 0, "", false, nil
	}
	code := strings.TrimSpace(oil.GroupCode)
	if code == "" {
		code = strings.TrimSpace(oil.DefaultGroup)
	}
	if code == "" {
		return 0, "", false, fmt.Errorf("%w: oil group: none configured (type %s)",
			costcalcdom.ErrMissingRMCost, oil.TypeCode)
	}
	if len(oil.Allowed) > 0 && !oil.Allowed[code] {
		return 0, "", false, fmt.Errorf("%w: oil group %s not allowed for type %s",
			costcalcdom.ErrMissingRMCost, code, oil.TypeCode)
	}
	rates, ok := in.RMCosts[code+"|"]
	if !ok {
		return 0, "", false, fmt.Errorf("%w: oil group %s", costcalcdom.ErrMissingRMCost, code)
	}
	rate, label = rmcost.FirstNonZeroWithLabel(rmRateCandidates(in.RMRateOrder, rates))
	if rate <= 0 {
		return 0, "", false, fmt.Errorf("%w: oil group %s: all rates zero",
			costcalcdom.ErrMissingRMCost, code)
	}
	return rate, label, true, nil
}

// oilGroupCodes returns existing plus the distinct oil RM group codes the
// chunk's products may resolve against (stored OIL_NAME and type default), so
// bulkLoad can add them to the LoadRMCosts code list. Codes already present in
// existing are not repeated; the appended codes are sorted so the query
// argument is deterministic regardless of map iteration order.
func oilGroupCodes(oil map[int64]*OilInput, existing []string) []string {
	seen := make(map[string]bool, len(existing))
	for _, c := range existing {
		seen[c] = true
	}
	var extra []string
	add := func(c string) {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			return
		}
		seen[c] = true
		extra = append(extra, c)
	}
	for _, in := range oil {
		if in == nil {
			continue
		}
		add(in.GroupCode)
		add(in.DefaultGroup)
	}
	slices.Sort(extra)
	return append(existing, extra...)
}
