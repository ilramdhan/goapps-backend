package erprule

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// ChangeSubject names the part of a RuleSet a RuleChange is about.
type ChangeSubject string

// Change subjects, in the order Diff reports them.
const (
	SubjectValLossRule ChangeSubject = "VALLOSS_RULE"
	SubjectSellPrice   ChangeSubject = "SELL_PRICE"
	SubjectGradeGroup  ChangeSubject = "GRADE_GROUP"
)

// ChangeKind says how an entry differs between two rule sets.
type ChangeKind string

// Change kinds.
const (
	ChangeAdded   ChangeKind = "ADDED"
	ChangeRemoved ChangeKind = "REMOVED"
	ChangeChanged ChangeKind = "CHANGED"
)

// ChangeValue is one side of a change.
//   - VALLOSS_RULE: Basis and Value (the value loss);
//   - SELL_PRICE:   Basis and Value (the price);
//   - GRADE_GROUP:  Group.
type ChangeValue struct {
	Basis Basis
	Value decimal.Decimal
	Group GradeGroup
}

// RuleChange is one difference between a previous and a current RuleSet
// (plan-03 P2-T5). It feeds the "rules changed since the last VALUATED batch"
// warning that must be acknowledged (V-05 ack, design §7) and the
// GetRuleSnapshotDiff read (design Part 2 §6.6).
//
// Before is empty for ADDED and After is empty for REMOVED. For a valloss
// rule both sides hold every rule of the key, so an ambiguous key (more than
// one active rule) is reported as a whole, ordered by basis then value loss.
// Sell prices and grade groups hold at most one value per side.
type RuleChange struct {
	Subject ChangeSubject
	Kind    ChangeKind
	// Key is the display key: "Type 1/POY/BC", "SPPTY" or the grade code.
	Key string
	// RuleKey is set for VALLOSS_RULE only.
	RuleKey RuleKey
	// Basis is set for SELL_PRICE only.
	Basis Basis
	// GradeCode is set for GRADE_GROUP only.
	GradeCode string
	Before    []ChangeValue
	After     []ChangeValue
}

// String renders the change for logs and messages, decimals as 6-dp text.
func (c RuleChange) String() string {
	return fmt.Sprintf("%s %s %s: %s -> %s", c.Subject, c.Kind, c.Key,
		formatChangeValues(c.Subject, c.Before), formatChangeValues(c.Subject, c.After))
}

func formatChangeValues(s ChangeSubject, vs []ChangeValue) string {
	if len(vs) == 0 {
		return "-"
	}
	var b strings.Builder
	for i, v := range vs {
		if i > 0 {
			b.WriteByte(',')
		}
		if s == SubjectGradeGroup {
			b.WriteString(string(v.Group))
			continue
		}
		b.WriteString(string(v.Basis) + "@" + formatCanonicalDecimal(v.Value))
	}
	return b.String()
}

// Diff returns every difference from prev to cur, in a deterministic order:
// valloss rules by (fg type, prod type, grade group), then sell prices by
// basis, then grade groups by grade code. Decimals compare by value, so 0.05
// and 0.050000 are equal. A nil set is treated as empty (every entry of the
// other side is ADDED or REMOVED); the caller decides whether a first-seen
// set is only informational (V-05, Q11). Equal sets return nil.
func Diff(prev, cur *RuleSet) []RuleChange {
	if prev == nil {
		prev = &RuleSet{}
	}
	if cur == nil {
		cur = &RuleSet{}
	}
	var out []RuleChange
	out = append(out, diffRules(prev, cur)...)
	out = append(out, diffPrices(prev, cur)...)
	out = append(out, diffGrades(prev, cur)...)
	if len(out) == 0 {
		return nil
	}
	return out
}

// DiffSnapshots diffs two stored canonical snapshots (ceib_rule_snapshot).
// A nil or empty prev means no previous snapshot. Errors wrap
// ErrInvalidSnapshot.
func DiffSnapshots(prev, cur []byte) ([]RuleChange, error) {
	var p, c *RuleSet
	var err error
	if len(prev) > 0 {
		if p, err = ParseCanonical(prev); err != nil {
			return nil, fmt.Errorf("previous: %w", err)
		}
	}
	if len(cur) > 0 {
		if c, err = ParseCanonical(cur); err != nil {
			return nil, fmt.Errorf("current: %w", err)
		}
	}
	return Diff(p, c), nil
}

