// Package lookupmaster provides the domain for the mst_lookup_master registry.
package lookupmaster

// LookupMaster represents one registered master table available for MASTER_LOOKUP params.
type LookupMaster struct {
	Code        string
	DisplayName string
	APIPath     string
	CodeField   string
	LabelField  string
	TableName   string
	IsActive    bool
}

// Column represents one fillable column for a master.
type Column struct {
	ID          string // UUID primary key.
	MasterCode  string
	ColumnName  string
	DisplayName string
	DataType    string // "NUMBER" or "TEXT"
	SortOrder   int
}

// UpdateMaster carries the mutable fields for UpdateLookupMaster.
type UpdateMaster struct {
	DisplayName *string
	TableName   *string
	IsActive    *bool
}

// TableColumn is one column from information_schema introspection.
type TableColumn struct {
	ColumnName      string
	DataType        string // "NUMBER" or "TEXT"
	RawType         string // e.g., "numeric", "character varying"
	OrdinalPosition int
}

// MasterOption is one combobox entry (value + label) returned by ListMasterOptions.
type MasterOption struct {
	Value string
	Label string
	// ⭐ DIPERBARUI 2026-08-26 (U-mbspin-lookup-detail): Denier, Filament, LdrPrsn,
	// and RunLdrPct are only populated when the option's source table is
	// mst_mb_spin (see LookupMasterRepository.ListMasterOptions) — nil for every
	// other lookup master.
	// ~~D30: Dozing is sourced from the retired/contaminated mbs_dozing column
	// (mixes oil-dozing-rate ~0.03 scale with run_ldr ~3.55 scale across
	// different MB Heads). Surfaced as-is per explicit product decision; not an
	// authoritative LDR value.~~
	// Dozing was withdrawn by explicit user decision on 2026-08-26 (D30
	// contamination) and replaced by LdrPrsn ("LDR Rencana (%)", from
	// mbs_ldr_prsn) and RunLdrPct ("LDR Aktual (%)", from mbs_run_ldr_pct) —
	// both unambiguous, uncontaminated columns.
	Denier    *float64
	Filament  *int
	LdrPrsn   *float64
	RunLdrPct *float64
}
