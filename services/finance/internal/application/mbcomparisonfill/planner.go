package mbcomparisonfill

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// PlanRow is the verdict for one Excel row.
type PlanRow struct {
	ExcelRow  int
	ItemCode  string
	Shade     string
	ProductID int64
	Decision  Decision
	Reason    string
	Before    map[string]string // current value of each changed param ("" = no CPP row)
	After     map[string]string // value to write per changed param
	CAPP      []string          // params whose CAPP row is missing and must be inserted
}

// Plan is the planner output.
type Plan struct {
	Rows           []PlanRow
	CPPWithoutCAPP int // products having any written-param CPP without its CAPP
}

// Counts returns rows per decision.
func (p *Plan) Counts() map[Decision]int {
	m := map[Decision]int{}
	for _, r := range p.Rows {
		m[r.Decision]++
	}
	return m
}

// BuildPlan classifies every Excel row against the active products (key -> products).
// orion maps the legacy SYSID to CMBH_ORION_ITEM_CODE (fallback for an empty MB_SP_CODE cell).
func BuildPlan(rows []ExcelRow, products map[string][]Product, orion map[string]string) *Plan {
	dup := map[string]int{}
	for _, r := range rows {
		dup[ProductKey(r.ItemCode, r.Shade)]++
	}
	plan := &Plan{}
	seen := map[int64]bool{}
	for _, r := range rows {
		key := ProductKey(r.ItemCode, r.Shade)
		pr := PlanRow{ExcelRow: r.Row, ItemCode: r.ItemCode, Shade: r.Shade}
		ps := products[key]
		switch {
		case r.HasNA:
			pr.Decision, pr.Reason = DecisionReport, "excel_na"
		case r.Malformed != "":
			pr.Decision, pr.Reason = DecisionReport, "excel_malformed: "+r.Malformed
		case len(ps) == 0:
			pr.Decision, pr.Reason = DecisionReport, "product_not_found"
		case len(ps) > 1:
			pr.Decision, pr.Reason = DecisionReview, fmt.Sprintf("multiple_products(%d)", len(ps))
		case dup[key] > 1:
			pr.Decision, pr.Reason = DecisionReview, "duplicate_excel_key"
		default:
			classify(&pr, r, ps[0], orion)
			if !seen[ps[0].SysID] {
				seen[ps[0].SysID] = true
				if cppWithoutCAPP(ps[0]) {
					plan.CPPWithoutCAPP++
				}
			}
		}
		plan.Rows = append(plan.Rows, pr)
	}
	return plan
}

func cppWithoutCAPP(p Product) bool {
	for code := range p.Values {
		if !p.HasCAPP[code] {
			return true
		}
	}
	return false
}

// desired returns the values to write from the Excel row (empty cells are never written).
func desired(r ExcelRow, orion map[string]string) map[string]string {
	want := map[string]string{}
	for _, p := range Params {
		c := strings.TrimSpace(r.Cells[p.Code])
		if p.Code == "MB_SP_CODE" && c == "" {
			c = strings.TrimSpace(orion[strings.TrimSpace(r.SysID)])
		}
		if c == "" {
			continue
		}
		if p.Kind == mblegacyfill.KindNum {
			c, _ = NormNum(c) // validated at parse time
		}
		want[p.Code] = c
	}
	return want
}

func classify(pr *PlanRow, r ExcelRow, p Product, orion map[string]string) {
	pr.ProductID = p.SysID
	if p.Locked {
		pr.Decision, pr.Reason = DecisionReport, "product_locked"
		return
	}
	want := desired(r, orion)
	pr.Before, pr.After = map[string]string{}, map[string]string{}
	codes := make([]string, 0, len(want))
	for _, pa := range Params {
		if _, ok := want[pa.Code]; ok {
			codes = append(codes, pa.Code)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		cur, has := p.Values[code]
		k, _ := ParamKind(code)
		if !has || !mblegacyfill.Equal(k, cur, want[code]) {
			pr.Before[code], pr.After[code] = cur, want[code]
		}
		if !p.HasCAPP[code] {
			pr.CAPP = append(pr.CAPP, code)
		}
	}
	switch {
	case len(want) == 0:
		pr.Decision, pr.Reason = DecisionSkipSame, "nothing_to_write"
	case len(pr.After) > 0:
		pr.Decision, pr.Reason = DecisionApply, fmt.Sprintf("changed:%s", strings.Join(sortedKeys(pr.After), ","))
	case len(pr.CAPP) > 0:
		pr.Decision, pr.Reason = DecisionApply, "capp_only"
	default:
		pr.Decision, pr.Reason = DecisionSkipSame, "same"
	}
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
