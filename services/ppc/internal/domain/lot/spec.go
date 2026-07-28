// Package lot provides domain logic for PPC lot-master data.
package lot

import "time"

// Lot provenance markers, mirroring the chk_lot_master_source constraint.
const (
	// SourcePPC marks a lot minted by the area-based sequence at work-order
	// creation (workorder.FormatLotNo).
	SourcePPC = "PPC"
	// SourceMMSMERGE marks a lot imported from the legacy Oracle lot master
	// ASPAK.MMSMERGE.
	SourceMMSMERGE = "MMSMERGE"
)

// Spec is the yarn and packing specification a lot carries. Every field is
// optional: a PPC-generated lot has none of it, and even an imported MMSMERGE
// row leaves most of it empty (the sampled rows have no SHADE_CODE at all).
//
// It is a plain value struct rather than a set of Master fields because it is
// wholly source-owned — PPC edits it on the lot screen but never derives
// invariants from it, so validating it inside the aggregate would buy nothing.
type Spec struct {
	// ProdType is MERGE_PROD_TYPE: PTY, POY or FOY.
	ProdType string
	// YarnType is YARN_TYPE. It overlaps ProdType but is not the same field —
	// the FOY sample carries PROD_TYPE=FOY with YARN_TYPE=BSD.
	YarnType string
	// Denier is MERGE_DENIER. Text in the source (VARCHAR2), so text here:
	// values like "150" coexist with grades that are not plain integers.
	Denier string
	// Filament is MERGE_FILAMENT (count of filaments).
	Filament *int32
	// CrossSection is MERGE_CROSS_SEC: RND (round) or TBL (trilobal).
	CrossSection string
	// QCGrade is MERGE_QC_GRADE (AX, Aa, ...).
	QCGrade string
	// Description is MERGE_DESCR, the legacy spec shorthand ("SD HCSH NI").
	Description string
	// ShadeColor is the human-readable colour, taken from SHADE_COLOR and
	// falling back to MERGE_COLOR / YARN_COLOR when that is empty.
	ShadeColor string
	// TareBoxWeight is MERGE_TARE_BOX_WT — the empty carton's weight in kg.
	TareBoxWeight *float64
	// TareBobbinWeight is MERGE_TARE_BOBIN_WT — the empty bobbin's weight in kg.
	TareBobbinWeight *float64
	// BobbinsPerBox is the carton fill count, NVL(MERGE_NOBOB, MERGE_BOX).
	BobbinsPerBox *int32
	// SourceBobWeight is MERGE_BOB verbatim, provisionally read as kilograms of
	// yarn per bobbin. UNCONFIRMED — see the migration comment. It seeds a new
	// lot's full standard weight and is otherwise informational.
	SourceBobWeight *float64
	// OrionItemCode is MERGE_ITEM_ORION, the ERP item this lot maps to.
	OrionItemCode string
	// MachineNo is MERGE_MACHINE, the machine the lot is habitually run on.
	MachineNo string
	// EfficiencyPct is MERGE_EFF.
	EfficiencyPct *int32
	// SourceStatus is MERGE_STATUS verbatim.
	SourceStatus string
	// SourcePakStatus is MERGE_PAK_STATUS verbatim. The Oracle packing
	// procedure excludes 'B', so the flag has to survive the import.
	SourcePakStatus string
}

// SourcedLot is a lot projection assembled from a sync source ahead of an
// UpsertSourced, mirroring machine.SourcedMachine.
type SourcedLot struct {
	// LotNo is the natural key (MERGE_CODE).
	LotNo string
	// SourceKey is the source's own key, kept verbatim alongside LotNo so a
	// future lot-number reformat does not lose the join back to Oracle.
	SourceKey string
	// ItemCode is MERGE_ITEM_CODE.
	ItemCode string
	// ShadeCode is SHADE_CODE; empty on every sampled row, in which case the
	// existing PPC value is preserved.
	ShadeCode string
	// StdWeightFull seeds lm_std_weight_full on insert only. Nil when the source
	// has no usable figure (MERGE_BOB = 0), leaving the column NULL.
	StdWeightFull *float64
	// Spec is the source-owned specification.
	Spec Spec
	// SyncedAt is the sync timestamp stamped on the row.
	SyncedAt time.Time
}

// UpsertOutcome reports the result of a sourced-lot merge.
type UpsertOutcome int

// Sourced-lot merge outcomes.
const (
	// OutcomeSkipped means no row was written (e.g. a source row with no lot
	// number).
	OutcomeSkipped UpsertOutcome = iota
	// OutcomeInserted means a new lot row was created.
	OutcomeInserted
	// OutcomeUpdated means an existing lot row was merged.
	OutcomeUpdated
)

// UpsertBatchResult reports how a batch of sourced lots landed.
type UpsertBatchResult struct {
	// Inserted counts lots that did not previously exist.
	Inserted int
	// Updated counts lots that were merged into an existing row.
	Updated int
	// Skipped counts entries the repository refused to write (missing key).
	Skipped int
}

// Add accumulates another batch's counts.
func (r *UpsertBatchResult) Add(other UpsertBatchResult) {
	r.Inserted += other.Inserted
	r.Updated += other.Updated
	r.Skipped += other.Skipped
}
