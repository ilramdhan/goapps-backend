// Package rmgroup provides domain logic for raw-material grouping and landed-cost configuration.
package rmgroup

import (
	"regexp"
	"strings"
)

// codePattern validates the group code format — uppercase alphanumeric with optional
// spaces and hyphens, starting with alphanumeric. Max 30 characters.
// Mirrors the chk_rm_group_code_format CHECK constraint on cst_rm_group_head.
var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9 \-]{0,29}$`)

// Code represents a validated RM group code (e.g., "BLUE MGTS-5109", "PIG0000005-COM").
type Code struct {
	value string
}

// NewCode creates a Code value object with validation.
func NewCode(raw string) (Code, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Code{}, ErrEmptyCode
	}
	if len(trimmed) > 30 {
		return Code{}, ErrCodeTooLong
	}
	// Normalize letters to uppercase; preserve spaces and hyphens.
	normalized := strings.ToUpper(trimmed)
	if !codePattern.MatchString(normalized) {
		return Code{}, ErrInvalidCodeFormat
	}
	return Code{value: normalized}, nil
}

// String returns the canonical string form.
func (c Code) String() string { return c.value }

// IsEmpty returns true if the code has no value.
func (c Code) IsEmpty() bool { return c.value == "" }

// Equal reports whether two codes are equal.
func (c Code) Equal(other Code) bool { return c.value == other.value }

// ItemCode represents a validated raw-material item code (mirrors cst_item_cons_stk_po.item_code).
type ItemCode struct {
	value string
}

// NewItemCode creates an ItemCode value object with validation.
func NewItemCode(raw string) (ItemCode, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ItemCode{}, ErrEmptyItemCode
	}
	if len(trimmed) > 20 {
		return ItemCode{}, ErrItemCodeTooLong
	}
	return ItemCode{value: trimmed}, nil
}

// String returns the item code.
func (i ItemCode) String() string { return i.value }

// IsEmpty returns true if the item code is empty.
func (i ItemCode) IsEmpty() bool { return i.value == "" }

// Flag represents the stage selector used to pick which rate feeds the landed cost formula.
// Mirrors the CHECK-constrained values on cst_rm_group_head.flag_*.
type Flag string

// Flag constants — MUST match DB CHECK constraint values exactly.
const (
	// FlagCons selects the consumption-aggregated rate.
	FlagCons Flag = "CONS"
	// FlagStores selects the stores-aggregated rate.
	FlagStores Flag = "STORES"
	// FlagDept selects the department-aggregated rate.
	FlagDept Flag = "DEPT"
	// FlagPO1 selects purchase-order slot 1 rate.
	FlagPO1 Flag = "PO_1"
	// FlagPO2 selects purchase-order slot 2 rate.
	FlagPO2 Flag = "PO_2"
	// FlagPO3 selects purchase-order slot 3 rate.
	FlagPO3 Flag = "PO_3"
	// FlagInit signals that the init_val override should be used instead of any aggregated rate.
	FlagInit Flag = "INIT"
)

// ParseFlag validates and returns a Flag, or ErrInvalidFlag if the value is unknown.
func ParseFlag(raw string) (Flag, error) {
	switch Flag(strings.ToUpper(strings.TrimSpace(raw))) {
	case FlagCons:
		return FlagCons, nil
	case FlagStores:
		return FlagStores, nil
	case FlagDept:
		return FlagDept, nil
	case FlagPO1:
		return FlagPO1, nil
	case FlagPO2:
		return FlagPO2, nil
	case FlagPO3:
		return FlagPO3, nil
	case FlagInit:
		return FlagInit, nil
	default:
		return "", ErrInvalidFlag
	}
}

// IsValid reports whether the flag is one of the recognised values.
func (f Flag) IsValid() bool {
	switch f {
	case FlagCons, FlagStores, FlagDept, FlagPO1, FlagPO2, FlagPO3, FlagInit:
		return true
	default:
		return false
	}
}

// IsInit reports whether the flag requests the init_val override.
func (f Flag) IsInit() bool { return f == FlagInit }

// String returns the canonical string form.
func (f Flag) String() string { return string(f) }
