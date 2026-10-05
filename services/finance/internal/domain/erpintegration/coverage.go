package erpintegration

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// CoverageStatus is the coverage outcome of one ERP (item, shade) combo
// (cst_erp_coverage.cec_status; design §4.2, C-5). The values are exactly the
// chk_cec_status set of migration 000549.
//
// Plan-04 P3-T4 names two of them differently: MULTI_MAPPING is stored as
// DUP_MAPPING (V-04 ambiguous, D-LINK) and CURRENCY as NOT_USD (V-09).
type CoverageStatus string

// Coverage statuses.
const (
	CoverageOK          CoverageStatus = "OK"
	CoverageNoMapping   CoverageStatus = "NO_MAPPING"
	CoverageDupMapping  CoverageStatus = "DUP_MAPPING"
	CoverageNoCost      CoverageStatus = "NO_COST"
	CoverageNotApproved CoverageStatus = "NOT_APPROVED"
	CoverageNotUSD      CoverageStatus = "NOT_USD"
	CoverageInvalid     CoverageStatus = "INVALID"
)

var allCoverageStatuses = []CoverageStatus{
	CoverageOK, CoverageNoMapping, CoverageDupMapping, CoverageNoCost,
	CoverageNotApproved, CoverageNotUSD, CoverageInvalid,
}

// AllCoverageStatuses returns every coverage status (a copy).
func AllCoverageStatuses() []CoverageStatus {
	out := make([]CoverageStatus, len(allCoverageStatuses))
	copy(out, allCoverageStatuses)
	return out
}

// ErrInvalidCoverageLine is returned for a coverage line that breaks an
// invariant.
var ErrInvalidCoverageLine = errors.New("erpintegration: invalid coverage line")

// ErrCoverageLineNotFound is returned when a coverage row id is unknown.
var ErrCoverageLineNotFound = errors.New("erpintegration: coverage line not found")

// ParseCoverageStatus parses an exact stored value.
func ParseCoverageStatus(s string) (CoverageStatus, error) {
	for _, v := range allCoverageStatuses {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: status %q", ErrInvalidCoverageLine, s)
}

// String returns the stored value.
func (s CoverageStatus) String() string { return string(s) }

// IsBlocking reports whether the status blocks COVERED (every non-OK status).
func (s CoverageStatus) IsBlocking() bool { return s != CoverageOK }

// CoverageLine is the coverage of one ERP (item, shade) combo of a batch
// (cst_erp_coverage). OK implies the product and cost ids are set.
// Candidates is the raw §9d link-suggestion JSON (nil = SQL NULL).
type CoverageLine struct {
	ID           int64 // cec_id (0 before insert)
	BatchID      int64
	Kind         ItemKind
	ItemCode     string
	ShadeCode    string
	GradeCodes   []string
	ProductSysID *int64
	CostID       *int64
	CostVersion  *int32
	Status       CoverageStatus
	Reason       string
	QtyKg        decimal.Decimal
	Candidates   []byte
}

// Validate checks the invariants and column widths of the line.
func (l CoverageLine) Validate() error {
	if _, err := ParseItemKind(string(l.Kind)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCoverageLine, err)
	}
	if _, err := ParseCoverageStatus(string(l.Status)); err != nil {
		return err
	}
	item := strings.TrimSpace(l.ItemCode)
	if item == "" || len([]rune(item)) > maxItemCodeLen {
		return fmt.Errorf("%w: item code %q", ErrInvalidCoverageLine, l.ItemCode)
	}
	if len([]rune(l.ShadeCode)) > maxGradeCodeLen {
		return fmt.Errorf("%w: shade code longer than %d", ErrInvalidCoverageLine, maxGradeCodeLen)
	}
	if l.QtyKg.IsNegative() {
		return fmt.Errorf("%w: negative qty for %s/%s", ErrInvalidCoverageLine, l.ItemCode, l.ShadeCode)
	}
	if l.Status == CoverageOK && (l.ProductSysID == nil || l.CostID == nil) {
		return fmt.Errorf("%w: OK requires product and cost ids (%s/%s)", ErrInvalidCoverageLine, l.ItemCode, l.ShadeCode)
	}
	return nil
}

// CoverageKey is the natural key of a coverage line inside a batch (uk_cec).
type CoverageKey struct {
	ItemCode  string
	ShadeCode string
}

// Key returns the uk_cec key of the line.
func (l CoverageLine) Key() CoverageKey {
	return CoverageKey{ItemCode: l.ItemCode, ShadeCode: l.ShadeCode}
}

// CoverageCounts is the per-status row count of a batch's coverage.
type CoverageCounts map[CoverageStatus]int64

// Total is the number of coverage rows.
func (c CoverageCounts) Total() int64 {
	var n int64
	for _, v := range c {
		n += v
	}
	return n
}

// AllOK reports whether there is at least one row and every row is OK, the
// condition for COVERED (plan-04 P3-T4).
func (c CoverageCounts) AllOK() bool {
	t := c.Total()
	return t > 0 && c[CoverageOK] == t
}
