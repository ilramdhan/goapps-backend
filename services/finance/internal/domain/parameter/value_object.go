// Package parameter provides domain logic for Parameter management.
package parameter

import (
	"regexp"
	"strings"
)

// =============================================================================
// Code Value Object
// =============================================================================

// Code represents a validated parameter code value object.
type Code struct {
	value string
}

// codePattern defines the valid format for parameter codes.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// NewCode creates a new validated Code value object.
func NewCode(code string) (Code, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Code{}, ErrEmptyCode
	}
	if len(code) > 20 {
		return Code{}, ErrCodeTooLong
	}
	if !codePattern.MatchString(code) {
		return Code{}, ErrInvalidCodeFormat
	}
	return Code{value: code}, nil
}

// String returns the string representation of the code.
func (c Code) String() string {
	return c.value
}

// IsEmpty returns true if the code is empty.
func (c Code) IsEmpty() bool {
	return c.value == ""
}

// Equals checks if two codes are equal.
func (c Code) Equals(other Code) bool {
	return c.value == other.value
}

// =============================================================================
// DataType Value Object
// =============================================================================

// DataType represents a validated parameter data type.
type DataType string

// DataType constants.
const (
	DataTypeNumber  DataType = "NUMBER"
	DataTypeText    DataType = "TEXT"
	DataTypeBoolean DataType = "BOOLEAN"
)

// validDataTypes is a set of valid data type values.
var validDataTypes = map[DataType]bool{
	DataTypeNumber:  true,
	DataTypeText:    true,
	DataTypeBoolean: true,
}

// NewDataType creates a new validated DataType value object.
func NewDataType(dt string) (DataType, error) {
	d := DataType(strings.ToUpper(strings.TrimSpace(dt)))
	if !validDataTypes[d] {
		return "", ErrInvalidDataType
	}
	return d, nil
}

// String returns the string representation of the data type.
func (d DataType) String() string {
	return string(d)
}

// IsValid returns true if the data type is valid.
func (d DataType) IsValid() bool {
	return validDataTypes[d]
}

// AllDataTypes returns a slice of all valid data types.
func AllDataTypes() []DataType {
	return []DataType{DataTypeNumber, DataTypeText, DataTypeBoolean}
}

// =============================================================================
// ParamCategory Value Object
// =============================================================================

// ParamCategory represents a validated parameter category.
type ParamCategory string

// ParamCategory constants.
const (
	ParamCategoryInput      ParamCategory = "INPUT"
	ParamCategoryRate       ParamCategory = "RATE"
	ParamCategoryCalculated ParamCategory = "CALCULATED"
)

// validParamCategories is a set of valid parameter category values.
var validParamCategories = map[ParamCategory]bool{
	ParamCategoryInput:      true,
	ParamCategoryRate:       true,
	ParamCategoryCalculated: true,
}

// NewParamCategory creates a new validated ParamCategory value object.
func NewParamCategory(cat string) (ParamCategory, error) {
	c := ParamCategory(strings.ToUpper(strings.TrimSpace(cat)))
	if !validParamCategories[c] {
		return "", ErrInvalidParamCategory
	}
	return c, nil
}

// String returns the string representation of the category.
func (c ParamCategory) String() string {
	return string(c)
}

// IsValid returns true if the category is valid.
func (c ParamCategory) IsValid() bool {
	return validParamCategories[c]
}

// AllParamCategories returns a slice of all valid parameter categories.
func AllParamCategories() []ParamCategory {
	return []ParamCategory{ParamCategoryInput, ParamCategoryRate, ParamCategoryCalculated}
}
