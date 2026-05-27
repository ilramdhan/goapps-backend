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
	FilledAt     time.Time
	FilledBy     string
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    *time.Time
	UpdatedBy    *string
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
}

// RequiredEntry is ParamMeta + the existing Value (zero when unbound).
type RequiredEntry struct {
	Meta  ParamMeta
	Value *Value // nil = not yet filled
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
