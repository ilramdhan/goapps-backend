// Package erprule is the Finance-owned ERP valuation rule master
// (design Part 1 §4.1, §5.1-5.2; Part 2 §6.2, §6.6; plan-03 P2-T1).
//
// It models the three rule sources that replace legacy MGTDAT tables:
//   - VallossRule: basis + value loss per (FG type, prod type, grade group)
//     (cst_erp_valloss_rule, legacy MGT_ITEM_COST_VAL_LOSS);
//   - SellPrice: reference selling price per basis (cst_erp_sell_price,
//     legacy IM_VS_STATIC_VALUE 'ITEMSELLPRIC');
//   - Grade: the ERP grade reference whose group (cost_erp_grade.
//     ceg_grade_group) is assigned only through Grade.AssignGroup.
//
// RuleSet is the immutable snapshot the derivation engine reads. Money and
// rates use github.com/shopspring/decimal only; float32/float64 are forbidden
// in this package by the path-scoped forbidigo rule in .golangci.yml.
package erprule

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// Column limits from migrations 000104 / 000547.
const (
	maxFgTypeLen    = 15 // cevr_fg_type VARCHAR(15)
	maxGradeCodeLen = 20 // ceg_grade_code VARCHAR(20)
	maxUserLen      = 64 // created_by / updated_by VARCHAR(64)

	// Scale is the stored scale of cevr_val_loss / cesp_price NUMERIC(20,6).
	Scale int32 = 6
)

var (
	// maxValLoss is the largest value loss accepted (proto pattern 1-3
	// integer digits, 6 decimals).
	maxValLoss = decimal.RequireFromString("999.999999")
	// maxPriceExclusive is 10^14, the NUMERIC(20,6) integer-part limit.
	maxPriceExclusive = decimal.New(1, 14)
)

// Basis is the valuation basis of a rule (chk_cevr_basis).
type Basis string

// Basis values. COST derives from the AX conversion cost; the SP* bases
// derive from the reference selling price of the same name.
const (
	BasisCost  Basis = "COST"
	BasisSPPTY Basis = "SPPTY"
	BasisSPITY Basis = "SPITY"
	BasisSPBSD Basis = "SPBSD"
)

// ParseBasis parses a basis, rejecting unknown values.
func ParseBasis(s string) (Basis, error) {
	b := Basis(strings.TrimSpace(s))
	switch b {
	case BasisCost, BasisSPPTY, BasisSPITY, BasisSPBSD:
		return b, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidBasis, s)
	}
}

// ParseSellPriceBasis parses a selling-price basis (chk_cesp_basis): COST is
// rejected because it has no price row.
func ParseSellPriceBasis(s string) (Basis, error) {
	b, err := ParseBasis(s)
	if err != nil {
		return "", err
	}
	if !b.IsSellPrice() {
		return "", fmt.Errorf("%w: %q has no sell price", ErrInvalidBasis, s)
	}
	return b, nil
}

// IsSellPrice reports whether the basis is valued from a selling price.
func (b Basis) IsSellPrice() bool {
	return b == BasisSPPTY || b == BasisSPITY || b == BasisSPBSD
}

// String returns the stored value.
func (b Basis) String() string { return string(b) }

// ProdType is the product type of a rule (chk_cevr_prod_type).
type ProdType string

// ProdType values.
const (
	ProdTypePOY ProdType = "POY"
	ProdTypePTY ProdType = "PTY"
	ProdTypeITY ProdType = "ITY"
)

// ParseProdType parses a prod type, rejecting unknown values.
func ParseProdType(s string) (ProdType, error) {
	p := ProdType(strings.TrimSpace(s))
	switch p {
	case ProdTypePOY, ProdTypePTY, ProdTypeITY:
		return p, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidProdType, s)
	}
}

// String returns the stored value.
func (p ProdType) String() string { return string(p) }

// GradeGroup is an ERP grade group (design §4.1: NS/AE/BC/BB/JLT/POYA/AX).
type GradeGroup string

// GradeGroup values.
const (
	GradeGroupNS   GradeGroup = "NS"
	GradeGroupAE   GradeGroup = "AE"
	GradeGroupBC   GradeGroup = "BC"
	GradeGroupBB   GradeGroup = "BB"
	GradeGroupJLT  GradeGroup = "JLT"
	GradeGroupPOYA GradeGroup = "POYA"
	GradeGroupAX   GradeGroup = "AX"
)

