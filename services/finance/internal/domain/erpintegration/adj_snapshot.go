package erpintegration

// adj_snapshot.go holds the valuation preview and PG snapshot model (plan-06
// P5-T4; design §3.2 G4-G7a, §4.7, §9.4). The snapshot is the immutable PG
// backup of every OT_ADJ_ITEM row the period-wide VALUATE_ADJ would touch,
// read with SELECT only and committed before any writer call (G7). The row
// hash covers the columns the package changes plus the head eligibility, so
// the executor's G6 re-probe detects any change since the preview.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// AdjFlexCount is the number of ADJI_FLEX_* columns VALUATE_ADJ writes.
const AdjFlexCount = 14

// adjApprovedStatus is ADJH_APPR_STATUS of an approved head.
const adjApprovedStatus = 3

// adjValScale is the ADJI_VAL rounding of VALUATE_ADJ (ROUND(x, 7)).
const adjValScale = 7

// AdjFlex is the 14 FLEX text columns of one item; nil is NULL.
type AdjFlex [AdjFlexCount]*string

// AdjSnapshotRow is one OT_ADJ_ITEM row (with its head status) as read by
// the read-only snapshot reader, plus the values the valuation would write.
// HeadApprStatus / HeadPostStatus nil means NULL.
type AdjSnapshotRow struct {
	HeadSysID      int64
	ItemSysID      int64
	TxnCode        string
	HeadApprStatus *int64
	HeadPostStatus *string
	ItemCode       string
	GradeCode      string
	ShadeCode      string
	QtyBu          decimal.NullDecimal
	ItemDesc       string
	Rate           decimal.NullDecimal
	Val            decimal.NullDecimal
	Flex           AdjFlex

	// Projected values (set by ProjectAdjRow when a valued std row matches).
	NewRate decimal.NullDecimal
	NewVal  decimal.NullDecimal
	NewFlex *[AdjFlexCount]string
}

// Key returns the std row key (trimmed, as validation.AdjSetItem.Key).
func (r AdjSnapshotRow) Key() ErpKey {
	return ErpKey{
		ItemCode:  strings.TrimSpace(r.ItemCode),
		GradeCode: strings.TrimSpace(r.GradeCode),
		ShadeCode: strings.TrimSpace(r.ShadeCode),
	}
}

// IsPosted reports whether the head is posted (ADJH_POST_STATUS IS NOT NULL).
func (r AdjSnapshotRow) IsPosted() bool { return r.HeadPostStatus != nil }

// IsApproved reports whether the head is approved (ADJH_APPR_STATUS = 3).
func (r AdjSnapshotRow) IsApproved() bool {
	return r.HeadApprStatus != nil && *r.HeadApprStatus == adjApprovedStatus
}

// IsGuardedTxn reports whether the txn code is one of the valuated ADJ txns.
func (r AdjSnapshotRow) IsGuardedTxn() bool {
	switch strings.TrimSpace(r.TxnCode) {
	case TxnInvAdj, TxnMbInvAdj, TxnMbInvAdjRp:
		return true
	}
	return false
}

// Eligible is the §S-R7 predicate: guarded txn, NVL(APPR,0) != 3 and POST
// IS NULL. The reader filters the same way; this is defense in depth.
func (r AdjSnapshotRow) Eligible() bool {
	return r.IsGuardedTxn() && !r.IsApproved() && !r.IsPosted()
}

// IsProjected reports whether a std row matched (the valuation changes it).
func (r AdjSnapshotRow) IsProjected() bool { return r.NewRate.Valid }

// hashNull is the field marker of a NULL value in the row hash.
const hashNull = "\x00"

func hashDec(d decimal.NullDecimal) string {
	if !d.Valid {
		return hashNull
	}
	return d.Decimal.String()
}

func hashStrPtr(s *string) string {
	if s == nil {
		return hashNull
	}
	return *s
}

func hashIntPtr(v *int64) string {
	if v == nil {
		return hashNull
	}
	return strconv.FormatInt(*v, 10)
}

