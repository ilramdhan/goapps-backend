package mbhead

import "context"

// CostCalcDetailFilter narrows the flat MB cost-calculation detail dump.
//
// ⛔ Every field is "absence = no filter". Only CalculationType is defaulted, and that
// defaulting happens in the application layer (see ExportCostCalcDetailHandler), never
// here and never in the repository.
type CostCalcDetailFilter struct {
	// IsActive nil means no active filter at all — absence stays absence (D13).
	IsActive *bool
	// Period is YYYYMM, or empty for "latest calculated period per head".
	Period string
	// CalculationType is already defaulted by the application layer.
	CalculationType string
	// CalcStatus filters cst_product_cost.cpc_status. Empty means no status filter.
	CalcStatus string
	// IncludeRejected false (the zero value) EXCLUDES REJECTED heads, mirroring
	// RecipeFullFilter.IncludeRejected.
	IncludeRejected bool
}

// CostCalcDetailRow is ONE (MB head, RM line) row of the calc dump. The MB-level
// columns repeat identically across every RM line belonging to the same head.
//
// ⭐ SINGLE SOURCE: every MB-level number on this row comes from ONE cst_product_cost
// snapshot row — cpc_param_snapshot for both the input params and the calc results,
// cpc_rm_cost_detail for the RM lines. ⛔ No MB-level aggregate is ever RE-DERIVED from
// the RM lines: the stored aggregate is authoritative and legitimately diverges from
// SUM(contribution) when an RM line's rate was missing at calc time (verified: the sum
// matches for only 2315 of 4188 MBs in the reference dataset).
//
// Numeric columns are pointers so "the calc never produced this param" stays
// distinguishable from "the calc produced 0" (D13). ⛔ A missing MB_FIXED_TOTAL must
// render as an empty cell, never as 0.
type CostCalcDetailRow struct {
	// MB identity block, from mst_mb_head.
	MBCode string
	MBName *string

	// MB input-param block, read from cpc_param_snapshot by mst_parameter code. These
	// are the FROZEN inputs the calc actually ran on, not the live editable head
	// columns — reading the head would retroactively rewrite historical dumps.
	//
	// TotalFix is MACHINE_MB_FIXED_TOTAL (frozen from mst_mb_head.mbh_machine_fixed_total
	// at Validate time). It is CONSTANT across the reference dataset but is read
	// per-MB — never hardcoded.
	//
	// ThroughputPerHr is MB_THROUGHPUT, the resolved NUMERIC throughput. ⚠ It is NOT
	// restricted to the THROUGHPUT_PER_HOUR picklist options (30/40/50/60/70): the
	// reference dataset carries 40/34/20/55, so per-MB overrides are normal.
	// NoProcess is MB_NO_PROCESS, the NUMBER (1/2/3) — ⛔ never the S/D/T letter code.
	TotalFix        *float64
	PctWaste        *float64
	PctQualityLoss  *float64
	PctEfficiency   *float64
	DevExpense      *float64
	Packing         *float64
	ProdPerDay      *float64
	ThroughputPerHr *float64
	NoProcess       *float64

	// MB calc-result block, read from cpc_param_snapshot keyed by the MB_* result param
	// codes (see mbbatch.ResultParam*). Invariants that hold across the whole reference
	// dataset and are pinned by TestCostCalcDetail_Identities:
	//   MBNetProd    == ThroughputPerHr * ProdPerDay * PctEfficiency/100
	//   MBFixedTotal == TotalFix / MBNetProd
	//   MBTotalCost  == MBRMCost + MBConvCost
	//   MBCostPerUnit is byte-identical to MBTotalCost (both are MB_FINAL_COST).
	MBNetProd     *float64
	MBWasteVal    *float64
	MBFixedTotal  *float64
	MBCostOthers  *float64
	MBRMCost      *float64
	MBConvCost    *float64
	MBTotalCost   *float64
	MBCostPerUnit *float64

	// Cost-snapshot traceability block.
	CalcVersion int32
	CalcStatus  string
	// RowNo is the DENSE PER-PERIOD ORDINAL taken from the numeric suffix of
	// cost_product_master.cpm_product_code (CSTMB<YYMM><NNNNNN>). It runs 1..N within a
	// period. It is a PRESENTATION row number, nothing more — ⛔ it is NOT an id, not a
	// foreign key, and nothing may be looked up by it.
	//
	// It was named "seq" until the column was renamed to row_no precisely because "seq"
	// kept inviting the question "which id is this?". The answer is: none.
	//
	// ============== WHY THIS WILL NOT MATCH THE REFERENCE FILE (BY CHOICE) ==============
	// ⚠ STATE THIS PLAINLY: this column does NOT reproduce the reference export's seq
	// values, and is not meant to. For CSTMB2607000001 this emits 1 where the reference
	// file shows 52939. That divergence is a DECISION, not a defect.
	//
	// ⭐ RESOLVED — the reference file's seq IS reproducible from this database: it is
	// cost_route_seq.crs_seq_id. Proof: the production sequence
	// cost_route_seq_crs_seq_id_seq has last_value = 57931, exactly the reference file's
	// max(seq) (recon/production.txt:135). Mechanism: one transaction per MB Validate
	// advances cpm_product_sys_id, crh_head_id and crs_seq_id in lockstep
	// (infrastructure/postgres/mb_autogen_repository.go:268-284) and mbInsertRouteSeq
	// (same file, :449-459) inserts exactly one cost_route_seq row per MB product — so
	// crs_seq_id is 1:1 with the MB product, which is why it looks like a per-MB id. The
	// 801-wide "gap" at the 2607/2608 period boundary is real interleaved NON-MB
	// cost_route_seq traffic (cost_import_resolve.go:418; cost_route_repository.go:66,
	// 377, 1251, 1457). The reference file is not a legacy-Oracle artifact either: it was
	// authored 2026-08-20, after this Postgres system went live — an ad-hoc query against
	// THIS database.
	//
	// ⛔ AND IT IS DELIBERATELY NOT USED. Three independent reasons:
	//  1. crs_seq_id is UNSTABLE. cost_route_repository.go:364 DELETEs and reinserts
	//     cost_route_seq rows, so ids are burned and reassigned: production holds 34,219
	//     live rows against a 57,931 counter (~23.7k ids already burned). Exporting it
	//     would publish a number that silently changes under recalculation.
	//  2. Nothing consumes it. This column is WRITE-ONLY end to end — repository ->
	//     handler -> browser download. No import path reads it back (there is no
	//     ImportMBCostCalcDetail among the finance service's 23 Import* RPCs).
	//  3. The real business key is mb_code, which this export already carries.
	//
	// ⛔ Also NOT cst_product_cost.cpc_cost_id (production: 421776/422973/422970/388073
	// for CSTMB2607000001/2/3/5 — wrong range, non-monotonic, moves with every recalc).
	// ⛔ Also NOT cost_product_master.cpm_product_sys_id (production: CSTMB2607000001 ->
	// 36519; the MB population spans 36519..41024 with ~313 gaps).
	// ⛔ NOT mbcm_seq_no, and not a running row counter across the file.
	RowNo int64

	// RM line block, one element of the cpc_rm_cost_detail JSONB array.
	RMType string
	RMRef  string
	// RMGroupName resolves RMRef against cst_rm_group_head.group_name. nil when the ref
	// cannot be resolved — PRODUCT-type refs never resolve, they are nested MBs.
	RMGroupName *string
	Komposisi   *float64
	// RMRateActual is cpc_rm_cost_detail[].unit_cost — the IMMUTABLE rate recorded at
	// calc time. ⛔ Never a live join to cst_rm_cost.cost_val: that would retroactively
	// rewrite historical dumps when a period is recalculated.
	RMRateActual *float64
	// RMRateTier is the Selection Flags V2 tier (CL/SL/FL/PR/CR/SR) the RM cost engine
	// applied. It is READ from cst_rm_cost.flag_valuation_used whenever that column
	// holds a V2 label, and only RE-DERIVED from valuation_flag_v2 + the per-tier rate
	// columns (or the AUTO cascade) when it does not — see resolveRMRateTier in
	// infrastructure/postgres/mb_cost_calc_detail_export_repository.go for the exact
	// precedence.
	//
	// ⭐ PRODUCTION-VERIFIED (periods 202607/202608/202609, 1050 cst_rm_cost rows):
	// valuation_flag_v2 is NULL on EVERY row while flag_valuation_used already carries
	// the V2 vocabulary only (SL/PR/FL/CL/NONE) with ZERO legacy V1 labels, and cost_val
	// matches the rate column it names. Re-deriving from the all-NULL valuation_flag_v2
	// — as this field's earlier contract demanded — mislabeled every row.
	//
	// ⚠ The V1-label guard is still enforced: a flag_valuation_used holding a legacy
	// CONS/STORES/DEPT/PO_* label (possible on pre-ENG-RM-01/P3 rows) is rejected and
	// falls through to the re-derive. That guard is what makes reading the column safe.
	//
	// Empty for PRODUCT-type rows, which have no cst_rm_cost row at all — in the
	// reference dataset the blank count (353) equals the PRODUCT row count exactly.
	// Also empty when the resolved tier is NONE (no price source existed).
	RMRateTier string
	// CostActual is cpc_rm_cost_detail[].contribution (== Komposisi * RMRateActual).
	CostActual *float64
}

// CostCalcDetailReader is the single read-only port the calc-detail dump depends on.
//
// ⛔ Read-only by contract: cst_product_cost and cst_rm_cost are read for display only.
type CostCalcDetailReader interface {
	ListCostCalcDetailRows(ctx context.Context, filter CostCalcDetailFilter) ([]CostCalcDetailRow, error)
}
