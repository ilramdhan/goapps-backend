package erpintegration

// recon.go holds the recon read-back model (plan-06 P5-T6; design Part 1 §2
// step 12, §5.3 VALUATED -> RECONCILED, C-4, C-12; PRD §4.10 / recon §4,
// §4a, §5, §7). Recon reads Oracle back with SELECT only (the batch header
// and cost rows GoApps pushed, and the period's ADJ items aggregated per
// key) and compares them with the PG std rows:
//
//   - per OK std row: MATCH (one ADJ rate, ROUND(rate,5) = std and every item
//     stamped FLEX_13 = batch), DIFF (anything else) or NOT_IN_ADJ (the key
//     has no ADJ item);
//   - ADJ keys without an OK std row are NOT_COVERED (a count, C-4);
//   - control totals and md5 of the pushed rows, PG vs Oracle (§4a).
//
// The batch is RECONCILED only when everything is MATCH, NOT_IN_ADJ and
// NOT_COVERED are empty and the totals and md5 are equal.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// ReconStatus is cst_erp_std_cost.cesc_recon_status (C-4).
type ReconStatus string

// Recon statuses (CHECK chk_cesc_recon_status).
const (
	ReconMatch    ReconStatus = "MATCH"
	ReconDiff     ReconStatus = "DIFF"
	ReconNotInAdj ReconStatus = "NOT_IN_ADJ"
)

// reconStoreScale is the scale of cesc_erp_rate / _qty_kg / _value.
const reconStoreScale int32 = 7

// reconFlexMax is the width of cesc_erp_flex13.
const reconFlexMax = 20

// AdjReadBackCombo is the read-back of the period's ADJ items of one key
// (recon §4 / PRD §4.10): item count, COUNT(DISTINCT rate), MAX(rate),
// SUM(qty_bu)/1000, SUM(val), MAX(FLEX_13) and the number of items whose
// FLEX_13 equals the batch id. NULL aggregates are invalid NullDecimals.
type AdjReadBackCombo struct {
	Key          ErpKey
	Items        int64
	RateVariants int64
	MaxRate      decimal.NullDecimal
	QtyKg        decimal.NullDecimal
	Value        decimal.NullDecimal
	Flex13       string
	Stamped      int64
}

// MergeAdjReadBack folds combos whose trimmed keys coincide (the reader
// groups by the raw codes) and returns them ordered by key.
func MergeAdjReadBack(in []AdjReadBackCombo) []AdjReadBackCombo {
	byKey := make(map[ErpKey]*AdjReadBackCombo, len(in))
	order := make([]ErpKey, 0, len(in))
	for _, c := range in {
		k := ErpKey{
			ItemCode:  strings.TrimSpace(c.Key.ItemCode),
			GradeCode: strings.TrimSpace(c.Key.GradeCode),
			ShadeCode: strings.TrimSpace(c.Key.ShadeCode),
		}
		cur, ok := byKey[k]
		if !ok {
			cp := c
			cp.Key = k
			byKey[k] = &cp
			order = append(order, k)
			continue
		}
		cur.Items += c.Items
		cur.Stamped += c.Stamped
		// Distinct rates across the merged groups: different maxima prove
		// at least two rates; equal maxima keep the larger variant count.
		switch {
		case cur.MaxRate.Valid && c.MaxRate.Valid && !cur.MaxRate.Decimal.Equal(c.MaxRate.Decimal):
			cur.RateVariants = max(cur.RateVariants+c.RateVariants, 2)
		default:
			cur.RateVariants = max(cur.RateVariants, c.RateVariants)
		}
		cur.MaxRate = maxNull(cur.MaxRate, c.MaxRate)
		cur.QtyKg = addNull(cur.QtyKg, c.QtyKg)
		cur.Value = addNull(cur.Value, c.Value)
		if c.Flex13 > cur.Flex13 {
			cur.Flex13 = c.Flex13
		}
	}
	sort.Slice(order, func(i, j int) bool { return lessErpKey(order[i], order[j]) })
	out := make([]AdjReadBackCombo, len(order))
	for i, k := range order {
		out[i] = *byKey[k]
	}
	return out
}

func maxNull(a, b decimal.NullDecimal) decimal.NullDecimal {
	switch {
	case !a.Valid:
		return b
	case !b.Valid:
		return a
	case b.Decimal.GreaterThan(a.Decimal):
		return b
	default:
		return a
	}
}

func addNull(a, b decimal.NullDecimal) decimal.NullDecimal {
	switch {
	case !a.Valid:
		return b
	case !b.Valid:
		return a
	default:
		return decimal.NewNullDecimal(a.Decimal.Add(b.Decimal))
	}
}

