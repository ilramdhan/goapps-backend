package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

// loadMachineParamDefaultsQuery resolves, per product, the live
// mst_machine_param_default rows (000614) of the machine named by the
// product's MC_NAME text value. The machine matches on trimmed,
// case-insensitive mc_name OR mc_code (same rule as the 000614 backfill).
const loadMachineParamDefaultsQuery = `
	SELECT DISTINCT ON (cpp.cpp_product_sys_id, p.param_code)
	       cpp.cpp_product_sys_id, p.param_code, d.mmpd_default_value::float8
	FROM cost_product_parameter cpp
	JOIN mst_parameter mc
	     ON mc.id = cpp.cpp_param_id
	    AND mc.param_code = 'MC_NAME'
	    AND mc.deleted_at IS NULL
	JOIN mst_machine m
	     ON m.deleted_at IS NULL
	    AND (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))
	         OR LOWER(BTRIM(m.mc_code)) = LOWER(BTRIM(cpp.cpp_value_text)))
	JOIN mst_machine_param_default d
	     ON d.mmpd_machine_id = m.mc_id
	    AND d.deleted_at IS NULL
	JOIN mst_parameter p
	     ON p.id = d.mmpd_param_id
	    AND p.deleted_at IS NULL
	WHERE cpp.cpp_product_sys_id = ANY($1)
	  AND NULLIF(BTRIM(cpp.cpp_value_text), '') IS NOT NULL
	ORDER BY cpp.cpp_product_sys_id, p.param_code,
	         (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))) DESC, m.mc_code`

// MachineParamDefaultLoader is the narrow read used by the product parameter form
// to flag params whose formula is skipped for the product's machine.
type MachineParamDefaultLoader interface {
	LoadMachineParamDefaults(ctx context.Context, productSysIDs []int64) (map[int64]map[string]float64, error)
}

// NewMachineParamDefaultLoader returns the SQL-backed MachineParamDefaultLoader.
func NewMachineParamDefaultLoader(db *sql.DB) MachineParamDefaultLoader {
	return &productLoader{db: db}
}

// LoadMachineParamDefaults implements ProductLoader.LoadMachineParamDefaults:
// productSysID -> paramCode -> machine default. Products whose machine has no
// configured default are absent.
func (l *productLoader) LoadMachineParamDefaults(ctx context.Context, productSysIDs []int64) (map[int64]map[string]float64, error) {
	defer observeLoad(loaderKindMachineDefault, time.Now())
	out := map[int64]map[string]float64{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := l.db.QueryContext(ctx, loadMachineParamDefaultsQuery, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load machine param defaults: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			pid  int64
			code string
			val  float64
		)
		if err := rows.Scan(&pid, &code, &val); err != nil {
			return nil, fmt.Errorf("scan machine param default row: %w", err)
		}
		inner, ok := out[pid]
		if !ok {
			inner = map[string]float64{}
			out[pid] = inner
		}
		// One row per (product, param): DISTINCT ON prefers a mc_name match over a
		// mc_code match, the same tie-break as the 000614 backfill.
		inner[code] = val
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machine param default rows: %w", err)
	}
	return out, nil
}

// applyMachineDefaults fixes each param in in.MachineDefaults to the product's
// stored value (CAPP, editable per product) or, when the product has none, the
// machine default; it clears the param from zeroFilled and records it in
// inherited (code -> trace expression) so evalFormulaChain skips its formula.
// Nil/empty MachineDefaults returns inherited unchanged.
func applyMachineDefaults(in ComputeInput, scope map[string]any, zeroFilled map[string]bool, inherited map[string]string) map[string]string {
	if len(in.MachineDefaults) == 0 {
		return inherited
	}
	// Only params this product actually produces (its formula list is filtered
	// by applicability), so a non-applicable param never enters the snapshot.
	produced := make(map[string]bool, len(in.Formulas))
	for _, f := range in.Formulas {
		produced[f.ResultParamCode] = true
	}
	codes := make([]string, 0, len(in.MachineDefaults))
	for code := range in.MachineDefaults {
		if produced[code] {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return inherited
	}
	sort.Strings(codes)
	if inherited == nil {
		inherited = make(map[string]string, len(codes))
	}
	for _, code := range codes {
		def := in.MachineDefaults[code]
		value, source := def, fmt.Sprintf("machine initial %g", def)
		if v, ok := in.CAPP[code]; ok {
			value, source = v, "product value"
		}
		scope[code] = value
		delete(zeroFilled, code)
		inherited[code] = "machine default (" + source + ")"
	}
	return inherited
}
