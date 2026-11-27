// Package mbcomparisonfill fills the per-product MB/SP parameters (and the ACTUAL conv/RP dozing
// inputs added by migration 000616) from the Finance mb-comparison Excel dump.
// Spec: docs/superpowers/specs/2026-11-24-mb-comparison-p1-design.md section 3.
package mbcomparisonfill

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// Decision is the plan verdict of one Excel row.
type Decision string

// Decisions.
const (
	DecisionApply    Decision = "APPLY"
	DecisionSkipSame Decision = "SKIP_SAME"
	DecisionReview   Decision = "REVIEW"
	DecisionReport   Decision = "REPORT"
)

// Param is a product parameter the tool writes.
type Param struct {
	Code string
	Kind mblegacyfill.Kind // KindNum -> cpp_value_numeric, KindText -> cpp_value_text
}

// Params lists every written parameter, in write order.
var Params = []Param{
	{"MB_SP_CODE", mblegacyfill.KindText}, {"MB_SP_DYE", mblegacyfill.KindText},
	{"MB_SP_DENIER", mblegacyfill.KindNum}, {"MB_SP_FILAMENT", mblegacyfill.KindNum},
	{"MB_SP_CC", mblegacyfill.KindText}, {"MB_SP_DOZING", mblegacyfill.KindNum},
	{"CONV_FACTOR_ACTUAL", mblegacyfill.KindNum}, {"RP_DOZING_ACTUAL", mblegacyfill.KindNum},
}

// ParamKind returns the kind of a written parameter.
func ParamKind(code string) (mblegacyfill.Kind, bool) {
	for _, p := range Params {
		if p.Code == code {
			return p.Kind, true
		}
	}
	return "", false
}

// PreflightFormulas must have been rewritten by migration 000616 before apply.
var PreflightFormulas = []string{"F_YARN_CONV_FACTOR", "F_YARN_RP_DOZING", "F_YARN_MB_COST"}

// MigrationMarker is the updated_by written by migration 000616.
const MigrationMarker = "migration_000616"

// MigrationMarker618 is the updated_by written by migration 000618 on RP_DOZING / MB_COST (also accepted by Preflight).
const MigrationMarker618 = "migration_000618"

// ExcelRow is one data row of dozing.csv (Excel row, columns 63-71).
type ExcelRow struct {
	Row       int
	ItemCode  string
	Shade     string
	SysID     string
	Cells     map[string]string // param code -> raw cell ("" = empty)
	HasNA     bool              // any used cell is #N/A
	Malformed string            // reason, e.g. non-numeric cell
}

// Product is the DB snapshot of one active cost product.
type Product struct {
	SysID    int64
	ItemCode string
	Shade    string
	Locked   bool
	Values   map[string]string // existing CPP value by param code (absent key = no CPP row)
	HasCAPP  map[string]bool
	SpinPtr  string // cpp_value_mb_spin_id of MB_SP_CODE ("" = NULL / no row)
}

// ShadeKey normalizes a shade: upper-case, without '-' and spaces.
func ShadeKey(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(s))
}

// ProductKey is the join key item + normalized shade.
func ProductKey(item, shade string) string {
	return strings.ToUpper(strings.TrimSpace(item)) + "|" + ShadeKey(shade)
}

// NormNum renders a numeric cell with the column scale (6 dp, trailing zeros trimmed).
func NormNum(raw string) (string, bool) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok {
		return "", false
	}
	f, _ := r.Float64()
	s := strconv.FormatFloat(f, 'f', 6, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		s = "0"
	}
	return s, true
}
