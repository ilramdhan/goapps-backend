// Package costproductparameter is the per-product static parameter value
// domain (CPP_). One row binds a single parameter to a product with a value.
// Exactly one of valueNumeric / valueText / valueFlag is populated per row,
// matching the data_type of the referenced parameter.
package costproductparameter

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// DataType mirrors the mst_parameter.data_type domain check.
type DataType string

// Allowed data types.
const (
	DataTypeNumber  DataType = "NUMBER"
	DataTypeText    DataType = "TEXT"
	DataTypeBoolean DataType = "BOOLEAN"
)

// Sentinel errors.
var (
	ErrNotFound           = errors.New("product parameter value not found")
	ErrInvalidValueShape  = errors.New("exactly one value column must be populated")
	ErrInvalidDataType    = errors.New("invalid data_type for parameter")
	ErrProductNotFound    = errors.New("product not found")
	ErrParamNotFound      = errors.New("parameter not found")
	ErrPeriodDependent    = errors.New("parameter is period-dependent and cannot be stored in CPP")
	ErrParamNotApplicable = errors.New("parameter not in product's applicable list — add it first")
	ErrProductLocked      = errors.New("product is locked — route and parameters cannot be edited")
)

// Applicability is the per-product CAPP row metadata (no value).
type Applicability struct {
	CappID       int64
	ProductSysID int64
	ParamID      uuid.UUID
	IsRequired   bool
	DisplayOrder *int32 // nil = inherit from mst_parameter.display_order
	CreatedBy    string
	CreatedAt    time.Time
}

// Value is the CPP_ row aggregate.
type Value struct {
	ValueID      int64
	ProductSysID int64
	ParamID      uuid.UUID
	ValueNumeric *string // decimal as string for precision
	ValueText    *string
	ValueFlag    *bool
	// ValueMBSpinID is the resolved mst_mb_spin.mbs_id companion for MB_SPIN
	// lookup parameters (cpp_value_mb_spin_id, migration 000494). It is a
	// COMPANION to ValueText, never a replacement: ValueText keeps carrying
	// whatever the user selected (UUID or legacy ORION code) exactly as
	// before, and this field stays nil unless the value was resolved to
	// EXACTLY one mst_mb_spin row. Ambiguous (0 or >1 matches) resolutions
	// leave this nil rather than guessing — see
	// costproductparameter.Handlers.Upsert.
	ValueMBSpinID *uuid.UUID
	// MBSpinCandidateCount is the number of active-or-not mst_mb_spin rows
	// whose mbs_orion_item_code matches ValueText, computed AT READ TIME by
	// ListForProduct — never stored. It exists to let the UI tell apart three
	// states without guessing: (1) already resolved (ValueMBSpinID != nil,
	// this field is irrelevant), (2) unresolved with >1 candidates ("pick a
	// variant"), and (3) unresolved with 0 candidates ("code not found, fix
	// the data"). It is computed at read time rather than stored because
	// mst_mb_spin can change (variants added/soft-deleted) after the CPP row
	// was last saved, and a stored count would silently go stale with no
	// resync mechanism — see docs/superpowers/mbspin-tanda-varian-ganda-rancangan.md
	// §3 opsi (ii-a) vs (ii-b). Nil means "not applicable" (either this row
	// isn't an MB_SPIN lookup parameter, or ValueText is empty) — never
	// conflate nil with zero.
	MBSpinCandidateCount *int32
	FilledAt             time.Time
	FilledBy             string
	CreatedAt            time.Time
	CreatedBy            string
	UpdatedAt            *time.Time
	UpdatedBy            *string
}

// ParamMeta is the joined mst_parameter snapshot needed by the form / resolver.
type ParamMeta struct {
	ParamID              uuid.UUID
	ParamCode            string
	ParamName            string
	ParamShortName       string
	DataType             string
	ParamCategory        string
	UOMCode              string
	OwnerDepartment      string
	IsRequiredForCosting bool
	IsPeriodDependent    bool
	LookupMasterCode     string
	DisplayOrder         int32
	DisplayGroup         string
	LookupFillGroupCode  string // empty = not a child param
	LookupSourceColumn   string
}

// RemovePreview lists the trigger param and all child params (with current values) for confirmation UI.
type RemovePreview struct {
	TriggerParamCode string
	TriggerParamName string
	Children         []ChildPreview
}

// ChildPreview holds one child param's display info for the confirm dialog.
type ChildPreview struct {
	ParamCode    string
	ParamName    string
	CurrentValue string // formatted string, empty if not filled
}

// RequiredEntry is ParamMeta + the existing Value (zero when unbound).
type RequiredEntry struct {
	Meta  ParamMeta
	Value *Value // nil = not yet filled
}

// CAPPRow is a flat representation of a cost_product_applicable_param row used
// for export and reporting.
type CAPPRow struct {
	ProductCode  string
	ParamCode    string
	IsRequired   bool
	DisplayOrder *int32
}

// CPPRow is a flat representation of a cost_product_parameter row used for export.
type CPPRow struct {
	ProductCode  string
	ParamCode    string
	ValueNumeric *string
	ValueText    *string
	ValueFlag    *bool
}

// CPPUpsertInput is a single row for BulkUpsertValues.
type CPPUpsertInput struct {
	ProductSysID int64
	ParamID      uuid.UUID
	ValueNumeric *float64 // nil when not set
	ValueText    *string  // nil when not set
	ValueFlag    *bool    // nil when not set
	FilledAt     time.Time
	FilledBy     string
}

// CAPPUpsertInput is a single row for BulkUpsertApplicable.
type CAPPUpsertInput struct {
	ProductSysID int64
	ParamID      uuid.UUID
	IsRequired   bool
	DisplayOrder *int32 // nil when not provided
}

// EnsureValueShape verifies that the (numeric|text|flag) triple has exactly
// one populated field and that it matches the declared data_type.
func EnsureValueShape(dataType string, valueNumeric, valueText *string, valueFlag *bool) error {
	count := 0
	if valueNumeric != nil {
		count++
	}
	if valueText != nil {
		count++
	}
	if valueFlag != nil {
		count++
	}
	if count != 1 {
		return ErrInvalidValueShape
	}

	switch DataType(dataType) {
	case DataTypeNumber:
		if valueNumeric == nil {
			return ErrInvalidDataType
		}
	case DataTypeText:
		if valueText == nil {
			return ErrInvalidDataType
		}
	case DataTypeBoolean:
		if valueFlag == nil {
			return ErrInvalidDataType
		}
	default:
		return ErrInvalidDataType
	}
	return nil
}