// ParseGradeGroup parses a grade group, rejecting unknown values.
func ParseGradeGroup(s string) (GradeGroup, error) {
	g := GradeGroup(strings.TrimSpace(s))
	switch g {
	case GradeGroupNS, GradeGroupAE, GradeGroupBC, GradeGroupBB,
		GradeGroupJLT, GradeGroupPOYA, GradeGroupAX:
		return g, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidGradeGroup, s)
	}
}

// ParseRuleGradeGroup parses a grade group usable on a valloss rule: AX is
// rejected (chk_cevr_grade_group).
func ParseRuleGradeGroup(s string) (GradeGroup, error) {
	g, err := ParseGradeGroup(s)
	if err != nil {
		return "", err
	}
	if g == GradeGroupAX {
		return "", ErrAxRule
	}
	return g, nil
}

// String returns the stored value.
func (g GradeGroup) String() string { return string(g) }

// FgType is the FG type of a rule ('Type 1'..'Type 13'), matched exactly
// against the AX cost row's FG type. It is not a closed set: Finance may add
// a type, so only emptiness and length are checked.
type FgType string

// ParseFgType trims and validates an FG type.
func ParseFgType(s string) (FgType, error) {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > maxFgTypeLen {
		return "", fmt.Errorf("%w: %q", ErrInvalidFgType, s)
	}
	return FgType(t), nil
}

// String returns the stored value.
func (f FgType) String() string { return string(f) }

// RuleKey is the natural key of a valloss rule (uk_cevr_key).
type RuleKey struct {
	FgType     FgType
	ProdType   ProdType
	GradeGroup GradeGroup
}

// NewRuleKey validates and builds a rule key. AX is rejected.
func NewRuleKey(fgType, prodType, gradeGroup string) (RuleKey, error) {
	f, err := ParseFgType(fgType)
	if err != nil {
		return RuleKey{}, err
	}
	p, err := ParseProdType(prodType)
	if err != nil {
		return RuleKey{}, err
	}
	g, err := ParseRuleGradeGroup(gradeGroup)
	if err != nil {
		return RuleKey{}, err
	}
	return RuleKey{FgType: f, ProdType: p, GradeGroup: g}, nil
}

// String renders the key for messages: "Type 1/POY/BC".
func (k RuleKey) String() string {
	return string(k.FgType) + "/" + string(k.ProdType) + "/" + string(k.GradeGroup)
}

// Less orders keys by (fg type, prod type, grade group), the canonical
// ordering of the rule snapshot (design §6.6).
func (k RuleKey) Less(o RuleKey) bool {
	if k.FgType != o.FgType {
		return k.FgType < o.FgType
	}
	if k.ProdType != o.ProdType {
		return k.ProdType < o.ProdType
	}
	return k.GradeGroup < o.GradeGroup
}

// ValidateValLoss checks a value loss: 0 <= v <= 999.999999, at most 6 dp.
// Values with more decimals are rejected, never silently rounded.
func ValidateValLoss(v decimal.Decimal) error {
	if v.IsNegative() || v.GreaterThan(maxValLoss) || !hasMaxScale(v, Scale) {
		return fmt.Errorf("%w: %s", ErrInvalidPercent, v.String())
	}
	return nil
}

// ValidatePrice checks a sell price: 0 < p < 10^14, at most 6 dp.
func ValidatePrice(p decimal.Decimal) error {
	if !p.IsPositive() || p.GreaterThanOrEqual(maxPriceExclusive) || !hasMaxScale(p, Scale) {
		return fmt.Errorf("%w: %s", ErrInvalidPrice, p.String())
	}
	return nil
}

// ValidateGradeCode trims and validates an ERP grade code.
func ValidateGradeCode(code string) (string, error) {
	c := strings.TrimSpace(code)
	if c == "" || len(c) > maxGradeCodeLen {
		return "", fmt.Errorf("%w: %q", ErrInvalidGradeCode, code)
	}
	return c, nil
}

func validateUser(user string) (string, error) {
	u := strings.TrimSpace(user)
	if u == "" || len(u) > maxUserLen {
		return "", ErrUserRequired
	}
	return u, nil
}

func hasMaxScale(d decimal.Decimal, scale int32) bool {
	return d.Equal(d.Truncate(scale))
}
