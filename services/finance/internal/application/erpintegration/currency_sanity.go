// Package erpintegration is the application layer of the GoApps -> ERP standard
// cost integration.
//
// currency_sanity.go implements T-CUR (design Part 2 §7.3, P0-T10a): a
// read-only report over ACTUAL cst_product_cost rows that shows how the
// cpc_currency_code label is distributed and which rows have IDR-magnitude
// values, plus the V-09 interim acceptance rule. It changes no data.
package erpintegration

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// Currency labels seen on cst_product_cost.cpc_currency_code.
const (
	CurrencyUSD = "USD"
	CurrencyIDR = "IDR"
)

// Outlier thresholds (design §7.3). ACTUAL costs are USD/kg; a legitimate row
// stays well under 100 (the observed maximum outside 202605 is about 92, F-4),
// whereas an IDR-valued row is roughly 15,000 times larger.
var (
	// GuardedPrefixThreshold applies to items whose ERP item code starts with
	// one of GuardedItemPrefixes (mirrors V-07: 0 < std <= 20 USD).
	GuardedPrefixThreshold = decimal.NewFromInt(20)
	// OverallThreshold applies to every row.
	OverallThreshold = decimal.NewFromInt(100)
)

// GuardedItemPrefixes are the ERP item-code prefixes that carry the tighter
// V-07 bound (design Part 2 §7, V-07).
var GuardedItemPrefixes = []string{"POY", "PTY", "ACY", "ITY", "MMK", "TTY", "HOY"}

// DefaultOutlierLimit caps the number of outlier rows returned in one report
// (202605 alone has about 9,700; the total count is always exact).
const DefaultOutlierLimit = 1000

// MaxOutlierLimit is the hard cap on CurrencySanityFilter.OutlierLimit.
const MaxOutlierLimit = 20000

var periodPattern = regexp.MustCompile(`^20[0-9]{2}(0[1-9]|1[0-2])$`)

// ErrInvalidPeriod is returned for a period that is not YYYYMM.
var ErrInvalidPeriod = errors.New("erpintegration: period must be YYYYMM")

// IsGuardedItem reports whether an ERP item code carries the V-07 prefix bound.
func IsGuardedItem(erpItemCode string) bool {
	code := strings.ToUpper(strings.TrimSpace(erpItemCode))
	for _, p := range GuardedItemPrefixes {
		if strings.HasPrefix(code, p) {
			return true
		}
	}
	return false
}

// IsCurrencyOutlier applies the §7.3 rule to one row: value > 20 for a guarded
// prefix, or value > 100 for any row. erpItemCode may be empty (unlinked
// product), in which case only the overall bound applies. The SQL in the
// postgres CurrencySanityRepository uses the same constants.
func IsCurrencyOutlier(erpItemCode string, costPerUnit decimal.Decimal) bool {
	if costPerUnit.GreaterThan(OverallThreshold) {
		return true
	}
	return IsGuardedItem(erpItemCode) && costPerUnit.GreaterThan(GuardedPrefixThreshold)
}

// IsCurrencyAcceptable is the V-09 interim rule (design §7.3), valid until
// migration 000557 has relabelled period P and the writer fix is live:
//
//   - label USD is always accepted;
//   - label IDR is accepted if and only if the row is not an outlier;
//   - any other label is rejected.
//
// The caller applies it to ACTUAL source rows only. Once 000557 is applied for
// a period, V-09 becomes strict USD and this helper is no longer used for it.
// Period 202605 stays an error regardless (F-4/F-10); that is the caller's job.
func IsCurrencyAcceptable(label string, isOutlier bool) bool {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case CurrencyUSD:
		return true
	case CurrencyIDR:
		return !isOutlier
	default:
		return false
	}
}

// CurrencySanityFilter scopes a report.
type CurrencySanityFilter struct {
	// Period is YYYYMM; empty means all periods.
	Period string
	// LinkedOnly restricts to products with a non-null cpm_erp_item_code (the
	// design's "linked products"). The 000557 guard counts all active rows, so
	// the per-period summary is always computed without this restriction.
	LinkedOnly bool
	// OutlierLimit caps the returned outlier rows (0 = DefaultOutlierLimit).
	OutlierLimit int
}

// Normalize validates the filter and fills defaults.
func (f CurrencySanityFilter) Normalize() (CurrencySanityFilter, error) {
	f.Period = strings.TrimSpace(f.Period)
	if f.Period != "" && !periodPattern.MatchString(f.Period) {
		return f, fmt.Errorf("%w: %q", ErrInvalidPeriod, f.Period)
	}
	if f.OutlierLimit <= 0 {
		f.OutlierLimit = DefaultOutlierLimit
	}
	if f.OutlierLimit > MaxOutlierLimit {
		f.OutlierLimit = MaxOutlierLimit
	}
	return f, nil
}

