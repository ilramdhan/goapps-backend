package upload

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Grain values accepted in the PERIODE_GRAIN column.
const (
	grainDaily     = "DAILY"
	grainMonthly   = "MONTHLY"
	grainQuarterly = "QUARTERLY"
	grainYearly    = "YEARLY"
)

// Sign-convention constants (replicate bi_compute_display_value).
const (
	typeMIS         = "MIS"
	groupEBITDA     = "EBITDA"
	groupNetProfit  = "NET PROFIT"
	groupIncome     = "INCOME"
	scenarioDefault = "ACTUAL"
)

// Column header names (canonical, case-insensitive match).
const (
	colType        = "TYPE"
	colGroup1      = "GROUP_1"
	colGroup2      = "GROUP_2"
	colGroup3      = "GROUP_3"
	colGroup1Order = "GROUP_1_ORDER"
	colGroup2Order = "GROUP_2_ORDER"
	colGroup3Order = "GROUP_3_ORDER"
	colGrain       = "PERIODE_GRAIN"
	colPeriode     = "PERIODE"
	colValue       = "VALUE"
	colUOM         = "UOM"
	colScenario    = "SCENARIO"
)

const maxGroupLen = 100

// RawRow holds the trimmed string cells of one Excel data row, keyed by column.
type RawRow struct {
	Type           string
	Group1         string
	Group2         string
	Group3         string
	Group1Order    string
	Group2Order    string
	Group3Order    string
	Grain          string
	Periode        string
	Value          string
	UOM            string
	Scenario       string
	MetricName     string // optional — defaults to 'VALUE' when absent
	MetricCategory string // optional — defaults to 'VALUE' when absent
	AggMethod      string // optional — defaults to 'SUM' when absent
}

// FieldError describes a single per-cell validation failure.
type FieldError struct {
	Row      int
	Column   string
	Value    string
	Issue    string
	Expected string
}

// ValidateRow validates one raw Excel row against the target type and produces a
// staging row. The staging row is always returned (best-effort); when errs is
// non-empty the row's ValidationStatus is INVALID.
func ValidateRow(raw RawRow, targetType string, rowNumber int) (StagingRow, []FieldError) {
	var errs []FieldError
	row := StagingRow{
		RowNumber:        rowNumber,
		Type:             raw.Type,
		Group1:           raw.Group1,
		Group2:           raw.Group2,
		Group3:           raw.Group3,
		Scenario:         scenarioDefault,
		ValidationStatus: ValidationValid,
	}

	errs = appendErr(errs, validateType(raw, targetType, rowNumber))
	errs = appendErr(errs, validateGroup1(raw, rowNumber))
	errs = append(errs, validateOptionalGroups(raw, rowNumber)...)

	grain, grainErr := validateGrain(raw, rowNumber)
	errs = appendErr(errs, grainErr)
	row.PeriodGrain = grain

	pDate, label, pErr := validatePeriode(raw, grain, rowNumber)
	errs = appendErr(errs, pErr)
	row.PeriodDate = pDate
	row.PeriodLabel = label

	val, vErr := validateValue(raw, rowNumber)
	errs = appendErr(errs, vErr)
	row.Value = val

	row.UOM = raw.UOM
	if raw.Scenario != "" {
		row.Scenario = strings.ToUpper(raw.Scenario)
	}
	row.Group1Order = parseOrder(raw.Group1Order)
	row.Group2Order = parseOrder(raw.Group2Order)
	row.Group3Order = parseOrder(raw.Group3Order)
	row.DisplayValue = ComputeDisplayValue(row.Type, row.Group1, row.Group2, row.Value)
	// v1.1 multi-metric fields — pass through verbatim; defaults applied at Upsert boundary.
	row.MetricName = raw.MetricName
	row.MetricCategory = raw.MetricCategory
	row.AggMethod = raw.AggMethod

	if len(errs) > 0 {
		row.ValidationStatus = ValidationInvalid
		row.ValidationMsg = joinIssues(errs)
	}
	return row, errs
}

// ComputeDisplayValue replicates the SQL bi_compute_display_value sign convention.
func ComputeDisplayValue(factType, group1, group2 string, value float64) float64 {
	if factType != typeMIS {
		return value
	}
	if group1 != groupEBITDA && group1 != groupNetProfit {
		return value
	}
	if group2 == groupIncome || isCostGroup(group2) {
		return -value
	}
	return value
}

func isCostGroup(group2 string) bool {
	if strings.Contains(group2, "COST") || strings.Contains(group2, "CONSUMPTION") {
		return true
	}
	switch group2 {
	case "MANPOWER", "OVERHEADS", "SELLING COST", "BAD DEBT EXP":
		return true
	default:
		return false
	}
}

func validateType(raw RawRow, targetType string, rowNumber int) *FieldError {
	if targetType == "" {
		return nil
	}
	if raw.Type == "" {
		return &FieldError{Row: rowNumber, Column: colType, Value: raw.Type, Issue: "TYPE is required", Expected: targetType}
	}
	if !strings.EqualFold(raw.Type, targetType) {
		return &FieldError{Row: rowNumber, Column: colType, Value: raw.Type, Issue: "TYPE does not match target", Expected: targetType}
	}
	return nil
}

