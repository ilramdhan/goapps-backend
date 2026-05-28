package upload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRow_Valid(t *testing.T) {
	raw := RawRow{
		Type: "MIS", Group1: "EBITDA", Group2: "INCOME",
		Grain: "MONTHLY", Periode: "202604", Value: "1000",
		Group1Order: "2", Scenario: "actual",
	}
	row, errs := ValidateRow(raw, "MIS", 2)
	require.Empty(t, errs)
	assert.Equal(t, ValidationValid, row.ValidationStatus)
	assert.Equal(t, "MONTHLY", row.PeriodGrain)
	assert.Equal(t, "202604", row.PeriodLabel)
	assert.InDelta(t, 1000.0, row.Value, 1e-9)
	assert.Equal(t, 2, row.Group1Order)
	assert.Equal(t, "ACTUAL", row.Scenario)
}

func TestValidateRow_BadGrain(t *testing.T) {
	raw := RawRow{Type: "MIS", Group1: "EBITDA", Grain: "WEEKLY", Periode: "202604", Value: "1"}
	row, errs := ValidateRow(raw, "MIS", 3)
	require.NotEmpty(t, errs)
	assert.Equal(t, ValidationInvalid, row.ValidationStatus)
	assert.Equal(t, colGrain, errs[0].Column)
}

func TestValidateRow_NonNumericValue(t *testing.T) {
	raw := RawRow{Type: "MIS", Group1: "EBITDA", Grain: "MONTHLY", Periode: "202604", Value: "abc"}
	row, errs := ValidateRow(raw, "MIS", 4)
	require.NotEmpty(t, errs)
	assert.Equal(t, ValidationInvalid, row.ValidationStatus)
	found := false
	for _, e := range errs {
		if e.Column == colValue {
			found = true
		}
	}
	assert.True(t, found, "expected a VALUE error")
}

func TestValidateRow_BadDate(t *testing.T) {
	raw := RawRow{Type: "MIS", Group1: "EBITDA", Grain: "DAILY", Periode: "2026-13-99", Value: "1"}
	_, errs := ValidateRow(raw, "MIS", 5)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Column == colPeriode {
			found = true
		}
	}
	assert.True(t, found, "expected a PERIODE error")
}

func TestValidateRow_DailyDateParses(t *testing.T) {
	raw := RawRow{Type: "MIS", Group1: "EBITDA", Grain: "DAILY", Periode: "2026-04-15", Value: "1"}
	row, errs := ValidateRow(raw, "MIS", 6)
	require.Empty(t, errs)
	assert.Equal(t, "2026-04-15", row.PeriodLabel)
	assert.Equal(t, 2026, row.PeriodDate.Year())
}

func TestValidateRow_TypeMismatch(t *testing.T) {
	raw := RawRow{Type: "SALES", Group1: "EBITDA", Grain: "MONTHLY", Periode: "202604", Value: "1"}
	row, errs := ValidateRow(raw, "MIS", 7)
	require.NotEmpty(t, errs)
	assert.Equal(t, ValidationInvalid, row.ValidationStatus)
	assert.Equal(t, colType, errs[0].Column)
}

func TestValidateRow_Group1Required(t *testing.T) {
	raw := RawRow{Type: "MIS", Grain: "MONTHLY", Periode: "202604", Value: "1"}
	_, errs := ValidateRow(raw, "MIS", 8)
	require.NotEmpty(t, errs)
}

func TestComputeDisplayValue_IncomeFlipsNegative(t *testing.T) {
	got := ComputeDisplayValue(typeMIS, groupEBITDA, groupIncome, 500)
	assert.InDelta(t, -500.0, got, 1e-9)
}

func TestComputeDisplayValue_CostFlipsNegative(t *testing.T) {
	assert.InDelta(t, -300.0, ComputeDisplayValue(typeMIS, groupNetProfit, "MANPOWER", 300), 1e-9)
	assert.InDelta(t, -300.0, ComputeDisplayValue(typeMIS, groupEBITDA, "RM COST", 300), 1e-9)
	assert.InDelta(t, -300.0, ComputeDisplayValue(typeMIS, groupEBITDA, "RM CONSUMPTION", 300), 1e-9)
}

func TestComputeDisplayValue_OtherUnchanged(t *testing.T) {
	assert.InDelta(t, 400.0, ComputeDisplayValue(typeMIS, groupEBITDA, "EBITDA", 400), 1e-9)
	assert.InDelta(t, 400.0, ComputeDisplayValue("SALES", "ANY", "ANY", 400), 1e-9)
	assert.InDelta(t, 400.0, ComputeDisplayValue(typeMIS, "OTHER", groupIncome, 400), 1e-9)
}

func TestBusinessKey_Distinguishes(t *testing.T) {
	a := StagingRow{Type: "MIS", Group1: "EBITDA", PeriodGrain: "MONTHLY", Scenario: "ACTUAL"}
	b := a
	b.Group1 = "NET PROFIT"
	assert.NotEqual(t, BusinessKey(a), BusinessKey(b))
}
