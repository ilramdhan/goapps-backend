package erprule

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Rule is one immutable valloss rule inside a RuleSet.
type Rule struct {
	Key     RuleKey
	Basis   Basis
	ValLoss decimal.Decimal
}

// Price is one immutable sell price inside a RuleSet.
type Price struct {
	Basis Basis
	Price decimal.Decimal
}

// GradeAssignment maps one ERP grade code to its grade group.
type GradeAssignment struct {
	GradeCode string
	Group     GradeGroup
}

// RuleSet is the immutable snapshot of all active rules, sell prices and
// grade-group assignments that one derive run reads (design Part 1 §5.2,
// Part 2 §6.2, §6.6). It is built once (from LoadRuleSet's single snapshot)
// and never changes; accessors return copies.
//
// Duplicate rule keys are kept, not rejected, so that a lookup on such a key
// fails with ErrRuleAmbiguous instead of guessing. Duplicate sell-price
// bases or grade codes are rejected at construction (they are primary /
// unique keys in PostgreSQL).
type RuleSet struct {
	rules  map[RuleKey][]Rule
	prices map[Basis]decimal.Decimal
	grades map[string]GradeGroup
}

// NewRuleSet validates every entry and builds the snapshot. Inputs are the
// active rows only; ordering does not matter.
func NewRuleSet(rules []Rule, prices []Price, grades []GradeAssignment) (*RuleSet, error) {
	rs := &RuleSet{
		rules:  make(map[RuleKey][]Rule, len(rules)),
		prices: make(map[Basis]decimal.Decimal, len(prices)),
		grades: make(map[string]GradeGroup, len(grades)),
	}
	for _, r := range rules {
		if err := validateRule(r); err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.Key, err)
		}
		rs.rules[r.Key] = append(rs.rules[r.Key], r)
	}
	for _, p := range prices {
		if err := validateSellPriceBasis(p.Basis); err != nil {
			return nil, fmt.Errorf("sell price %q: %w", p.Basis, err)
		}
		if err := ValidatePrice(p.Price); err != nil {
			return nil, fmt.Errorf("sell price %q: %w", p.Basis, err)
		}
		if _, dup := rs.prices[p.Basis]; dup {
			return nil, fmt.Errorf("%w: sell price %q repeated", ErrDuplicateRule, p.Basis)
		}
		rs.prices[p.Basis] = p.Price
	}
	for _, g := range grades {
		code, err := ValidateGradeCode(g.GradeCode)
		if err != nil || code != g.GradeCode {
			return nil, fmt.Errorf("grade %q: %w", g.GradeCode, ErrInvalidGradeCode)
		}
		if parsed, err := ParseGradeGroup(string(g.Group)); err != nil || parsed != g.Group {
			return nil, fmt.Errorf("grade %q: %w: %q", g.GradeCode, ErrInvalidGradeGroup, g.Group)
		}
		if _, dup := rs.grades[code]; dup {
			return nil, fmt.Errorf("%w: grade %q repeated", ErrDuplicateRule, code)
		}
		rs.grades[code] = g.Group
	}
	return rs, nil
}

func validateRule(r Rule) error {
	if err := validateKey(r.Key); err != nil {
		return err
	}
	if err := validateBasis(r.Basis); err != nil {
		return err
	}
	return ValidateValLoss(r.ValLoss)
}

// Lookup returns the single active rule for the key. It returns
// ErrRuleMissing (V-08, NO_RULE) when none exists and ErrRuleAmbiguous when
// more than one does.
func (rs *RuleSet) Lookup(key RuleKey) (Rule, error) {
	found := rs.rules[key]
	switch len(found) {
	case 0:
		return Rule{}, fmt.Errorf("%w: %s", ErrRuleMissing, key)
	case 1:
		return found[0], nil
	default:
		return Rule{}, fmt.Errorf("%w: %s (%d rules)", ErrRuleAmbiguous, key, len(found))
	}
}