// OracleBatchReadBack is the read-back of CST_GOAPPS_STD_BATCH (header) and
// CST_GOAPPS_STD_COST (rows) for one batch. Found is false when Oracle has
// no header row for the batch id. CostRows carry the digest columns only,
// with Status DeriveOK (every pushed row is an OK row).
type OracleBatchReadBack struct {
	Found    bool
	BatchID  int64
	Period   string
	Seq      int
	Status   string // GSB_STATUS
	RuleHash string
	Header   ControlTotals // GSB_ROW_COUNT / GSB_SUM_*
	CostRows []StdRow
}

// Oracle GSB_STATUS values (design §5.3 S-4; PRD §3.1).
const (
	GsbPushed     = "PUSHED"
	GsbValuated   = "VALUATED"
	GsbApproved   = "APPROVED"
	GsbLocked     = "LOCKED"
	GsbFailed     = "FAILED"
	GsbSuperseded = "SUPERSEDED"
)

// ErpReconReader is the read-only recon port (SELECT only through the
// ReadOnlyGuard querier; design Part 2 §9.1 ErpAdjReader recon read-back).
type ErpReconReader interface {
	// ReadBackBatch reads the pushed header and cost rows of batchID.
	ReadBackBatch(ctx context.Context, batchID int64) (OracleBatchReadBack, error)
	// ReadBackAdj aggregates the period's INVADJ / MBINVADJ / MBINVADJRP
	// items per key; Stamped counts the items with FLEX_13 = batchID.
	ReadBackAdj(ctx context.Context, period string, batchID int64) ([]AdjReadBackCombo, error)
}

// ReconRow is the recon outcome of one OK std row (cesc_erp_* columns).
type ReconRow struct {
	Key          ErpKey
	Status       ReconStatus
	ErpRate      decimal.NullDecimal
	RateVariants *int64
	QtyKg        decimal.NullDecimal
	Value        decimal.NullDecimal
	Flex13       string
}

// ReconCounts are the per-status counts of a recon run.
type ReconCounts struct {
	Match            int   `json:"match"`
	Diff             int   `json:"diff"`
	NotInAdj         int   `json:"not_in_adj"`
	NotCoveredCombos int   `json:"not_covered_combos"`
	NotCoveredItems  int64 `json:"not_covered_items"`
}

// ReconClassification is the row-level part of a recon run.
type ReconClassification struct {
	Rows       []ReconRow
	Counts     ReconCounts
	NotCovered []ErpKey // ADJ keys without an OK std row (recon §7)
}

// AllMatch reports MATCH for every OK row with no NOT_IN_ADJ and no
// NOT_COVERED ADJ item.
func (c ReconClassification) AllMatch() bool {
	return c.Counts.Diff == 0 && c.Counts.NotInAdj == 0 && c.Counts.NotCoveredItems == 0 && c.Counts.NotCoveredCombos == 0
}

// ClassifyRecon compares the OK std rows with the ADJ read-back (C-12:
// ROUND(max_rate,5) = std). Non-OK std rows are not classified.
func ClassifyRecon(std []StdRow, adj []AdjReadBackCombo, batchID int64) ReconClassification {
	combos := make(map[ErpKey]AdjReadBackCombo, len(adj))
	for _, c := range MergeAdjReadBack(adj) {
		combos[c.Key] = c
	}
	var out ReconClassification
	covered := make(map[ErpKey]struct{}, len(std))
	ok := make([]StdRow, 0, len(std))
	for _, r := range std {
		if r.Status == DeriveOK {
			ok = append(ok, r)
		}
	}
	sort.Slice(ok, func(i, j int) bool { return lessErpKey(ok[i].Key, ok[j].Key) })
	batch := strconv.FormatInt(batchID, 10)
	for _, r := range ok {
		covered[r.Key] = struct{}{}
		c, found := combos[r.Key]
		if !found {
			out.Rows = append(out.Rows, ReconRow{Key: r.Key, Status: ReconNotInAdj})
			out.Counts.NotInAdj++
			continue
		}
		row := reconRowFrom(r.Key, c)
		if comboMatches(r, c, batch) {
			row.Status = ReconMatch
			out.Counts.Match++
		} else {
			row.Status = ReconDiff
			out.Counts.Diff++
		}
		out.Rows = append(out.Rows, row)
	}
	for _, c := range MergeAdjReadBack(adj) {
		if _, isStd := covered[c.Key]; isStd || c.Items <= 0 {
			continue
		}
		out.NotCovered = append(out.NotCovered, c.Key)
		out.Counts.NotCoveredCombos++
		out.Counts.NotCoveredItems += c.Items
	}
	return out
}

