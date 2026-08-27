package mbhead

import "context"

// RecipeFullFilter narrows the denormalized full-recipe export (P12, items C1 + C2).
//
// IsActive nil means "no active filter" — absence stays absence (D13), it is never
// coerced to a boolean default.
//
// Period empty means "the latest active period per head", resolved per head rather
// than globally, so a head whose newest push is older than another head's still
// exports its own newest cost instead of a blank.
//
// CostType empty is resolved by the handler to DefaultExportCostType before it ever
// reaches the reader. Restricting to one cost type is what keeps the row count at
// n_composition rather than n_composition × n_cost_type.
//
// CheckStatusCalc empty means ALL ROWS — including the heads whose derived status is
// still NULL. ⛔ It is NEVER defaulted anywhere: unlike CostType, an empty value here
// is a real, meaningful selection ("do not filter"), so coercing it would silently
// drop the "Belum dihitung" heads from a default export. See the handler for the note
// on which of the six legal values are actually produced today.
type RecipeFullFilter struct {
	IsActive *bool
	Period   string
	CostType string
	// CheckStatusCalc filters mst_mb_head.mbh_check_status_calc exactly. Empty = no
	// filter. A non-empty value necessarily EXCLUDES NULL-status heads, because SQL
	// equality never matches NULL.
	CheckStatusCalc string
	// IncludeRejected, when false (the zero value), EXCLUDES heads whose workflow
	// status (mbh_entry_status) is mbhead.StatusRejected from the export. Safe
	// default — mirrors mbhead.ExportFilter.IncludeRejected and closes the same leak
	// for the full-recipe export (§11 item 140).
	//
	// ⚠ PENDING FOLLOW-UP: no proto field exposes this yet, so every gRPC caller gets
	// the zero value (false) today. See mbhead.ExportFilter.IncludeRejected for the
	// full note.
	IncludeRejected bool
}

// RecipeFullRow is one denormalized export row: the recipe header repeated across
// every composition line, plus the MB cost block and the traceability block.
//
// Every optional column is a pointer precisely so that "absent" and "zero" stay
// distinguishable all the way into the spreadsheet (D13). A nil Denier must render
// as an empty cell, never as 0.
type RecipeFullRow struct {
	// Recipe block (repeated on every row belonging to the same head).
	MBCosting    string
	MgtName      *string
	Code         *string
	DevCode      string
	VSNumber     *string
	NoOfProcess  *string
	ShadeCode    string
	ShadeName    string
	Shade2Code   *string
	Shade2Name   *string
	Shade3Code   *string
	Shade3Name   *string
	Denier       *float64
	Filament     *int
	CrossSection string
	LustureCode  string
	LdrPct       *float64
	Dozing       *float64
	Status       *string
	// CheckStatusLegacy maps to the FROZEN mbh_check_status column. It is exported
	// read-only and is never written from any code path.
	CheckStatusLegacy *string
	// CheckStatusCalc maps to mbh_check_status_calc (P10). nil means the application
	// has never derived a value for this head; it is rendered EXPLICITLY (never as an
	// empty cell), matching the UI's "Belum dihitung".
	CheckStatusCalc *string
	FinalProduct    *string
	IsBoughtout     bool
	EntryStatus     string

	// Composition block. All nil when the head has no composition rows at all — such a
	// head still produces exactly one row with these columns blank.
	CompSeqNo      *int32
	CompSourceType *string
	// CompRMGroupCode is cst_rm_group_head.group_code for the referenced group.
	CompRMGroupCode *string
	// CompMBRefCosting is the mbh_mb_costing of the referenced MB, for source_type = MB.
	CompMBRefCosting *string
	CompPct          *string
	CompIsCarrier    *bool

	// MB cost block, read from cst_mb_cost. All nil when this head has no pushed cost
	// for the selected period/type — that is a normal outcome, not an error.
	CostPeriod   *string
	CostType     *string
	CostValue    *string
	CostPushedAt *string

	// Traceability block.
	CostProductSysID int64
	CostProductCode  *string
	CostGeneratedAt  *string
}

// RecipeFullReader is the single read-only port the full-recipe export depends on.
//
// ⛔ Read-only by contract: the export must never issue an UPDATE. cst_mb_cost and
// cst_product_cost are joined for display only.
type RecipeFullReader interface {
	ListRecipeFullRows(ctx context.Context, filter RecipeFullFilter) ([]RecipeFullRow, error)
}