// Loss returns the basis and value loss (as stored, 6 dp; the derivation
// engine applies R5) for (fg type, prod type, grade group). Errors are those
// of Lookup, or of key validation for an invalid or AX key.
func (rs *RuleSet) Loss(fgType FgType, prodType ProdType, group GradeGroup) (Basis, decimal.Decimal, error) {
	key := RuleKey{FgType: fgType, ProdType: prodType, GradeGroup: group}
	if err := validateKey(key); err != nil {
		return "", decimal.Zero, err
	}
	r, err := rs.Lookup(key)
	if err != nil {
		return "", decimal.Zero, err
	}
	return r.Basis, r.ValLoss, nil
}

// SellPrice returns the reference price (as stored, 6 dp) for a
// selling-price basis. COST is ErrInvalidBasis; a missing price is
// ErrSellPriceMissing (V-08, NO_SELL_PRICE).
func (rs *RuleSet) SellPrice(basis Basis) (decimal.Decimal, error) {
	if !basis.IsSellPrice() {
		return decimal.Zero, fmt.Errorf("%w: %q has no sell price", ErrInvalidBasis, basis)
	}
	p, ok := rs.prices[basis]
	if !ok {
		return decimal.Zero, fmt.Errorf("%w: %s", ErrSellPriceMissing, basis)
	}
	return p, nil
}

// GradeGroupOf returns the group assigned to an ERP grade code, and false
// when the grade has no group (V-08, NO_GRADE_GROUP).
func (rs *RuleSet) GradeGroupOf(gradeCode string) (GradeGroup, bool) {
	g, ok := rs.grades[gradeCode]
	return g, ok
}

// Rules returns every rule sorted by (fg type, prod type, grade group), with
// duplicates (ambiguous keys) adjacent and ordered by basis then value loss.
func (rs *RuleSet) Rules() []Rule {
	out := make([]Rule, 0, rs.RuleCount())
	for _, rr := range rs.rules {
		out = append(out, rr...)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Key != b.Key {
			return a.Key.Less(b.Key)
		}
		if a.Basis != b.Basis {
			return a.Basis < b.Basis
		}
		return a.ValLoss.LessThan(b.ValLoss)
	})
	return out
}

// Prices returns every sell price sorted by basis.
func (rs *RuleSet) Prices() []Price {
	out := make([]Price, 0, len(rs.prices))
	for b, p := range rs.prices {
		out = append(out, Price{Basis: b, Price: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Basis < out[j].Basis })
	return out
}

// GradeAssignments returns every grade assignment sorted by grade code.
func (rs *RuleSet) GradeAssignments() []GradeAssignment {
	out := make([]GradeAssignment, 0, len(rs.grades))
	for c, g := range rs.grades {
		out = append(out, GradeAssignment{GradeCode: c, Group: g})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GradeCode < out[j].GradeCode })
	return out
}

// RuleCount returns the number of rules, counting duplicates.
func (rs *RuleSet) RuleCount() int {
	n := 0
	for _, rr := range rs.rules {
		n += len(rr)
	}
	return n
}

// PriceCount returns the number of sell prices.
func (rs *RuleSet) PriceCount() int { return len(rs.prices) }

// GradeCount returns the number of grade assignments.
func (rs *RuleSet) GradeCount() int { return len(rs.grades) }

// AmbiguousKeys returns the keys with more than one rule, sorted.
func (rs *RuleSet) AmbiguousKeys() []RuleKey {
	var out []RuleKey
	for k, rr := range rs.rules {
		if len(rr) > 1 {
			out = append(out, k)
		}
	}
	sortKeys(out)
	return out
}

// RulesMissingPrice returns the keys of rules whose selling-price basis has
// no sell price (the service-side basis-to-price check, design §4.1), sorted.
func (rs *RuleSet) RulesMissingPrice() []RuleKey {
	var out []RuleKey
	for k, rr := range rs.rules {
		for _, r := range rr {
			if _, ok := rs.prices[r.Basis]; r.Basis.IsSellPrice() && !ok {
				out = append(out, k)
				break
			}
		}
	}
	sortKeys(out)
	return out
}

func sortKeys(keys []RuleKey) {
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
}
