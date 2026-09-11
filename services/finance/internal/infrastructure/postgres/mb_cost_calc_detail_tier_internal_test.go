package postgres

import (
	"database/sql"
	"strings"
	"testing"
)

// ns builds a valid sql.NullString; nsNull builds a NULL one.
func ns(s string) sql.NullString   { return sql.NullString{String: s, Valid: true} }
func nsNull() sql.NullString       { return sql.NullString{} }
func nf(f float64) sql.NullFloat64 { return sql.NullFloat64{Float64: f, Valid: true} }

// TestCostCalcDetailQuery_SelectsFlagValuationUsed pins that the SELECT actually fetches
// cst_rm_cost.flag_valuation_used. Production proved valuation_flag_v2 is NULL on every
// row while flag_valuation_used carries the real V2 tier — if this column is dropped from
// the projection again, resolveRMRateTier silently falls back to the cascade and every
// exported rm_rate_tier is a guess. Structural string test: the package has no SQL mock or
// test database (see the precedent in mb_recipe_full_export_where_internal_test.go).
func TestCostCalcDetailQuery_SelectsFlagValuationUsed(t *testing.T) {
	if !strings.Contains(costCalcDetailQuery, "rc.flag_valuation_used") {
		t.Fatalf("costCalcDetailQuery must select rc.flag_valuation_used (production has it "+
			"populated and valuation_flag_v2 NULL), got query:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "rc.valuation_flag_v2") {
		t.Fatalf("costCalcDetailQuery must still select rc.valuation_flag_v2 for the "+
			"re-derive fallback, got query:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "(rc.rm_code IS NOT NULL) AS has_rm_cost") {
		t.Fatalf("costCalcDetailQuery must project has_rm_cost so a PRODUCT row (no "+
			"cst_rm_cost match) is distinguishable from an all-NULL match, got query:\n%s",
			costCalcDetailQuery)
	}
}

// TestCostCalcDetailQuery_NoCompositionFanOut pins the ROOT CAUSE of the 5x row
// inflation proven in production (4192 MBs but 108090 rows against a 21813-row target).
//
// This report's RM lines come from the FROZEN cpc_rm_cost_detail JSONB array of the
// selected cost snapshot. mst_mb_composition is the LIVE working-set recipe: joining it
// multiplies every cost row by n_composition_rows and, worse, reports today's recipe
// against a historical calculation. If a future edit reintroduces that join, the dump
// silently regains its ~5x multiplication with no error anywhere.
func TestCostCalcDetailQuery_NoCompositionFanOut(t *testing.T) {
	for _, banned := range []string{"mst_mb_composition", "mbcm_", "mst_mb_composition_version", "mbcv_"} {
		if strings.Contains(costCalcDetailQuery, banned) {
			t.Fatalf("costCalcDetailQuery must never touch the live composition tables "+
				"(found %q) — RM lines come from cpc_rm_cost_detail, and joining "+
				"composition multiplies every cost row by n_composition_rows:\n%s",
				banned, costCalcDetailQuery)
		}
	}
}

// TestCostCalcDetailQuery_SelectsExactlyOneCostRow pins the second fan-out source.
// uk_cpc_active is unique only on (product, period, calc_type) WHERE status !=
// 'SUPERSEDED', so SEVERAL PERIODS are simultaneously non-SUPERSEDED for one product.
// A plain JOIN on cst_product_cost therefore yields roughly one row PER PERIOD. Only a
// LATERAL ... LIMIT 1 collapses that to the single snapshot the dump is about.
func TestCostCalcDetailQuery_SelectsExactlyOneCostRow(t *testing.T) {
	if !strings.Contains(costCalcDetailQuery, "JOIN LATERAL (") {
		t.Fatalf("costCalcDetailQuery must select the cost snapshot through a LATERAL "+
			"subquery, got:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "LIMIT 1") {
		t.Fatalf("the cost-snapshot LATERAL must be capped at LIMIT 1 — without it one "+
			"row per non-SUPERSEDED period leaks through:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "x.cpc_calculation_type = $3") {
		t.Fatalf("the cost-snapshot LATERAL must pin the calculation type to $3, got:\n%s",
			costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "x.cpc_status <> 'SUPERSEDED'") {
		t.Fatalf("with no explicit status filter the LATERAL must still exclude "+
			"SUPERSEDED snapshots, got:\n%s", costCalcDetailQuery)
	}
}

// TestCostCalcDetailQuery_ResolvesOneGlobalPeriod pins fix (D): rates and tiers differ per
// period, so the whole workbook must describe ONE period. The requested period ($2) is
// used verbatim; only when it is empty does the target_period CTE resolve a single global
// MAX(cpc_period) for the entire export.
//
// ⛔ The old per-head "ORDER BY x.cpc_period DESC LIMIT 1" is what made the export
// silently mix periods: each head independently picked its own latest, so a workbook
// requested for one month contained rows from several. That ordering must not come back.
func TestCostCalcDetailQuery_ResolvesOneGlobalPeriod(t *testing.T) {
	if !strings.Contains(costCalcDetailQuery, "WITH target_period AS (") {
		t.Fatalf("costCalcDetailQuery must resolve the period once in a target_period "+
			"CTE, got:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "WHEN $2::text <> '' THEN $2::text") {
		t.Fatalf("an explicitly requested period ($2) must be used VERBATIM, never "+
			"overridden by a latest-period lookup, got:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "x.cpc_period = tp.period") {
		t.Fatalf("the cost-snapshot LATERAL must pin the resolved period, got:\n%s",
			costCalcDetailQuery)
	}
	if strings.Contains(costCalcDetailQuery, "cpc_period DESC") {
		t.Fatalf("the per-head 'ORDER BY cpc_period DESC' latest-period pick must be "+
			"gone — it silently mixed periods within one workbook:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "tp.period IS NOT NULL") {
		t.Fatalf("the query must yield zero rows (not every row) when no period could "+
			"be resolved at all, got:\n%s", costCalcDetailQuery)
	}
}

// TestCostCalcDetailQuery_MBIdentityColumns pins assumption (A)'s correction. A production
// lookup of mst_mb_head.mbh_mb_costing = 'CSTMB2607000001' returns ZERO rows: the CSTMB*
// code lives in the PRODUCT MASTER as a type-MB product, generated by
// generate_cost_product_code() (migration 000450 seeds cost_product_type 'MB'). So
// mb_code MUST come from cost_product_master.cpm_product_code, and mbh_mb_costing is the
// recipe NAME ("ALLOY GREY TR-712-A") that belongs in mb_name.
func TestCostCalcDetailQuery_MBIdentityColumns(t *testing.T) {
	if !strings.Contains(costCalcDetailQuery, "p.cpm_product_code,\n    h.mbh_mb_costing,") {
		t.Fatalf("mb_code must be cost_product_master.cpm_product_code and mb_name must "+
			"be mst_mb_head.mbh_mb_costing, projected in that order, got:\n%s",
			costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "JOIN cost_product_master p\n       ON p.cpm_product_sys_id = h.mbh_cost_product_id") {
		t.Fatalf("cost_product_master must be joined INNER on mbh_cost_product_id — an "+
			"MB with no linked cost product has no snapshot and cannot appear, got:\n%s",
			costCalcDetailQuery)
	}
}

// TestCostCalcDetailQuery_ResolvesRMGroupNameByGroupCode pins assumption (B)'s correction.
// There is NO mbcm_rm_group_code, rm_group_code or rm_group_name column anywhere in the
// schema: mst_mb_composition references a group by the UUID mbcm_group_head_id, and
// cst_rm_group_head's own columns are group_code / group_name (migration 000010). The
// GROUP-type ref_code stored in cpc_rm_cost_detail is cost_route_rm.crm_rm_group_code,
// which IS a group_code value — so ref_code -> group_code -> group_name is the only
// correct resolution path.
func TestCostCalcDetailQuery_ResolvesRMGroupNameByGroupCode(t *testing.T) {
	if !strings.Contains(costCalcDetailQuery, "ON g.group_code = (d.elem ->> 'ref_code')") {
		t.Fatalf("rm_group_name must resolve via cst_rm_group_head.group_code = ref_code, "+
			"got:\n%s", costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "g.group_name,") {
		t.Fatalf("rm_group_name must be projected from cst_rm_group_head.group_name, "+
			"got:\n%s", costCalcDetailQuery)
	}
	for _, phantom := range []string{"rm_group_code", "rm_group_name", "mbcm_rm_group_code"} {
		if strings.Contains(costCalcDetailQuery, phantom) {
			t.Fatalf("column %q does not exist in this schema — the real names are "+
				"group_code / group_name on cst_rm_group_head:\n%s", phantom, costCalcDetailQuery)
		}
	}
}

// TestCostCalcDetailQuery_NeverAggregatesRMLines pins the standing contract that the MB
// aggregates are READ from cpc_param_snapshot and never recomputed. SUM(cost_actual)
// disagrees with the stored mb_rm_cost for roughly half the MBs (2315 of 4188 match),
// because RM lines whose rate was missing at calc time contribute 0.
func TestCostCalcDetailQuery_NeverAggregatesRMLines(t *testing.T) {
	for _, banned := range []string{"SUM(", "sum(", "GROUP BY", "group by"} {
		if strings.Contains(costCalcDetailQuery, banned) {
			t.Fatalf("costCalcDetailQuery must never aggregate (found %q) — every mb_* "+
				"number is read from cpc_param_snapshot:\n%s", banned, costCalcDetailQuery)
		}
	}
	for _, want := range []string{"'MB_RM_COST'", "'MB_CONV_COST'", "'MB_FINAL_COST'", "'MB_NET_PROD'"} {
		if !strings.Contains(costCalcDetailQuery, want) {
			t.Errorf("expected the aggregate %s to be read from cpc_param_snapshot", want)
		}
	}
}

// TestResolveRMRateTier covers the full precedence:
//  1. flag_valuation_used, when it speaks the V2 vocabulary → used DIRECTLY
//  2. else valuation_flag_v2 → re-derived through the shared cascade
//  3. else AUTO cascade
//  4. PRODUCT-type rows (no cst_rm_cost match) → blank
func TestResolveRMRateTier(t *testing.T) {
	// The rate shapes below are borrowed from the production spot-check rows for
	// 202006004/DYE0000015 (202607: cl 14.77461060, sl 15.73355845, fl NULL,
	// pr 22.12671590 — 202609: cl NULL, sl 15.80083226, fl 1.9125, pr 22.12671590).
	tests := []struct {
		name string
		dto  costCalcDetailDTO
		want string
	}{
		// --- 1. V2 label passthrough -------------------------------------------------
		{
			name: "V2 label CL in flag_valuation_used is used directly",
			dto:  costCalcDetailDTO{HasRMCost: true, ValuationFlagV2: ns("SL"), FlagValuationUsed: ns("CL")},
			want: "CL",
		},
		{
			name: "V2 label PR wins over a conflicting valuation_flag_v2",
			dto:  costCalcDetailDTO{HasRMCost: true, ValuationFlagV2: ns("CL"), FlagValuationUsed: ns("PR")},
			want: "PR",
		},
		{
			name: "V2 label CR is in the vocabulary even though the cascade never picks it",
			dto:  costCalcDetailDTO{HasRMCost: true, FlagValuationUsed: ns("CR")},
			want: "CR",
		},
		{
			name: "V2 label SR is in the vocabulary even though the cascade never picks it",
			dto:  costCalcDetailDTO{HasRMCost: true, FlagValuationUsed: ns("SR")},
			want: "SR",
		},
		{
			name: "NONE in flag_valuation_used renders blank, never the literal NONE",
			dto:  costCalcDetailDTO{HasRMCost: true, FlagValuationUsed: ns("NONE")},
			want: "",
		},
		{
			// The 114 production NONE rows must render blank even though rate columns
			// are populated — NONE is authoritative, not an invitation to cascade.
			name: "NONE is authoritative and does NOT fall through to the cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("NONE"),
				CLRate: nf(14.77461060), SLRate: nf(15.73355845),
			},
			want: "",
		},

		// --- 2. The PRODUCTION case: v2 NULL, flag_valuation_used populated -----------
		{
			name: "production: valuation_flag_v2 NULL + flag_valuation_used CL",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: nsNull(), FlagValuationUsed: ns("CL"),
				CLRate: nf(14.77461060), SLRate: nf(15.73355845), PRRate: nf(22.12671590),
			},
			want: "CL",
		},
		{
			// 202006004/DYE0000015/202609: used=SL, cl_rate NULL, sl_rate 15.80083226,
			// fl_rate 1.9125. The old re-derive saw v2 NULL and returned "" for this row.
			name: "production: valuation_flag_v2 NULL + flag_valuation_used SL, cl_rate NULL",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: nsNull(), FlagValuationUsed: ns("SL"),
				SLRate: nf(15.80083226), FLRate: nf(1.9125), PRRate: nf(22.12671590),
			},
			want: "SL",
		},
		{
			name: "production: valuation_flag_v2 NULL + flag_valuation_used FL",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: nsNull(), FlagValuationUsed: ns("FL"),
				CLRate: nf(14.77461060), FLRate: nf(1.9125),
			},
			want: "FL",
		},

		// --- 3. Legacy V1 labels must NOT be trusted → fall through -------------------
		{
			name: "legacy V1 CONS falls through to the AUTO cascade (v2 also NULL)",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: nsNull(), FlagValuationUsed: ns("CONS"),
				CLRate: nf(14.77461060), SLRate: nf(15.73355845), PRRate: nf(22.12671590),
			},
			want: "CL", // cascade CL→SL→FL→PR picks the first non-zero
		},
		{
			name: "legacy V1 STORES falls through to the valuation_flag_v2 re-derive",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: ns("PR"), FlagValuationUsed: ns("STORES"),
				CLRate: nf(14.77461060), PRRate: nf(22.12671590),
			},
			want: "PR", // the configured v2 flag wins, not the V1 label, not the cascade
		},
		{
			name: "legacy V1 DEPT falls through to the cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("DEPT"),
				SLRate: nf(15.73355845), PRRate: nf(22.12671590),
			},
			want: "SL",
		},
		{
			name: "legacy V1 PO_1 falls through to the cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("PO_1"), PRRate: nf(22.12671590),
			},
			want: "PR",
		},
		{
			name: "legacy V1 INIT falls through to the cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("INIT"), FLRate: nf(1.9125),
			},
			want: "FL",
		},
		{
			name: "an unknown/garbage label is treated exactly like a V1 label",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("XX"), CLRate: nf(14.77461060),
			},
			want: "CL",
		},
		{
			name: "lowercase 'cl' is NOT a V2 label — the vocabulary is exact-match",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("cl"), SLRate: nf(15.73355845),
			},
			want: "SL", // cascade result, not the echoed "cl"
		},

		// --- 4. Empty / empty → cascade ----------------------------------------------
		{
			name: "both flags empty strings → AUTO cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: ns(""), FlagValuationUsed: ns(""),
				SLRate: nf(15.73355845), PRRate: nf(22.12671590),
			},
			want: "SL",
		},
		{
			name: "both flags NULL but the row exists → AUTO cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, ValuationFlagV2: nsNull(), FlagValuationUsed: nsNull(),
				PRRate: nf(22.12671590),
			},
			want: "PR",
		},
		{
			name: "empty flags with every candidate zero resolves to NONE → blank",
			dto:  costCalcDetailDTO{HasRMCost: true, ValuationFlagV2: ns(""), FlagValuationUsed: ns("")},
			want: "",
		},
		{
			name: "explicit AUTO in flag_valuation_used is not a tier label → cascade",
			dto: costCalcDetailDTO{
				HasRMCost: true, FlagValuationUsed: ns("AUTO"), CLRate: nf(14.77461060),
			},
			want: "CL",
		},

		// --- PRODUCT-type rows still render blank ------------------------------------
		{
			name: "PRODUCT row: no cst_rm_cost match at all → blank",
			dto:  costCalcDetailDTO{HasRMCost: false},
			want: "",
		},
		{
			name: "PRODUCT row stays blank even though the cascade would find nothing anyway",
			dto:  costCalcDetailDTO{HasRMCost: false, ValuationFlagV2: nsNull(), FlagValuationUsed: nsNull()},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.dto
			if got := resolveRMRateTier(&d); got != tt.want {
				t.Errorf("resolveRMRateTier() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestV2ValuationLabels_ExcludesLegacyV1 pins the guard set itself. The
// chk_rm_cost_flag_valuation_used CHECK constraint (migrations 000472/000501) still admits
// the V1 vocabulary, so this map — NOT the constraint — is what decides whether
// flag_valuation_used may be trusted. Widening it to the V1 labels would re-introduce the
// mislabeling the re-derive was originally written to prevent.
func TestV2ValuationLabels_ExcludesLegacyV1(t *testing.T) {
	for _, want := range []string{"CR", "SR", "PR", "CL", "SL", "FL", "NONE"} {
		if _, ok := v2ValuationLabels[want]; !ok {
			t.Errorf("v2ValuationLabels must contain the V2 label %q", want)
		}
	}
	for _, legacy := range []string{"CONS", "STORES", "DEPT", "PO_1", "PO_2", "PO_3", "INIT", "AUTO", ""} {
		if _, ok := v2ValuationLabels[legacy]; ok {
			t.Errorf("v2ValuationLabels must NOT contain %q — a non-V2 label must fall "+
				"through to the re-derive/cascade, not be echoed as a tier", legacy)
		}
	}
	if len(v2ValuationLabels) != 7 {
		t.Errorf("v2ValuationLabels has %d entries, want exactly 7 (CR/SR/PR/CL/SL/FL/NONE)",
			len(v2ValuationLabels))
	}
}

// TestCostCalcDetailQuery_RowNoIsPeriodOrdinal pins the row_no projection: a dense
// per-period ordinal read from the numeric suffix of cost_product_master.cpm_product_code.
//
// ⭐ row_no is a PRESENTATION row number, not an id. ⚠ It deliberately does NOT reproduce
// the reference export's seq values (this emits 1 for CSTMB2607000001 where the reference
// file shows 52939). That divergence is the DECISION this test protects — it must NOT be
// "fixed" by hardcoding a base offset or by wiring in a surrogate id.
//
// ⭐ RESOLVED, for the reader who wonders where the reference file's seq came from: it is
// cost_route_seq.crs_seq_id (production sequence cost_route_seq_crs_seq_id_seq last_value
// = 57931 = the file's max(seq), recon/production.txt:135; one cost_route_seq row per MB
// product via mb_autogen_repository.go:268-284 / :449-459). It is rejected on purpose —
// see the ban below and the row_no note on costCalcDetailQuery.
//
// ⛔ THREE BANNED SOURCES, all as the row_no projection:
//   - cst_product_cost.cpc_cost_id (production 421776/422973/422970/388073 — wrong range,
//     non-monotonic, moves with every recalc)
//   - cost_product_master.cpm_product_sys_id (production CSTMB2607000001 -> 36519)
//   - cost_route_seq.crs_seq_id (unstable: rows are deleted and reinserted)
func TestCostCalcDetailQuery_RowNoIsPeriodOrdinal(t *testing.T) {
	// The ordinal comes from the code suffix, not from any surrogate key.
	for _, want := range []string{
		"p.cpm_product_code FROM '([0-9]{6})$'",
		"AS row_no",
	} {
		if !strings.Contains(costCalcDetailQuery, want) {
			t.Fatalf("row_no must be projected as the per-period ordinal derived from the "+
				"cpm_product_code suffix (missing %q), got query:\n%s", want, costCalcDetailQuery)
		}
	}

	// row_no must sit immediately after the calc_status column, matching
	// costCalcDetailDTO's scan order (CalcVersion, CalcStatus, RowNo).
	const wantOrder = "COALESCE(pc.cpc_status, ''),\n    -- row_no:"
	if !strings.Contains(costCalcDetailQuery, wantOrder) {
		t.Fatalf("row_no must be projected directly after calc_status to match "+
			"costCalcDetailDTO's scan order, got query:\n%s", costCalcDetailQuery)
	}

	// No fabricated base offset may creep back in. These are the reference file's
	// per-period bases; row_no is a plain 1..N ordinal BY DECISION, so none of them
	// belongs in the SQL.
	for _, base := range []string{"52938", "52939", "57905", "57906"} {
		if strings.Contains(costCalcDetailQuery, base) {
			t.Fatalf("the query must not hardcode the reference file's per-period base %q — "+
				"row_no is a pure 1..N ordinal by decision, not an offset id:\n%s",
				base, costCalcDetailQuery)
		}
	}

	// ⛔ THE DELIBERATE REJECTION, not an oversight. cost_route_seq.crs_seq_id IS the
	// column the reference file's seq came from, and it is REFUSED because it is
	// UNSTABLE: cost_route_repository.go:364 deletes and reinserts cost_route_seq rows,
	// so ids are burned and reassigned (production: 34,219 live rows against a 57,931
	// counter, ~23.7k ids burned). Nothing reads this export back either — it is
	// write-only to a browser download, with no ImportMBCostCalcDetail RPC. So if you
	// arrived here intending to "helpfully" wire crs_seq_id in: don't. Reproducing the
	// reference file is not a goal; mb_code is the business key.
	for _, banned := range []string{"crs_seq_id", "cost_route_seq"} {
		if strings.Contains(costCalcDetailQuery, banned) {
			t.Fatalf("%q must NOT appear in costCalcDetailQuery — it is the reference "+
				"file's seq source and is DELIBERATELY rejected as unstable (rows are "+
				"deleted and reinserted), not overlooked:\n%s", banned, costCalcDetailQuery)
		}
	}

	// The ban: no dead source may be the row_no projection. cpc_cost_id may still
	// appear inside the snapshot-selecting LATERAL (legitimate tie-breaker); nothing
	// legitimises cpm_product_sys_id in the SELECT list.
	for _, banned := range []string{
		"COALESCE(pc.cpc_cost_id, 0)",
		"pc.cpc_cost_id AS row_no",
		"pc.cpc_cost_id AS seq",
		"pc.cpc_cost_id,\n    COALESCE(d.elem",
		"COALESCE(p.cpm_product_sys_id, 0)",
		"p.cpm_product_sys_id AS row_no",
		"p.cpm_product_sys_id AS seq",
		"p.cpm_product_sys_id,\n    COALESCE(d.elem",
	} {
		if strings.Contains(costCalcDetailQuery, banned) {
			t.Fatalf("dead row_no hypothesis resurrected (found %q) — both cpc_cost_id and "+
				"cpm_product_sys_id were disproven by production data:\n%s",
				banned, costCalcDetailQuery)
		}
	}

	// Guard the remaining legitimate uses so the bans above stay meaningful rather than
	// accidentally passing because the columns vanished from the query entirely.
	if !strings.Contains(costCalcDetailQuery, "ORDER BY x.cpc_version DESC, x.cpc_cost_id DESC") {
		t.Fatalf("the snapshot LATERAL must still tie-break on x.cpc_cost_id DESC — "+
			"that is cpc_cost_id's only legitimate use in this query, got:\n%s",
			costCalcDetailQuery)
	}
	if !strings.Contains(costCalcDetailQuery, "ON p.cpm_product_sys_id = h.mbh_cost_product_id") {
		t.Fatalf("cost_product_master must still be joined on cpm_product_sys_id — that "+
			"is the column's only legitimate use here, got:\n%s", costCalcDetailQuery)
	}
}