// RowHash is the SHA-256 (hex) over the canonical encoding of the current
// (pre-valuation) columns: ids, txn, head status, key, qty, desc, rate, val
// and FLEX_01..14. Decimals are canonical (trailing zeros dropped), so the
// hash does not depend on the TO_CHAR rendering of the same value.
func (r AdjSnapshotRow) RowHash() string {
	fields := make([]string, 0, 12+AdjFlexCount)
	fields = append(fields,
		strconv.FormatInt(r.HeadSysID, 10), strconv.FormatInt(r.ItemSysID, 10),
		strings.TrimSpace(r.TxnCode), hashIntPtr(r.HeadApprStatus), hashStrPtr(r.HeadPostStatus),
		r.ItemCode, r.GradeCode, r.ShadeCode, hashDec(r.QtyBu), r.ItemDesc,
		hashDec(r.Rate), hashDec(r.Val),
	)
	for _, f := range r.Flex {
		fields = append(fields, hashStrPtr(f))
	}
	sum := sha256.Sum256([]byte(strings.Join(fields, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// AdjSetHash is the confirm_set_hash of a snapshot set: SHA-256 (hex) over
// the rows sorted by ADJI_SYS_ID, each contributing "head:item:row_hash".
// The executor recomputes it from its G6 re-probe (design §9.4).
func AdjSetHash(rows []AdjSnapshotRow) string {
	type ent struct {
		head, item int64
		hash       string
	}
	es := make([]ent, len(rows))
	for i, r := range rows {
		es[i] = ent{r.HeadSysID, r.ItemSysID, r.RowHash()}
	}
	sort.Slice(es, func(i, j int) bool { return es[i].item < es[j].item })
	var b strings.Builder
	for _, e := range es {
		b.WriteString(strconv.FormatInt(e.head, 10))
		b.WriteByte(':')
		b.WriteString(strconv.FormatInt(e.item, 10))
		b.WriteByte(':')
		b.WriteString(e.hash)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// AdjExclusions counts the heads and items of the period the valuation must
// not touch (deny list), and eligible items without a valued std row.
type AdjExclusions struct {
	PostedHeads    int `json:"posted_heads"`
	PostedItems    int `json:"posted_items"`
	ApprovedHeads  int `json:"approved_heads"`
	ApprovedItems  int `json:"approved_items"`
	OtherTxnItems  int `json:"other_txn_items"`
	UnmatchedItems int `json:"unmatched_items"`
}

// SplitAdjSnapshot separates the eligible rows (§S-R7) from the deny list.
// A posted head counts as posted even when also approved.
func SplitAdjSnapshot(rows []AdjSnapshotRow) ([]AdjSnapshotRow, AdjExclusions) {
	var ex AdjExclusions
	posted, approved := map[int64]struct{}{}, map[int64]struct{}{}
	eligible := make([]AdjSnapshotRow, 0, len(rows))
	for _, r := range rows {
		switch {
		case !r.IsGuardedTxn():
			ex.OtherTxnItems++
		case r.IsPosted():
			ex.PostedItems++
			posted[r.HeadSysID] = struct{}{}
		case r.IsApproved():
			ex.ApprovedItems++
			approved[r.HeadSysID] = struct{}{}
		default:
			eligible = append(eligible, r)
		}
	}
	ex.PostedHeads, ex.ApprovedHeads = len(posted), len(approved)
	return eligible, ex
}

// StdRowsByKey indexes the valued (DeriveOK, std set) rows by key, the same
// selection as validation.ProjectRates.
func StdRowsByKey(rows []StdRow) map[ErpKey]StdRow {
	out := make(map[ErpKey]StdRow, len(rows))
	for _, r := range rows {
		if r.Status == DeriveOK && r.StdCost.Valid {
			out[r.Key] = r
		}
	}
	return out
}

// ProjectAdjRow fills the values VALUATE_ADJ writes from the matching std
// row: RATE = std, VAL = ROUND(qty_bu/1000*std, 7) and FLEX_01..14 (numeric
// flex via FormatFlexNull, text flex as-is, FLEX_13 = batch id, FLEX_14 =
// source). The row is returned unchanged when std is nil (not valued).
func ProjectAdjRow(r AdjSnapshotRow, std *StdRow, batchID int64) (AdjSnapshotRow, error) {
	if std == nil || std.Status != DeriveOK || !std.StdCost.Valid {
		return r, nil
	}
	r.NewRate = std.StdCost
	if r.QtyBu.Valid {
		v := r.QtyBu.Decimal.Div(decimal.NewFromInt(1000)).Mul(std.StdCost.Decimal).Round(adjValScale)
		r.NewVal = decimal.NewNullDecimal(v)
	}
	nums := map[int]decimal.NullDecimal{
		0: std.ConvCost, 1: std.ChpConKg, 2: std.ChpCost, 6: std.SellingPrice,
		7: std.AxCost, 8: std.AxConvCost, 9: std.ValueLoss, 10: std.ProdValLoss,
	}
	var flex [AdjFlexCount]string
	for i, v := range nums {
		t, err := FormatFlexNull(v)
		if err != nil {
			return r, fmt.Errorf("item %d FLEX_%02d: %w", r.ItemSysID, i+1, err)
		}
		flex[i] = t.String()
	}
	flex[3] = std.ChpItemCode
	flex[4] = std.FgType
	flex[5] = string(std.Basis)
	flex[11] = std.MsBatchItem
	flex[12] = strconv.FormatInt(batchID, 10)
	flex[13] = string(std.Source)
	r.NewFlex = &flex
	return r, nil
}

// AdjOperation is cst_erp_valuation_preview.cevp_operation.
type AdjOperation string

// ADJ operations.
const (
	AdjOpValuate AdjOperation = "VALUATE"
	AdjOpApprove AdjOperation = "APPROVE"
	AdjOpRestore AdjOperation = "RESTORE"
)

// PreviewStatus is cst_erp_valuation_preview.cevp_status.
type PreviewStatus string

// Preview statuses (design §4.7).
const (
	PreviewBuilding PreviewStatus = "BUILDING"
	PreviewOpen     PreviewStatus = "OPEN"
	PreviewConsumed PreviewStatus = "CONSUMED"
	PreviewExpired  PreviewStatus = "EXPIRED"
	PreviewStale    PreviewStatus = "STALE"
	PreviewFailed   PreviewStatus = "FAILED"
)

// ErrPreviewNotFound is returned when no preview has the id.
var ErrPreviewNotFound = errors.New("erpintegration: preview not found")

// ValuationPreview is one cst_erp_valuation_preview row. Totals is the
// cevp_totals JSON (Σ, per-head counts and the G7a V-07 result); Excluded is
// cevp_excluded.
type ValuationPreview struct {
	ID          string
	BatchID     int64
	Operation   AdjOperation
	Status      PreviewStatus
	HeadCount   int
	ItemCount   int
	Excluded    AdjExclusions
	SetHash     string
	Totals      []byte
	ConfirmText string
	CreatedBy   string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	ConsumedBy  string
}

// PreviewConfirmText is the G5 typed confirmation "<period>/<batch_id>".
func PreviewConfirmText(period string, batchID int64) string {
	return period + "/" + strconv.FormatInt(batchID, 10)
}

// AdjSnapshotReader reads every OT_ADJ_ITEM row of the period's guarded ADJ
// heads with the head status (SELECT only, read-only user; no FOR UPDATE).
type AdjSnapshotReader interface {
	SnapshotAdjRows(ctx context.Context, period string) ([]AdjSnapshotRow, error)
}

// ValuationPreviewRepository persists previews and their snapshot.
type ValuationPreviewRepository interface {
	// CreateOpen, in one PG transaction: checks the batch is still at
	// expected (else ErrStaleBatchStatus), marks any live preview of the same
	// (batch, operation) STALE, inserts p as BUILDING, inserts every snapshot
	// row, then sets it OPEN. It returns the stored preview (id set). A
	// concurrent live preview returns ErrConcurrentRun.
	CreateOpen(ctx context.Context, p ValuationPreview, period string, expected BatchStatus, rows []AdjSnapshotRow) (ValuationPreview, error)
	// CreateFailed inserts p with status FAILED and no snapshot (the G7a
	// refusal record). Live previews are untouched.
	CreateFailed(ctx context.Context, p ValuationPreview) (ValuationPreview, error)
	// Get returns the preview, or ErrPreviewNotFound.
	Get(ctx context.Context, id string) (ValuationPreview, error)
}