func comboMatches(r StdRow, c AdjReadBackCombo, batch string) bool {
	return r.StdCost.Valid && c.Items > 0 && c.RateVariants == 1 && c.MaxRate.Valid &&
		Round5(c.MaxRate.Decimal).Equal(Round5(r.StdCost.Decimal)) &&
		c.Stamped == c.Items && strings.TrimSpace(c.Flex13) == batch
}

func reconRowFrom(k ErpKey, c AdjReadBackCombo) ReconRow {
	v := c.RateVariants
	flex := strings.TrimSpace(c.Flex13)
	if r := []rune(flex); len(r) > reconFlexMax {
		flex = string(r[:reconFlexMax])
	}
	return ReconRow{
		Key: k, ErpRate: round7(c.MaxRate), RateVariants: &v,
		QtyKg: round7(c.QtyKg), Value: round7(c.Value), Flex13: flex,
	}
}

// round7 rounds to the NUMERIC(…,7) scale of the cesc_erp_* columns (half
// away from zero, as PG NUMERIC casting does).
func round7(d decimal.NullDecimal) decimal.NullDecimal {
	if !d.Valid {
		return d
	}
	return decimal.NewNullDecimal(d.Decimal.Round(reconStoreScale))
}

// ReconStore is the transaction-scoped store of the recon step. The PG
// BatchTxRunner hands a BatchStore that also implements it.
type ReconStore interface {
	BatchStore
	// ListStdRows returns every std row of the locked batch ordered by key.
	ListStdRows(ctx context.Context) ([]StdRow, error)
	// ApplyRecon clears the recon columns of every std row of the locked
	// batch and writes rows (matched by key) in the same transaction. It
	// returns the number of rows written.
	ApplyRecon(ctx context.Context, rows []ReconRow) (int64, error)
}

// ExpectedGsbStatusFor reports whether a GSB_STATUS proves that the call
// keyed by statementKey committed (done) or did not commit (notDone). When
// neither holds the outcome stays unresolved.
func ExpectedGsbStatusFor(statementKey, gsbStatus string) (done, notDone bool) {
	in := func(vs ...string) bool {
		for _, v := range vs {
			if gsbStatus == v {
				return true
			}
		}
		return false
	}
	switch statementKey {
	case CallKeyW2ValuateAdj:
		return in(GsbValuated, GsbApproved, GsbLocked), in(GsbPushed)
	case CallKeyW2ApproveAdj:
		return in(GsbApproved, GsbLocked), in(GsbValuated)
	case CallKeyW2LockBatch:
		return in(GsbLocked), in(GsbApproved, GsbValuated)
	case CallKeyW2RestoreAdj:
		return in(GsbFailed), in(GsbValuated, GsbApproved)
	default:
		return false, false
	}
}

// String renders a short form for messages.
func (r OracleBatchReadBack) String() string {
	if !r.Found {
		return fmt.Sprintf("batch %d: not in Oracle", r.BatchID)
	}
	return fmt.Sprintf("batch %d: %s %s rows=%d", r.BatchID, r.Period, r.Status, len(r.CostRows))
}

// OracleCallRecord is one unresolved (STARTED / UNKNOWN) call-log row.
type OracleCallRecord struct {
	CallID       string
	StatementKey string
	Status       OracleCallStatus
	Params       json.RawMessage
}

// OracleCallResolver settles unresolved call-log rows from read-only
// evidence (design Part 3 §14 "Unknown outcome": never a blind re-execute).
// It only ever rewrites the PG call log; no Oracle call is made.
type OracleCallResolver interface {
	// ListUnresolved returns the batch's STARTED / UNKNOWN rows.
	ListUnresolved(ctx context.Context, batchID int64) ([]OracleCallRecord, error)
	// Resolve moves one STARTED / UNKNOWN row to SUCCESS or FAILED and
	// merges note into ceocl_summary. A row that is no longer unresolved
	// is an error (ErrCallAlreadyResolved).
	Resolve(ctx context.Context, callID string, to OracleCallStatus, note json.RawMessage) error
}

// ErrCallAlreadyResolved is returned by Resolve when the row is no longer
// STARTED / UNKNOWN (or does not exist).
var ErrCallAlreadyResolved = errors.New("erpintegration: oracle call already resolved")

// ReconExportRow is a persisted recon outcome joined with the GoApps std
// cost, as read back for the recon export.
type ReconExportRow struct {
	ReconRow
	Basis   string
	StdCost decimal.NullDecimal
}
