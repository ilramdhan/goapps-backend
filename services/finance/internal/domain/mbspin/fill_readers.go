package mbspin

// NumericFillReaders maps lookup_source_column → numeric value extractor for mst_mb_spin entity.
var NumericFillReaders = map[string]func(*Entity) (float64, bool){
	"mbs_denier": func(e *Entity) (float64, bool) {
		if v := e.Denier(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbs_dozing is the retired, contaminated legacy column. The READER is kept
	// on purpose until L1 repoints lookup_source_column — removing it now would empty
	// out the fills that are currently live.
	// G5 (2026-08-22): it is deliberately NOT registered in mst_lookup_master_column
	// (pulled back out of 000477), so it is not offered in the "Source Column"
	// dropdown — its units are mixed across heads (oil-rate vs run_ldr scale). It is a
	// documented t7Exceptions entry in yarn_lookup_fill_column_registry_test.go.
	"mbs_dozing": func(e *Entity) (float64, bool) {
		if v := e.Dozing(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbs_run_ldr_pct is the actual LDR used in production — the correct value for costing.
	"mbs_run_ldr_pct": func(e *Entity) (float64, bool) {
		if v := e.MBSRunLdrPct(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbs_ldr_prsn is the planned LDR, set while the product is still new.
	"mbs_ldr_prsn": func(e *Entity) (float64, bool) {
		if v := e.MBSLdrPrsn(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"mbs_filament": func(e *Entity) (float64, bool) {
		if v := e.Filament(); v != nil {
			return float64(*v), true
		}
		return 0, false
	},
	"mbs_cost_rate_mkt": func(e *Entity) (float64, bool) {
		if v := e.CostRateMkt(); v != nil {
			return *v, true
		}
		return 0, false
	},
}

// TextFillReaders maps lookup_source_column → text value extractor for mst_mb_spin entity.
var TextFillReaders = map[string]func(*Entity) (string, bool){
	"mbs_mgt_name": func(e *Entity) (string, bool) {
		if v := e.MgtName(); v != "" {
			return v, true
		}
		return "", false
	},
	"mbs_cc": func(e *Entity) (string, bool) {
		if v := e.CC(); v != nil && *v != "" {
			return *v, true
		}
		return "", false
	},
}
