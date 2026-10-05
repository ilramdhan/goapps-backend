package validation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// V07MaxRate is the upper bound of ODBTRG_COST_VAL_MGT error 241441 (and
// ORA-20902 in the lead's package): 0 < rate <= 20 USD.
var V07MaxRate = decimal.NewFromInt(20)

// V07Prefixes are the item prefixes the ERP trigger guards (design §7 V-07).
// Masterbatch (CMB) is exempt. Keep in sync with
// application/erpintegration.GuardedItemPrefixes (same list, T-CUR).
var V07Prefixes = []string{"POY", "PTY", "ACY", "ITY", "MMK", "TTY", "HOY"}

// IsV07Guarded reports whether an ERP item code carries the V-07 bound.
func IsV07Guarded(itemCode string) bool {
	c := strings.ToUpper(strings.TrimSpace(itemCode))
	if erpintegration.ItemKindForCode(c) == erpintegration.ItemKindMB {
		return false
	}
	for _, p := range V07Prefixes {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

// V07RateOK is the 241441 rule for one rate of a guarded item: set, > 0 and
// <= 20. Non-guarded items always pass.
func V07RateOK(itemCode string, rate decimal.NullDecimal) bool {
	if !IsV07Guarded(itemCode) {
		return true
	}
	return rate.Valid && rate.Decimal.IsPositive() && rate.Decimal.LessThanOrEqual(V07MaxRate)
}

// V07 checks every valued guarded yarn row has 0 < std <= 20 (design §7
// V-07, row scope; error). The period-set half is PrevalidatePeriodSet.
func V07() Validator {
	return validatorFunc{code: CodeV07, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			if r.Status != erpintegration.DeriveOK || r.Kind == erpintegration.ItemKindMB {
				continue
			}
			if !V07RateOK(r.Key.ItemCode, r.StdCost) {
				out = append(out, rowFinding(CodeV07, SeverityError, r.Key,
					"std "+stdText(r)+" violates ERP rule 241441 (0 < std <= 20 for "+strings.Join(V07Prefixes, "/")+")"))
			}
		}
		return out
	}}
}

// AdjSetItem is one OT_ADJ_ITEM row of the eligible period set as read
// (SELECT only). ProjectedRate is the rate the period-wide VALUATE_ADJ would
// write (the batch std for the item's key); when not set, the current rate
// stays and is what the trigger sees.
type AdjSetItem struct {
	ItemSysID     int64
	ItemCode      string
	GradeCode     string
	ShadeCode     string
	CurrentRate   decimal.NullDecimal
	ProjectedRate decimal.NullDecimal
}

// Key returns the std row key of the item.
func (it AdjSetItem) Key() erpintegration.ErpKey {
	return erpintegration.ErpKey{
		ItemCode:  strings.TrimSpace(it.ItemCode),
		GradeCode: strings.TrimSpace(it.GradeCode),
		ShadeCode: strings.TrimSpace(it.ShadeCode),
	}
}

// EffectiveRate is ProjectedRate when set, else CurrentRate.
func (it AdjSetItem) EffectiveRate() decimal.NullDecimal {
	if it.ProjectedRate.Valid {
		return it.ProjectedRate
	}
	return it.CurrentRate
}

// AdjSetHead is one eligible OT_ADJ_HEAD of the period with its items.
type AdjSetHead struct {
	HeadSysID int64
	TxnCode   string
	Items     []AdjSetItem
}

// ProjectRates fills ProjectedRate of every item from the std rows of the
// batch (valued OK rows only), matched by (item, grade, shade). It returns a
// copy; the input is not modified.
func ProjectRates(heads []AdjSetHead, rows []erpintegration.StdRow) []AdjSetHead {
	std := make(map[erpintegration.ErpKey]decimal.Decimal, len(rows))
	for _, r := range rows {
		if r.Status == erpintegration.DeriveOK && r.StdCost.Valid {
			std[r.Key] = r.StdCost.Decimal
		}
	}
	out := make([]AdjSetHead, len(heads))
	for i, h := range heads {
		items := make([]AdjSetItem, len(h.Items))
		for j, it := range h.Items {
			if v, ok := std[it.Key()]; ok {
				it.ProjectedRate = decimal.NullDecimal{Decimal: v, Valid: true}
			}
			items[j] = it
		}
		h.Items = items
		out[i] = h
	}
	return out
}

// OffendingHead is a head the ERP trigger would reject, with its bad items.
type OffendingHead struct {
	HeadSysID int64
	TxnCode   string
	Items     []AdjSetItem
}

// PeriodSetResult is the G7a pre-validation outcome over the whole set.
type PeriodSetResult struct {
	Period    string
	HeadCount int
	ItemCount int
	Offending []OffendingHead
}

// OK reports whether no head would fail (VALUATE_ADJ / APPROVE_ADJ allowed).
func (r PeriodSetResult) OK() bool { return len(r.Offending) == 0 }

// OffendingHeadIDs lists the offending head ids in ascending order.
func (r PeriodSetResult) OffendingHeadIDs() []int64 {
	ids := make([]int64, len(r.Offending))
	for i, h := range r.Offending {
		ids[i] = h.HeadSysID
	}
	return ids
}

// Findings renders the offending items as period-set V-07 errors, ordered
// by head id then item id (the order of Offending).
func (r PeriodSetResult) Findings() []Finding {
	var out []Finding
	for _, h := range r.Offending {
		for _, it := range h.Items {
			rate := "NULL"
			if er := it.EffectiveRate(); er.Valid {
				rate = er.Decimal.String()
			}
			out = append(out, Finding{
				Code: CodeV07, Severity: SeverityError, Scope: ScopePeriodSet, Key: it.Key(), HeadSysID: h.HeadSysID,
				Message: fmt.Sprintf("head %d item %d rate %s violates ERP rule 241441 (0 < rate <= 20)", h.HeadSysID, it.ItemSysID, rate),
			})
		}
	}
	return out
}

// PrevalidatePeriodSet evaluates the 241441 rule over the whole eligible
// period set (G7a, User decision 2026-09-29 U-1): every item of a guarded
// prefix must have an effective rate in (0, 20]. It returns every offending
// head (ascending head id, items ascending by item id). The package is
// period-wide, so one offending head refuses the whole call.
func PrevalidatePeriodSet(period string, heads []AdjSetHead) PeriodSetResult {
	res := PeriodSetResult{Period: period, HeadCount: len(heads)}
	for _, h := range heads {
		res.ItemCount += len(h.Items)
		var bad []AdjSetItem
		for _, it := range h.Items {
			if !V07RateOK(it.ItemCode, it.EffectiveRate()) {
				bad = append(bad, it)
			}
		}
		if len(bad) == 0 {
			continue
		}
		sort.Slice(bad, func(i, j int) bool { return bad[i].ItemSysID < bad[j].ItemSysID })
		res.Offending = append(res.Offending, OffendingHead{HeadSysID: h.HeadSysID, TxnCode: h.TxnCode, Items: bad})
	}
	sort.Slice(res.Offending, func(i, j int) bool { return res.Offending[i].HeadSysID < res.Offending[j].HeadSysID })
	return res
}