func validateGroup1(raw RawRow, rowNumber int) *FieldError {
	if raw.Group1 == "" {
		return &FieldError{Row: rowNumber, Column: colGroup1, Value: raw.Group1, Issue: "GROUP_1 is required", Expected: "non-empty"}
	}
	if len(raw.Group1) > maxGroupLen {
		return &FieldError{Row: rowNumber, Column: colGroup1, Value: raw.Group1, Issue: "GROUP_1 too long", Expected: "<= 100 chars"}
	}
	return nil
}

func validateOptionalGroups(raw RawRow, rowNumber int) []FieldError {
	var errs []FieldError
	if len(raw.Group2) > maxGroupLen {
		errs = append(errs, FieldError{Row: rowNumber, Column: colGroup2, Value: raw.Group2, Issue: "GROUP_2 too long", Expected: "<= 100 chars"})
	}
	if len(raw.Group3) > maxGroupLen {
		errs = append(errs, FieldError{Row: rowNumber, Column: colGroup3, Value: raw.Group3, Issue: "GROUP_3 too long", Expected: "<= 100 chars"})
	}
	return errs
}

func validateGrain(raw RawRow, rowNumber int) (string, *FieldError) {
	grain := strings.ToUpper(strings.TrimSpace(raw.Grain))
	switch grain {
	case grainDaily, grainMonthly, grainQuarterly, grainYearly:
		return grain, nil
	default:
		return grain, &FieldError{
			Row: rowNumber, Column: colGrain, Value: raw.Grain,
			Issue: "invalid PERIODE_GRAIN", Expected: "DAILY|MONTHLY|QUARTERLY|YEARLY",
		}
	}
}

func validatePeriode(raw RawRow, grain string, rowNumber int) (time.Time, string, *FieldError) {
	v := strings.TrimSpace(raw.Periode)
	if v == "" {
		return time.Time{}, "", &FieldError{Row: rowNumber, Column: colPeriode, Value: raw.Periode, Issue: "PERIODE is required", Expected: "YYYYMM or YYYY-MM-DD"}
	}
	switch grain {
	case grainDaily:
		return parseDaily(v, raw.Periode, rowNumber)
	default:
		return parseMonthly(v, raw.Periode, rowNumber)
	}
}

func parseDaily(v, original string, rowNumber int) (time.Time, string, *FieldError) {
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, "", &FieldError{Row: rowNumber, Column: colPeriode, Value: original, Issue: "invalid daily PERIODE", Expected: "YYYY-MM-DD"}
	}
	return t, t.Format("2006-01-02"), nil
}

func parseMonthly(v, original string, rowNumber int) (time.Time, string, *FieldError) {
	// Accept the PRD template form (YYYYMM) and the source-file forms (YYYY-MM-DD, YYYY-MM);
	// monthly facts are normalized to the first day of the month with a YYYYMM label.
	for _, layout := range []string{"200601", "2006-01-02", "2006-01"} {
		if t, err := time.Parse(layout, v); err == nil {
			first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
			return first, first.Format("200601"), nil
		}
	}
	return time.Time{}, "", &FieldError{Row: rowNumber, Column: colPeriode, Value: original, Issue: "invalid PERIODE", Expected: "YYYYMM or YYYY-MM-DD"}
}

func validateValue(raw RawRow, rowNumber int) (float64, *FieldError) {
	v := strings.TrimSpace(raw.Value)
	if v == "" {
		return 0, &FieldError{Row: rowNumber, Column: colValue, Value: raw.Value, Issue: "VALUE is required", Expected: "numeric"}
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	if err != nil {
		return 0, &FieldError{Row: rowNumber, Column: colValue, Value: raw.Value, Issue: "VALUE is not numeric", Expected: "numeric"}
	}
	return f, nil
}

// parseOrder parses an optional order column; defaults to 1 on empty/invalid input.
func parseOrder(s string) int {
	v := strings.TrimSpace(s)
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 1
	}
	return n
}

// BusinessKey returns the duplicate-detection key for a staging row.
// Must match the unique constraint on bi_fact_metric (v1.1 adds metric_name).
func BusinessKey(r StagingRow) string {
	metricName := r.MetricName
	if metricName == "" {
		metricName = "VALUE"
	}
	return strings.Join([]string{
		r.Type, r.Group1, r.Group2, r.Group3,
		r.PeriodGrain, r.PeriodDate.Format("2006-01-02"), metricName, r.Scenario,
	}, "|")
}

func appendErr(errs []FieldError, e *FieldError) []FieldError {
	if e == nil {
		return errs
	}
	return append(errs, *e)
}

func joinIssues(errs []FieldError) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%s: %s", e.Column, e.Issue))
	}
	return strings.Join(parts, "; ")
}
