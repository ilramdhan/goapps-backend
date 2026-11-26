package mbspin

// NumericFillReaders maps lookup_source_column → numeric value extractor for mst_mb_spin entity.
var NumericFillReaders = map[string]func(*Entity) (float64, bool){
	"mbs_denier": func(e *Entity) (float64, bool) {
		if v := e.Denier(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbs_dozing is the retired, contaminated legacy column. L1 (migration 000611)
	// repoints MB_SP_DOZING to mbs_run_ldr_pct. The READER is kept on purpose so any
	// parameter still configured with mbs_dozing (and the 000611 down path) keeps
	// filling instead of going empty.
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
	// mb-comparison-p2 item 2: honor the manual LDR adjustment. When adjustment is
	// non-zero the value is (calculated, else run_ldr) + adjustment (same as the
	// effective LDR shown in the form); otherwise raw run_ldr, unchanged. Requires the
	// entity loaded with HydrateShadeAndLDR (all repository paths do).
	"mbs_run_ldr_pct": func(e *Entity) (float64, bool) {
		if adj := e.LDRAdjustmentPct(); adj != nil && *adj != 0 {
			base := e.LDRCalculatedPct()
			if base == nil {
				base = e.MBSRunLdrPct()
			}
			if base == nil {
				return 0, false
			}
			return *base + *adj, true
		}
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