// CurrencyLabelCount is one label-distribution bucket.
type CurrencyLabelCount struct {
	Period   string
	Currency string
	Rows     int64
}

// CurrencyPercentiles summarizes cpc_cost_per_unit for the filtered rows.
type CurrencyPercentiles struct {
	Rows int64
	Min  decimal.NullDecimal
	P50  decimal.NullDecimal
	P90  decimal.NullDecimal
	P99  decimal.NullDecimal
	Max  decimal.NullDecimal
}

// CurrencyOutlier is one active ACTUAL row that fails the §7.3 bound.
type CurrencyOutlier struct {
	CostID       int64
	ProductSysID int64
	ProductCode  string
	ErpItemCode  string
	Period       string
	Currency     string
	Status       string
	CostPerUnit  decimal.Decimal
	// Threshold is the bound the row exceeded (20 for guarded prefixes, else 100).
	Threshold decimal.Decimal
}

// CurrencyPeriodSummary is the per-period input for the 000557 guard: every
// active (non-SUPERSEDED) ACTUAL row, linked or not.
type CurrencyPeriodSummary struct {
	Period         string
	Rows           int64
	ApprovedRows   int64
	IDRRows        int64
	USDRows        int64
	Max            decimal.Decimal
	OverOverall    int64 // rows with cost_per_unit > OverallThreshold (the 000557 guard count)
	OutlierRows    int64 // rows failing the full §7.3 rule (prefix bound included)
	RelabelSafe    bool  // OverOverall == 0
	ExcludedByPlan bool  // period is excluded from 000557 by decision (202605)
}

// CurrencySanityReport is the T-CUR report.
type CurrencySanityReport struct {
	Filter        CurrencySanityFilter
	Labels        []CurrencyLabelCount
	Percentiles   CurrencyPercentiles
	OutlierTotal  int64
	Outliers      []CurrencyOutlier
	Periods       []CurrencyPeriodSummary
	OutlierCapped bool // OutlierTotal > len(Outliers)
}

// CurrencySanityReader is the read-only port implemented by
// postgres.CurrencySanityRepository. Every method is a SELECT.
type CurrencySanityReader interface {
	LabelDistribution(ctx context.Context, f CurrencySanityFilter) ([]CurrencyLabelCount, error)
	Percentiles(ctx context.Context, f CurrencySanityFilter) (CurrencyPercentiles, error)
	ListOutliers(ctx context.Context, f CurrencySanityFilter) (rows []CurrencyOutlier, total int64, err error)
	PeriodSummaries(ctx context.Context, period string) ([]CurrencyPeriodSummary, error)
}

// RelabelExcludedPeriods are excluded from 000557 by User decision 2026-09-29
// U-3 (F-4/F-10: 202605 is a broken early run).
var RelabelExcludedPeriods = map[string]struct{}{"202605": {}}

// CurrencySanityService builds the report.
type CurrencySanityService struct {
	reader CurrencySanityReader
}

// NewCurrencySanityService constructs the service.
func NewCurrencySanityService(reader CurrencySanityReader) *CurrencySanityService {
	return &CurrencySanityService{reader: reader}
}

// Report returns CurrencySanityReport(period) (design §7.3). Read-only.
func (s *CurrencySanityService) Report(ctx context.Context, f CurrencySanityFilter) (*CurrencySanityReport, error) {
	nf, err := f.Normalize()
	if err != nil {
		return nil, err
	}
	labels, err := s.reader.LabelDistribution(ctx, nf)
	if err != nil {
		return nil, fmt.Errorf("currency sanity: label distribution: %w", err)
	}
	pct, err := s.reader.Percentiles(ctx, nf)
	if err != nil {
		return nil, fmt.Errorf("currency sanity: percentiles: %w", err)
	}
	outliers, total, err := s.reader.ListOutliers(ctx, nf)
	if err != nil {
		return nil, fmt.Errorf("currency sanity: outliers: %w", err)
	}
	periods, err := s.reader.PeriodSummaries(ctx, nf.Period)
	if err != nil {
		return nil, fmt.Errorf("currency sanity: period summaries: %w", err)
	}
	for i := range periods {
		periods[i].RelabelSafe = periods[i].OverOverall == 0
		_, periods[i].ExcludedByPlan = RelabelExcludedPeriods[periods[i].Period]
	}
	return &CurrencySanityReport{
		Filter:        nf,
		Labels:        labels,
		Percentiles:   pct,
		OutlierTotal:  total,
		Outliers:      outliers,
		Periods:       periods,
		OutlierCapped: total > int64(len(outliers)),
	}, nil
}
