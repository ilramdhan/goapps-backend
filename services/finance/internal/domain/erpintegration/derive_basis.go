package erpintegration

// derive_basis.go holds the basis-selection half of the derivation engine
// (design Part 2 §6.2): prod type, grade group, valloss rule and sell price
// resolution. It is pure: no I/O, no clock, no globals.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// Grade codes with a fixed meaning in the derivation (design §6.2).
const (
	GradeAX   = "AX" // yarn base grade valued from the AX cost row
	GradeMBA  = "A"  // the only valued masterbatch grade (legacy block E)
	prefixLen = 3    // SUBSTR(item,1,3)
)

// basisOutcome is the resolved valuation basis of one derived-grade row.
type basisOutcome struct {
	prodType   erprule.ProdType
	prodIssue  *Issue // V-08w when the prefix fell back to PTY (C-18)
	gradeGroup erprule.GradeGroup
	fgType     string
	basis      erprule.Basis
	valLoss    decimal.Decimal // as stored (6 dp); R5 is applied by the caller
	sellPrice  decimal.Decimal // as stored (6 dp); zero for COST
	status     DeriveStatus
	failure    string // message of the V-08 issue when status != OK
}

// ProdTypeForItem returns the rule prod type of a yarn item code: POY for
// prefix POY, ITY for prefix ITY, else PTY (legacy std-value insert procedure step
// 1). fallback is true when the prefix is not one of POY/PTY/ITY, which the
// engine reports as the V-08w warning (C-18).
func ProdTypeForItem(itemCode string) (prodType erprule.ProdType, fallback bool) {
	prefix := itemPrefix(itemCode)
	switch erprule.ProdType(prefix) {
	case erprule.ProdTypePOY:
		return erprule.ProdTypePOY, false
	case erprule.ProdTypeITY:
		return erprule.ProdTypeITY, false
	case erprule.ProdTypePTY:
		return erprule.ProdTypePTY, false
	default:
		return erprule.ProdTypePTY, true
	}
}

// itemPrefix is SUBSTR(TRIM(item),1,3); shorter codes return themselves.
func itemPrefix(itemCode string) string {
	c := strings.TrimSpace(itemCode)
	if len(c) > prefixLen {
		return c[:prefixLen]
	}
	return c
}

// resolveBasis selects the basis, value loss and sell price of a derived
// yarn grade from the rule set. The first missing input decides the status:
// NO_GRADE_GROUP, then NO_FG_TYPE, then NO_RULE, then NO_SELL_PRICE.
func resolveBasis(itemCode, gradeCode, fgType string, rs *erprule.RuleSet) basisOutcome {
	out := basisOutcome{status: DeriveOK, fgType: strings.TrimSpace(fgType)}
	var fallback bool
	out.prodType, fallback = ProdTypeForItem(itemCode)
	if fallback {
		out.prodIssue = &Issue{
			Code:     IssueV08w,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("item prefix %q is not POY/PTY/ITY; prod type falls back to PTY", itemPrefix(itemCode)),
		}
	}

	if rs == nil {
		return out.fail(DeriveNoGradeGroup, "no rule set loaded")
	}
	group, ok := rs.GradeGroupOf(gradeCode)
	if !ok {
		return out.fail(DeriveNoGradeGroup, fmt.Sprintf("grade %q has no grade group", gradeCode))
	}
	out.gradeGroup = group
	if out.fgType == "" {
		return out.fail(DeriveNoFgType, "AX cost row has no FG type")
	}

	basis, loss, err := rs.Loss(erprule.FgType(out.fgType), out.prodType, group)
	if err != nil {
		return out.fail(DeriveNoRule, ruleFailureMessage(out.fgType, out.prodType, group, err))
	}
	out.basis, out.valLoss = basis, loss
	if !basis.IsSellPrice() {
		return out
	}
	price, err := rs.SellPrice(basis)
	if err != nil {
		return out.fail(DeriveNoSellPrice, fmt.Sprintf("no sell price for basis %s", basis))
	}
	out.sellPrice = price
	return out
}

func (o basisOutcome) fail(status DeriveStatus, msg string) basisOutcome {
	o.status = status
	o.failure = msg
	return o
}

func ruleFailureMessage(fgType string, prodType erprule.ProdType, group erprule.GradeGroup, err error) string {
	key := fgType + "/" + string(prodType) + "/" + string(group)
	switch {
	case errors.Is(err, erprule.ErrRuleAmbiguous):
		return "more than one valloss rule for " + key
	case errors.Is(err, erprule.ErrAxRule):
		return "grade group AX has no valloss rule (" + key + ")"
	case errors.Is(err, erprule.ErrInvalidFgType):
		return "FG type " + fmt.Sprintf("%q", fgType) + " cannot match a valloss rule"
	default:
		return "no valloss rule for " + key
	}
}
