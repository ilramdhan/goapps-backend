// Package yarntxweightgroup provides domain logic for TX Weight groups.
package yarntxweightgroup

import (
	"math"
	"unicode/utf8"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// maxRules is the number of supported grades (AE, A9, A, B, C).
const maxRules = 5

// Rule is one grade rule of a group (a mst_yarn_tx_weight row). Grade and
// Mode reuse the yarntxweight value objects so the engine semantics stay one.
type Rule struct {
	Grade       yarntxweight.Grade
	Mode        yarntxweight.Mode
	Value       float64
	Description string
}

// Apply computes this rule's grade weight for axWt.
func (r Rule) Apply(axWt float64) float64 { return r.Mode.Apply(axWt, r.Value) }

// ProductTypeRef is a product type mapped to a group. Code and Name are
// populated on read (joined from cost_product_type).
type ProductTypeRef struct {
	ID   int32
	Code string
	Name string
}

// normalizeRules validates rules and returns them normalized (parsed grade and
// mode) in grade display order.
func normalizeRules(rules []Rule) ([]Rule, error) {
	if len(rules) == 0 || len(rules) > maxRules {
		return nil, ErrInvalidRuleCount
	}
	byGrade := make(map[yarntxweight.Grade]Rule, len(rules))
	for _, r := range rules {
		nr, err := normalizeRule(r)
		if err != nil {
			return nil, err
		}
		if _, dup := byGrade[nr.Grade]; dup {
			return nil, ErrDuplicateGrade
		}
		byGrade[nr.Grade] = nr
	}
	out := make([]Rule, 0, len(byGrade))
	for _, g := range yarntxweight.AllGrades {
		if r, ok := byGrade[g]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func normalizeRule(r Rule) (Rule, error) {
	g, err := yarntxweight.ParseGrade(string(r.Grade))
	if err != nil {
		return Rule{}, err
	}
	m, err := yarntxweight.ParseMode(string(r.Mode))
	if err != nil {
		return Rule{}, err
	}
	if math.IsNaN(r.Value) || math.IsInf(r.Value, 0) {
		return Rule{}, yarntxweight.ErrInvalidValue
	}
	if utf8.RuneCountInString(r.Description) > maxDescriptionLen {
		return Rule{}, yarntxweight.ErrDescriptionTooLong
	}
	return Rule{Grade: g, Mode: m, Value: r.Value, Description: r.Description}, nil
}

// normalizeProductTypeIDs validates ids (min 1, positive, unique) and keeps order.
func normalizeProductTypeIDs(ids []int32) ([]ProductTypeRef, error) {
	if len(ids) == 0 {
		return nil, ErrNoProductTypes
	}
	seen := make(map[int32]struct{}, len(ids))
	out := make([]ProductTypeRef, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidProductType
		}
		if _, dup := seen[id]; dup {
			return nil, ErrDuplicateProductType
		}
		seen[id] = struct{}{}
		out = append(out, ProductTypeRef{ID: id})
	}
	return out, nil
}
