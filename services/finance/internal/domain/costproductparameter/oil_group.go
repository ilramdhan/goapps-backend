package costproductparameter

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// OilNameParamCode is the mst_parameter code whose value must be an oil RM group
// allowed for the product's type (oil-cost-rm-group, D3/D4).
const OilNameParamCode = "OIL_NAME"

// ErrOilGroupNotAllowed is returned when an OIL_NAME value is not in the set of
// oil groups allowed for the product's type.
var ErrOilGroupNotAllowed = errors.New("oil group not allowed for product type")

// OilGroupRule is the allowed/default oil RM group configuration of a product's
// type. A nil rule means the product type has no oil class.
type OilGroupRule struct {
	TypeCode string
	OilClass string
	Allowed  []string
	Default  string
}

// OilGroupPolicy resolves the oil group rule for products.
type OilGroupPolicy interface {
	// RuleForProduct returns the rule for one product; nil when its type has no oil class.
	RuleForProduct(ctx context.Context, productSysID int64) (*OilGroupRule, error)
	// RulesForProducts returns rules keyed by product; products without an oil class are absent.
	RulesForProducts(ctx context.Context, productSysIDs []int64) (map[int64]*OilGroupRule, error)
}

// IsAllowed reports whether code (trimmed) is in the allowed set. A nil rule
// allows everything.
func (r *OilGroupRule) IsAllowed(code string) bool {
	if r == nil {
		return true
	}
	code = strings.TrimSpace(code)
	for _, a := range r.Allowed {
		if a == code {
			return true
		}
	}
	return false
}

// Validate returns ErrOilGroupNotAllowed (wrapped with a user-facing message)
// when code is not allowed for the rule's product type. A nil rule, or an empty
// code (the default applies), is valid.
func (r *OilGroupRule) Validate(code string) error {
	code = strings.TrimSpace(code)
	if r == nil || code == "" {
		return nil
	}
	if r.IsAllowed(code) {
		return nil
	}
	return &OilGroupError{Message: fmt.Sprintf("OIL_NAME %q is not allowed for product type %s; allowed: %s",
		code, r.TypeCode, strings.Join(r.Allowed, ", "))}
}

// OilGroupError carries the user-facing OIL_NAME validation message and
// unwraps to ErrOilGroupNotAllowed (errors.Is works).
type OilGroupError struct {
	Message string
}

// Error returns the user-facing message.
func (e *OilGroupError) Error() string { return e.Message }

// Unwrap returns ErrOilGroupNotAllowed.
func (e *OilGroupError) Unwrap() error { return ErrOilGroupNotAllowed }
