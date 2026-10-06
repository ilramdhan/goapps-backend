package erprule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrInvalidSnapshot is returned by ParseCanonical for bytes that are not a
// valid canonical rule snapshot.
var ErrInvalidSnapshot = errors.New("erprule: invalid canonical rule snapshot")

// CanonicalVersion identifies the canonical snapshot layout. Changing the
// layout (fields, ordering or decimal format) changes every hash, so it must
// be bumped together with a note in the design (§6.6).
const CanonicalVersion = 1

// The canonical snapshot (design Part 2 §6.6). Struct fields are declared in
// lexicographic key order so encoding/json emits sorted keys; json.Marshal
// emits no insignificant whitespace.
type canonicalSnapshot struct {
	GradeGroups []canonicalGrade `json:"grade_groups"`
	Prices      []canonicalPrice `json:"prices"`
	Rules       []canonicalRule  `json:"rules"`
	Version     int              `json:"version"`
}

type canonicalGrade struct {
	GradeCode  string `json:"grade_code"`
	GradeGroup string `json:"grade_group"`
}

type canonicalPrice struct {
	Basis string `json:"basis"`
	Price string `json:"price"`
}

type canonicalRule struct {
	Basis      string `json:"basis"`
	FgType     string `json:"fg_type"`
	GradeGroup string `json:"grade_group"`
	ProdType   string `json:"prod_type"`
	ValLoss    string `json:"val_loss"`
}

// formatCanonicalDecimal writes a decimal as a fixed Scale (6) dp string,
// the stored scale of NUMERIC(20,6). Inputs are validated to at most 6 dp,
// so this never rounds.
func formatCanonicalDecimal(v decimal.Decimal) string {
	return v.StringFixed(Scale)
}

// Canonical returns the canonical JSON of the rule set (design §6.6), the
// exact bytes stored in ceib_rule_snapshot and hashed into ceib_rule_hash:
//   - object keys sorted, no whitespace;
//   - grade_groups ordered by grade code;
//   - prices ordered by basis;
//   - rules ordered by (fg_type, prod_type, grade_group), duplicates of an
//     ambiguous key adjacent and ordered by basis then val_loss;
//   - val_loss and price as fixed 6-dp strings.
//
// The output depends only on the content of the set, never on the order in
// which the rules were supplied.
func (rs *RuleSet) Canonical() []byte {
	snap := canonicalSnapshot{
		GradeGroups: make([]canonicalGrade, 0, rs.GradeCount()),
		Prices:      make([]canonicalPrice, 0, rs.PriceCount()),
		Rules:       make([]canonicalRule, 0, rs.RuleCount()),
		Version:     CanonicalVersion,
	}
	for _, g := range rs.GradeAssignments() {
		snap.GradeGroups = append(snap.GradeGroups, canonicalGrade{
			GradeCode: g.GradeCode, GradeGroup: string(g.Group),
		})
	}
	for _, p := range rs.Prices() {
		snap.Prices = append(snap.Prices, canonicalPrice{
			Basis: string(p.Basis), Price: formatCanonicalDecimal(p.Price),
		})
	}
	for _, r := range rs.Rules() {
		snap.Rules = append(snap.Rules, canonicalRule{
			Basis:      string(r.Basis),
			FgType:     string(r.Key.FgType),
			GradeGroup: string(r.Key.GradeGroup),
			ProdType:   string(r.Key.ProdType),
			ValLoss:    formatCanonicalDecimal(r.ValLoss),
		})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(snap); err != nil {
		// Only strings and ints are encoded; this cannot fail.
		panic(fmt.Sprintf("erprule: canonical encode: %v", err))
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// Hash returns the lower-hex SHA-256 of Canonical() (64 chars, the
// ceib_rule_hash CHAR(64) / GSB_RULE_HASH value).
func (rs *RuleSet) Hash() string {
	sum := sha256.Sum256(rs.Canonical())
	return hex.EncodeToString(sum[:])
}

// ParseCanonical rebuilds a RuleSet from a stored canonical snapshot (for the
// previous-period diff, P2-T5). Every entry is re-validated through
// NewRuleSet; unknown fields and an unknown version are rejected.
func ParseCanonical(b []byte) (*RuleSet, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var snap canonicalSnapshot
	if err := dec.Decode(&snap); err != nil {
		return nil, fmt.Errorf("%w: canonical snapshot: %w", ErrInvalidSnapshot, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: canonical snapshot: trailing data", ErrInvalidSnapshot)
	}
	if snap.Version != CanonicalVersion {
		return nil, fmt.Errorf("%w: canonical snapshot version %d", ErrInvalidSnapshot, snap.Version)
	}
	rules := make([]Rule, 0, len(snap.Rules))
	for _, r := range snap.Rules {
		v, err := decimal.NewFromString(r.ValLoss)
		if err != nil {
			return nil, fmt.Errorf("%w: val_loss %q", ErrInvalidSnapshot, r.ValLoss)
		}
		rules = append(rules, Rule{
			Key: RuleKey{
				FgType:     FgType(r.FgType),
				ProdType:   ProdType(r.ProdType),
				GradeGroup: GradeGroup(r.GradeGroup),
			},
			Basis:   Basis(r.Basis),
			ValLoss: v,
		})
	}
	prices := make([]Price, 0, len(snap.Prices))
	for _, p := range snap.Prices {
		v, err := decimal.NewFromString(p.Price)
		if err != nil {
			return nil, fmt.Errorf("%w: price %q", ErrInvalidSnapshot, p.Price)
		}
		prices = append(prices, Price{Basis: Basis(p.Basis), Price: v})
	}
	grades := make([]GradeAssignment, 0, len(snap.GradeGroups))
	for _, g := range snap.GradeGroups {
		grades = append(grades, GradeAssignment{GradeCode: g.GradeCode, Group: GradeGroup(g.GradeGroup)})
	}
	rs, err := NewRuleSet(rules, prices, grades)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSnapshot, err)
	}
	return rs, nil
}
