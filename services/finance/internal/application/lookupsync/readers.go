package lookupsync

import (
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// machineNumericReaders maps lookup_source_column → value extractor for mst_machine entity.
// Add new entries here when new fillable columns are added to mst_lookup_master_column.
var machineNumericReaders = map[string]func(*machine.Entity) (float64, bool){
	"mc_speed":       func(e *machine.Entity) (float64, bool) { return e.MCSpeed(), true },
	"mc_efficiency":  func(e *machine.Entity) (float64, bool) { return e.MCEfficiency(), true },
	"no_of_position": func(e *machine.Entity) (float64, bool) { return float64(e.NoOfPosition()), true },
	"no_of_end":      func(e *machine.Entity) (float64, bool) { return float64(e.NoOfEnd()), true },
	"machine_rpm": func(e *machine.Entity) (float64, bool) {
		if v := e.MachineRPM(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"power_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.PowerPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"mp_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.MpPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"ohs_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.OhsPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"spares_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.SparesPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"kgs_lost_change": func(e *machine.Entity) (float64, bool) {
		if v := e.KgsLostChange(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb1_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb1Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb2_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb2Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb3_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb3Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb4_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb4Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb5_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb5Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// mc_weightage was registered in mst_lookup_master_column by 000425 but had no
	// reader here, so the fill-group UI could offer the column while the handler
	// could not resolve it. Wired up alongside the MC_WEIGHTAGE param (000475).
	"mc_weightage": func(e *machine.Entity) (float64, bool) {
		if v := e.McWeightage(); v != nil {
			return *v, true
		}
		return 0, false
	},
}

// interminglingNumericReaders maps lookup_source_column → value extractor for mst_intermingling entity.
var interminglingNumericReaders = map[string]func(*intermingling.Entity) (float64, bool){
	"intm_cost_per_kg": func(e *intermingling.Entity) (float64, bool) { return e.CostPerKg(), true },
}

// productGradeNumericReaders maps lookup_source_column → value extractor for mst_product_grade entity.
var productGradeNumericReaders = map[string]func(*productgrade.Entity) (float64, bool){
	"bc_perc":           func(e *productgrade.Entity) (float64, bool) { return e.BCPerc(), true },
	"non_std_perc":      func(e *productgrade.Entity) (float64, bool) { return e.NonStdPerc(), true },
	"bc_recovery_rate":  func(e *productgrade.Entity) (float64, bool) { return e.BCRecoveryRate(), true },
	"std_selling_price": func(e *productgrade.Entity) (float64, bool) { return e.StdSellingPrice(), true },
	"sp_value":          func(e *productgrade.Entity) (float64, bool) { return e.SpValue(), true },
}

// productGradeTextReaders maps lookup_source_column → text value extractor for mst_product_grade entity.
var productGradeTextReaders = map[string]func(*productgrade.Entity) (string, bool){
	"pg_detail_product": func(e *productgrade.Entity) (string, bool) {
		if v := e.PgDetailProduct(); v != "" {
			return v, true
		}
		return "", false
	},
	"pg_grade_label": func(e *productgrade.Entity) (string, bool) {
		if v := e.PgGradeLabel(); v != "" {
			return v, true
		}
		return "", false
	},
}

// MBHeadNumericReaders maps lookup_source_column → numeric value extractor for mst_mb_head entity.
// It is exported (unlike the other reader maps) only so the delivery-layer reader tests keep
// asserting against the live map; treat it as read-only.
var MBHeadNumericReaders = map[string]func(*mbhead.Entity) (float64, bool){
	"mbh_dozing": func(e *mbhead.Entity) (float64, bool) {
		if v := e.Dozing(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbh_run_ldr_pct is the actual LDR used in production — the correct value for costing.
	"mbh_run_ldr_pct": func(e *mbhead.Entity) (float64, bool) {
		if v := e.MBHRunLdrPct(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbh_ldr_prsn is the planned LDR, set while the product is still new.
	"mbh_ldr_prsn": func(e *mbhead.Entity) (float64, bool) {
		if v := e.MBHLdrPrsn(); v != nil {
			return *v, true
		}
		return 0, false
	},
}

// mbHeadTextReaders maps lookup_source_column → text value extractor for mst_mb_head entity.
var mbHeadTextReaders = map[string]func(*mbhead.Entity) (string, bool){
	"mbh_mgt_name": func(e *mbhead.Entity) (string, bool) {
		if v := e.MgtName(); v != nil && *v != "" {
			return *v, true
		}
		return "", false
	},
}

// mbSpinNumericReaders / mbSpinTextReaders map lookup_source_column → value extractor for
// mst_mb_spin. The maps live in the mbspin domain package (mbspin.NumericFillReaders /
// mbspin.TextFillReaders) so the shade-driven auto-fill (mbsource) reuses EXACTLY the same
// mapping as the interactive Param-tab fill and cannot diverge from it.
var (
	mbSpinNumericReaders = mbspin.NumericFillReaders
	mbSpinTextReaders    = mbspin.TextFillReaders
)

// boxBobbinCostColumns mirrors the `case` labels of fillFromBoxBobbinCost, which
// resolves columns with a literal switch instead of a reader map and therefore
// cannot be enumerated reflectively. TestBoxBobbinSwitchColumnsMatchSource
// asserts this slice against resolver_box_bobbin.go so it cannot drift from the
// switch it copies.
// switch it copies.
var boxBobbinCostColumns = []string{
	"no_of_bob",
	"bbcr_bob_rate_mkt",
	"bbcr_box_rate_mkt",
	"bbn_reuse",
	"box_reuse",
	"box_cost",
	"bobin_cost",
	"box_cost_val",
	"bobin_cost_val",
	"bbn_reuse_val",
	"box_reuse_val",
	"bbcr_bob_rate_val",
	"bbcr_box_rate_val",
}

// LookupReaderColumns returns every lookup_source_column the Resolver can
// actually resolve, keyed by lookup master code.
//
// This is derived from the reader maps above — the single source of truth. It
// exists so the startup registry-divergence check can compare the live contents
// of mst_lookup_master_column against the readers without maintaining a second
// hand-written column list, which is the very duplication that produced the R30
// drift in the first place.
func LookupReaderColumns() map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, 6)
	add := func(master, col string) {
		if out[master] == nil {
			out[master] = map[string]struct{}{}
		}
		out[master][col] = struct{}{}
	}
	for col := range machineNumericReaders {
		add(MasterMachine, col)
	}
	for col := range interminglingNumericReaders {
		add(MasterIntermingling, col)
	}
	for col := range productGradeNumericReaders {
		add(MasterProductGrade, col)
	}
	for col := range productGradeTextReaders {
		add(MasterProductGrade, col)
	}
	for col := range MBHeadNumericReaders {
		add(MasterMBHead, col)
	}
	for col := range mbHeadTextReaders {
		add(MasterMBHead, col)
	}
	for col := range mbSpinNumericReaders {
		add(MasterMBSpin, col)
	}
	for col := range mbSpinTextReaders {
		add(MasterMBSpin, col)
	}
	for _, col := range boxBobbinCostColumns {
		add(MasterBoxBobbinCost, col)
	}
	return out
}