func kindOf(before, after int) ChangeKind {
	switch {
	case before == 0:
		return ChangeAdded
	case after == 0:
		return ChangeRemoved
	default:
		return ChangeChanged
	}
}

func diffRules(prev, cur *RuleSet) []RuleChange {
	keys := make(map[RuleKey]struct{}, len(prev.rules)+len(cur.rules))
	for k := range prev.rules {
		keys[k] = struct{}{}
	}
	for k := range cur.rules {
		keys[k] = struct{}{}
	}
	sorted := make([]RuleKey, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sortKeys(sorted)

	out := make([]RuleChange, 0, len(sorted))
	for _, k := range sorted {
		before, after := ruleValues(prev.rules[k]), ruleValues(cur.rules[k])
		if equalValues(before, after) {
			continue
		}
		out = append(out, RuleChange{
			Subject: SubjectValLossRule,
			Kind:    kindOf(len(before), len(after)),
			Key:     k.String(),
			RuleKey: k,
			Before:  before,
			After:   after,
		})
	}
	return out
}

// ruleValues returns the rules of one key ordered by basis then value loss.
func ruleValues(rr []Rule) []ChangeValue {
	if len(rr) == 0 {
		return nil
	}
	out := make([]ChangeValue, 0, len(rr))
	for _, r := range rr {
		out = append(out, ChangeValue{Basis: r.Basis, Value: r.ValLoss})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Basis != out[j].Basis {
			return out[i].Basis < out[j].Basis
		}
		return out[i].Value.LessThan(out[j].Value)
	})
	return out
}

func equalValues(a, b []ChangeValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Basis != b[i].Basis || a[i].Group != b[i].Group || !a[i].Value.Equal(b[i].Value) {
			return false
		}
	}
	return true
}

func diffPrices(prev, cur *RuleSet) []RuleChange {
	bases := make(map[Basis]struct{}, len(prev.prices)+len(cur.prices))
	for b := range prev.prices {
		bases[b] = struct{}{}
	}
	for b := range cur.prices {
		bases[b] = struct{}{}
	}
	sorted := make([]Basis, 0, len(bases))
	for b := range bases {
		sorted = append(sorted, b)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	out := make([]RuleChange, 0, len(sorted))
	for _, b := range sorted {
		var before, after []ChangeValue
		if p, ok := prev.prices[b]; ok {
			before = []ChangeValue{{Basis: b, Value: p}}
		}
		if p, ok := cur.prices[b]; ok {
			after = []ChangeValue{{Basis: b, Value: p}}
		}
		if equalValues(before, after) {
			continue
		}
		out = append(out, RuleChange{
			Subject: SubjectSellPrice,
			Kind:    kindOf(len(before), len(after)),
			Key:     string(b),
			Basis:   b,
			Before:  before,
			After:   after,
		})
	}
	return out
}

func diffGrades(prev, cur *RuleSet) []RuleChange {
	codes := make(map[string]struct{}, len(prev.grades)+len(cur.grades))
	for c := range prev.grades {
		codes[c] = struct{}{}
	}
	for c := range cur.grades {
		codes[c] = struct{}{}
	}
	sorted := make([]string, 0, len(codes))
	for c := range codes {
		sorted = append(sorted, c)
	}
	sort.Strings(sorted)

	out := make([]RuleChange, 0, len(sorted))
	for _, c := range sorted {
		var before, after []ChangeValue
		if g, ok := prev.grades[c]; ok {
			before = []ChangeValue{{Group: g}}
		}
		if g, ok := cur.grades[c]; ok {
			after = []ChangeValue{{Group: g}}
		}
		if equalValues(before, after) {
			continue
		}
		out = append(out, RuleChange{
			Subject:   SubjectGradeGroup,
			Kind:      kindOf(len(before), len(after)),
			Key:       c,
			GradeCode: c,
			Before:    before,
			After:     after,
		})
	}
	return out
}
