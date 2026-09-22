package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// allNamedFormulaTypes pairs every non-UNSPECIFIED FormulaType enum value with the
// exact DB string stored in mst_formula.formula_type (see the CHECK constraint added
// by migrations/postgres/000402_extend_formula_type.up.sql). Any enum value added to
// the proto in the future must be added here too, or this test will start failing to
// cover it once the switch statements are extended.
func allNamedFormulaTypes() []struct {
	proto financev1.FormulaType
	str   string
} {
	return []struct {
		proto financev1.FormulaType
		str   string
	}{
		{financev1.FormulaType_FORMULA_TYPE_CALCULATION, "CALCULATION"},
		{financev1.FormulaType_FORMULA_TYPE_SQL_QUERY, "SQL_QUERY"},
		{financev1.FormulaType_FORMULA_TYPE_CONSTANT, "CONSTANT"},
		{financev1.FormulaType_FORMULA_TYPE_CONDITIONAL, "CONDITIONAL"},
		{financev1.FormulaType_FORMULA_TYPE_LOOKUP, "LOOKUP"},
		{financev1.FormulaType_FORMULA_TYPE_RM_LOOKUP, "RM_LOOKUP"},
		{financev1.FormulaType_FORMULA_TYPE_FROM_MARKETING, "FROM_MARKETING"},
		{financev1.FormulaType_FORMULA_TYPE_INTERMINGLING, "INTERMINGLING"},
		{financev1.FormulaType_FORMULA_TYPE_SNAPSHOT, "SNAPSHOT"},
		{financev1.FormulaType_FORMULA_TYPE_PENDING, "PENDING"},
		{financev1.FormulaType_FORMULA_TYPE_INITIAL_VALUE, "INITIAL_VALUE"},
	}
}

func TestProtoFormulaTypeToString_AllNamedValues(t *testing.T) {
	for _, tt := range allNamedFormulaTypes() {
		t.Run(tt.str, func(t *testing.T) {
			assert.Equal(t, tt.str, protoFormulaTypeToString(tt.proto))
		})
	}
}

func TestProtoFormulaTypeToString_Unspecified_ReturnsEmptyString(t *testing.T) {
	assert.Empty(t, protoFormulaTypeToString(financev1.FormulaType_FORMULA_TYPE_UNSPECIFIED))
}

func TestStringToProtoFormulaType_AllNamedValues(t *testing.T) {
	for _, tt := range allNamedFormulaTypes() {
		t.Run(tt.str, func(t *testing.T) {
			assert.Equal(t, tt.proto, stringToProtoFormulaType(tt.str))
		})
	}
}

func TestStringToProtoFormulaType_UnrecognizedOrEmpty_ReturnsUnspecified(t *testing.T) {
	tests := []string{"", "NOT_A_TYPE", "calculation"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, financev1.FormulaType_FORMULA_TYPE_UNSPECIFIED, stringToProtoFormulaType(in))
		})
	}
}

func TestFormulaTypeMapping_RoundTrip(t *testing.T) {
	// Every named enum value must survive a proto -> string -> proto round trip
	// unchanged; this is exactly the path the formula edit UI relies on when
	// loading, then re-saving, a formula of any RM_LOOKUP/CONDITIONAL/etc. type.
	for _, tt := range allNamedFormulaTypes() {
		t.Run(tt.str, func(t *testing.T) {
			roundTripped := stringToProtoFormulaType(protoFormulaTypeToString(tt.proto))
			assert.Equal(t, tt.proto, roundTripped)
		})
	}
}
