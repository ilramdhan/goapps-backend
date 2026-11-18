package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBSupersedeRepository implements mbspin.SupersedeReader (mb-spin-fg-poy-link P3,
// design §2.3 / §2.5). Its building blocks (findSupersedeCandidatesOn,
// listSupersedeParamRowsOn, supersede*WillDeactivateOn) take a supersedeQuerier so the
// Validate supersede path can reuse them inside its own transaction.
type MBSupersedeRepository struct{ db *DB }

// NewMBSupersedeRepository creates an MBSupersedeRepository.
func NewMBSupersedeRepository(db *DB) *MBSupersedeRepository { return &MBSupersedeRepository{db: db} }

var _ mbspin.SupersedeReader = (*MBSupersedeRepository)(nil)

// supersedeQuerier is the subset of *DB / *sql.Tx the supersede reads need.
type supersedeQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// supersedeSpin is one conflicting live spin of another head (a supersede candidate).
type supersedeSpin struct {
	SpinID        uuid.UUID
	SpinCode      string  // mbsource.SpinCode semantics: ORION code, else MB costing, else id
	OrionItemCode *string // raw mbs_orion_item_code
	ShadeCode     string
	FgProductID   int64
	FgProductCode string
	HeadID        uuid.UUID
	HeadCode      string // mbh_mb_costing
	HeadIsActive  bool
	MBProductID   int64 // old head's mbh_cost_product_id, 0 when none
}

// supersedeCandidatesQuery: live + active + not-superseded spins with the same
// UPPER(TRIM(shade)) and FG on a DIFFERENT head (the uix_mbs_shade_fg_live identity).
const supersedeCandidatesQuery = `
	SELECT s.mbs_id,
	       COALESCE(NULLIF(s.mbs_orion_item_code, ''), NULLIF(s.mbs_mb_costing, ''), CAST(s.mbs_id AS TEXT)),
	       s.mbs_orion_item_code, COALESCE(s.mbs_shade_code, ''), s.mbs_fg_product_id,
	       COALESCE(fg.cpm_product_code, ''),
	       h.mbh_id, h.mbh_mb_costing, h.mbh_is_active, COALESCE(h.mbh_cost_product_id, 0)
	FROM mst_mb_spin s
	JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
	LEFT JOIN cost_product_master fg ON fg.cpm_product_sys_id = s.mbs_fg_product_id
	WHERE s.deleted_at IS NULL
	  AND s.mbs_is_active
	  AND s.mbs_superseded_by_spin_id IS NULL
	  AND s.mbs_fg_product_id = $1
	  AND UPPER(TRIM(s.mbs_shade_code)) = UPPER(TRIM($2))
	  AND s.mbs_mbh_id <> $3
	ORDER BY s.created_at, s.mbs_id`

// findSupersedeCandidatesOn returns the spins a new root spin of newHeadID with
// (shade, fgProductID) would supersede. forUpdate locks the candidate spin rows
// (FOR UPDATE OF s) and must only be used inside a transaction. A blank shade or a
// non-positive FG never conflicts (legacy rows without FG are not checked).
func findSupersedeCandidatesOn(ctx context.Context, q supersedeQuerier, newHeadID uuid.UUID, shade string, fgProductID int64, forUpdate bool) ([]supersedeSpin, error) {
	if fgProductID <= 0 || strings.TrimSpace(shade) == "" {
		return nil, nil
	}
	query := supersedeCandidatesQuery
	if forUpdate {
		query += " FOR UPDATE OF s"
	}
	return queryCandidates(ctx, q, "find supersede candidates", query, fgProductID, shade, newHeadID)
}

// supersedeCandidatesBySpinQuery (K5): the same columns as supersedeCandidatesQuery
// but selected by the OLD spin id, with COALESCE(s.mbs_fg_product_id, 0) because
// legacy spins have a NULL FG (supersedeSpin.FgProductID is int64).
const supersedeCandidatesBySpinQuery = `
	SELECT s.mbs_id,
	       COALESCE(NULLIF(s.mbs_orion_item_code, ''), NULLIF(s.mbs_mb_costing, ''), CAST(s.mbs_id AS TEXT)),
	       s.mbs_orion_item_code, COALESCE(s.mbs_shade_code, ''), COALESCE(s.mbs_fg_product_id, 0),
	       COALESCE(fg.cpm_product_code, ''),
	       h.mbh_id, h.mbh_mb_costing, h.mbh_is_active, COALESCE(h.mbh_cost_product_id, 0)
	FROM mst_mb_spin s
	JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
	LEFT JOIN cost_product_master fg ON fg.cpm_product_sys_id = s.mbs_fg_product_id
	WHERE s.deleted_at IS NULL
	  AND s.mbs_is_active
	  AND s.mbs_superseded_by_spin_id IS NULL
	  AND s.mbs_id = $1
	ORDER BY s.created_at, s.mbs_id`

// findSupersedeCandidatesBySpinOn returns the single live, active, not-superseded
// spin oldSpinID as a supersede candidate (empty when it is already superseded,
// inactive or deleted). It does not require an FG or shade. Locks the row
// (FOR UPDATE OF s) and must only be used inside a transaction.
func findSupersedeCandidatesBySpinOn(ctx context.Context, q supersedeQuerier, oldSpinID uuid.UUID) ([]supersedeSpin, error) {
	return queryCandidates(ctx, q, "find supersede candidate by spin", supersedeCandidatesBySpinQuery+" FOR UPDATE OF s", oldSpinID)
}

func queryCandidates(ctx context.Context, q supersedeQuerier, what, query string, args ...any) ([]supersedeSpin, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer closeRows(rows)
	var out []supersedeSpin
	for rows.Next() {
		var (
			c     supersedeSpin
			orion sql.NullString
		)
		if err := rows.Scan(&c.SpinID, &c.SpinCode, &orion, &c.ShadeCode, &c.FgProductID, &c.FgProductCode,
			&c.HeadID, &c.HeadCode, &c.HeadIsActive, &c.MBProductID); err != nil {
			return nil, fmt.Errorf("scan supersede candidate: %w", err)
		}
		c.OrionItemCode = nullableStringPtr(orion)
		out = append(out, c)
	}
	return out, rows.Err()
}

// supersedeParamRow is one MB_SP_CODE cost_product_parameter row that points at a
// candidate spin, by pointer (POINTER) or by its ORION code text (TEXT_ONLY).
type supersedeParamRow struct {
	CppValueID   int64
	ProductSysID int64
	ProductCode  string
	ProductName  string
	ValueText    *string
	FilledBy     string
	IsLocked     bool
	Kind         string // mbspin.SupersedeRepointPointer | mbspin.SupersedeRepointTextOnly
}

const supersedeParamRowCols = `
	SELECT c.cpp_value_id, c.cpp_product_sys_id, m.cpm_product_code, m.cpm_product_name,
	       c.cpp_value_text, c.cpp_filled_by, m.cpm_is_locked
	FROM cost_product_parameter c
	JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = $1 AND p.deleted_at IS NULL
	JOIN cost_product_master m ON m.cpm_product_sys_id = c.cpp_product_sys_id`

// supersedePointerRowsQuery: rows whose permanent pointer is the old spin (D7: ALL filled_by).
const supersedePointerRowsQuery = supersedeParamRowCols + `
	WHERE c.cpp_value_mb_spin_id = $2
	ORDER BY m.cpm_product_code, c.cpp_value_id`

// supersedeTextRowsQuery: pointer-less rows whose text is the old spin's code (Revisi 2).
// Blank text never matches.
const supersedeTextRowsQuery = supersedeParamRowCols + `
	WHERE c.cpp_value_mb_spin_id IS NULL
	  AND TRIM(COALESCE(c.cpp_value_text, '')) <> ''
	  AND UPPER(TRIM(c.cpp_value_text)) = $2
	ORDER BY m.cpm_product_code, c.cpp_value_id`

// supersedeOrionSharedQuery: another non-deleted spin carries the same ORION code
// (legacy duplicates, 000494) — text-only rows are then ambiguous and never repointed.
const supersedeOrionSharedQuery = `
	SELECT EXISTS (
		SELECT 1 FROM mst_mb_spin x
		WHERE x.deleted_at IS NULL AND x.mbs_id <> $1
		  AND UPPER(TRIM(x.mbs_orion_item_code)) = $2)`

// supersedeTextKey is the UPPER(TRIM(...)) key text-only rows are matched on:
// COALESCE(ORION code, CAST(mbs_id AS TEXT)) with a blank ORION code treated as NULL.
func supersedeTextKey(c supersedeSpin) (key string, fromOrion bool) {
	if c.OrionItemCode != nil {
		if v := strings.TrimSpace(*c.OrionItemCode); v != "" {
			return strings.ToUpper(v), true
		}
	}
	return strings.ToUpper(c.SpinID.String()), false
}

// supersedeParamRows is the result of listSupersedeParamRowsOn.
type supersedeParamRows struct {
	Pointer []supersedeParamRow
	// TextOnly is empty when TextShared is true.
	TextOnly []supersedeParamRow
	// TextShared: the ORION code is carried by another non-deleted spin; SharedTextRows
	// counts the text-only rows skipped because of it.
	TextShared     bool
	SharedTextRows int
}

// listSupersedeParamRowsOn lists the MB_SP_CODE rows that reference candidate c (locked
// products included — the caller decides). forUpdate locks the cpp rows (FOR UPDATE OF c)
// and must only be used inside a transaction.
func listSupersedeParamRowsOn(ctx context.Context, q supersedeQuerier, c supersedeSpin, forUpdate bool) (*supersedeParamRows, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF c"
	}
	out := &supersedeParamRows{}
	var err error
	if out.Pointer, err = querySupersedeParamRows(ctx, q, supersedePointerRowsQuery+lock, c.SpinID, mbspin.SupersedeRepointPointer); err != nil {
		return nil, err
	}
	key, fromOrion := supersedeTextKey(c)
	text, err := querySupersedeParamRows(ctx, q, supersedeTextRowsQuery+lock, key, mbspin.SupersedeRepointTextOnly)
	if err != nil {
		return nil, err
	}
	if fromOrion && len(text) > 0 {
		if err := q.QueryRowContext(ctx, supersedeOrionSharedQuery, c.SpinID, key).Scan(&out.TextShared); err != nil {
			return nil, fmt.Errorf("check shared orion code: %w", err)
		}
	}
	if out.TextShared {
		out.SharedTextRows = len(text)
		return out, nil
	}
	out.TextOnly = text
	return out, nil
}

func querySupersedeParamRows(ctx context.Context, q supersedeQuerier, query string, arg any, kind string) ([]supersedeParamRow, error) {
	rows, err := q.QueryContext(ctx, query, mbsource.ParamSPCode, arg)
	if err != nil {
		return nil, fmt.Errorf("list supersede %s rows: %w", strings.ToLower(kind), err)
	}
	defer closeRows(rows)
	var out []supersedeParamRow
	for rows.Next() {
		var (
			r    supersedeParamRow
			text sql.NullString
		)
		if err := rows.Scan(&r.CppValueID, &r.ProductSysID, &r.ProductCode, &r.ProductName, &text, &r.FilledBy, &r.IsLocked); err != nil {
			return nil, fmt.Errorf("scan supersede param row: %w", err)
		}
		r.ValueText = nullableStringPtr(text)
		r.Kind = kind
		out = append(out, r)
	}
	return out, rows.Err()
}

// supersedeHeadWillDeactivateOn mirrors design §2.5 step 3: the head is active and has no
// live + active + not-superseded spin other than the excluded (candidate) spins.
func supersedeHeadWillDeactivateOn(ctx context.Context, q supersedeQuerier, headID uuid.UUID, exclude []string) (bool, error) {
	var will bool
	err := q.QueryRowContext(ctx, `
		SELECT h.mbh_is_active AND NOT EXISTS (
			SELECT 1 FROM mst_mb_spin s
			WHERE s.mbs_mbh_id = h.mbh_id AND s.deleted_at IS NULL AND s.mbs_is_active
			  AND s.mbs_superseded_by_spin_id IS NULL
			  AND NOT (s.mbs_id = ANY(CAST($2 AS uuid[]))))
		FROM mst_mb_head h WHERE h.mbh_id = $1`, headID, pq.Array(exclude)).Scan(&will)
	if err != nil {
		return false, fmt.Errorf("check old head deactivation: %w", err)
	}
	return will, nil
}

// supersedeProductWillDeactivateOn mirrors design §2.5 step 4: the old MB product is active
// and no live + active + not-superseded spin other than the excluded ones points at it
// through mbs_cost_product_id. productSysID <= 0 → false.
func supersedeProductWillDeactivateOn(ctx context.Context, q supersedeQuerier, productSysID int64, exclude []string) (bool, error) {
	if productSysID <= 0 {
		return false, nil
	}
	var will bool
	err := q.QueryRowContext(ctx, `
		SELECT m.cpm_is_active AND NOT EXISTS (
			SELECT 1 FROM mst_mb_spin s
			WHERE s.mbs_cost_product_id = m.cpm_product_sys_id AND s.deleted_at IS NULL AND s.mbs_is_active
			  AND s.mbs_superseded_by_spin_id IS NULL
			  AND NOT (s.mbs_id = ANY(CAST($2 AS uuid[]))))
		FROM cost_product_master m WHERE m.cpm_product_sys_id = $1`, productSysID, pq.Array(exclude)).Scan(&will)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check old mb product deactivation: %w", err)
	}
	return will, nil
}

// PreviewSupersede implements mbspin.SupersedeReader (read-only, no locks).
func (r *MBSupersedeRepository) PreviewSupersede(ctx context.Context, newHeadID uuid.UUID, shadeCode string, fgProductID int64) ([]mbspin.SupersedeCandidate, error) {
	return buildSupersedePreviewOn(ctx, r.db, newHeadID, shadeCode, fgProductID, false)
}

// buildSupersedePreviewOn finds the candidates and computes, per candidate, the
// will-deactivate flags and the repoint counts. forUpdate locks candidate spins and their
// MB_SP_CODE rows (transaction only).
func buildSupersedePreviewOn(ctx context.Context, q supersedeQuerier, newHeadID uuid.UUID, shadeCode string, fgProductID int64, forUpdate bool) ([]mbspin.SupersedeCandidate, error) {
	spins, err := findSupersedeCandidatesOn(ctx, q, newHeadID, shadeCode, fgProductID, forUpdate)
	if err != nil || len(spins) == 0 {
		return nil, err
	}
	exclude := make([]string, 0, len(spins))
	for _, s := range spins {
		exclude = append(exclude, s.SpinID.String())
	}
	out := make([]mbspin.SupersedeCandidate, 0, len(spins))
	for _, s := range spins {
		cand, err := buildSupersedeCandidateOn(ctx, q, s, exclude, forUpdate)
		if err != nil {
			return nil, err
		}
		out = append(out, cand)
	}
	return out, nil
}

func buildSupersedeCandidateOn(ctx context.Context, q supersedeQuerier, s supersedeSpin, exclude []string, forUpdate bool) (mbspin.SupersedeCandidate, error) {
	cand := mbspin.SupersedeCandidate{
		OldSpinID: s.SpinID, OldSpinCode: s.SpinCode, ShadeCode: s.ShadeCode,
		FgProductID: s.FgProductID, FgProductCode: s.FgProductCode,
		OldHeadID: s.HeadID, OldHeadCode: s.HeadCode, OldMBProductSysID: s.MBProductID,
	}
	var err error
	if cand.OldHeadWillDeactivate, err = supersedeHeadWillDeactivateOn(ctx, q, s.HeadID, exclude); err != nil {
		return cand, err
	}
	if cand.OldMBProductWillDeactivate, err = supersedeProductWillDeactivateOn(ctx, q, s.MBProductID, exclude); err != nil {
		return cand, err
	}
	rows, err := listSupersedeParamRowsOn(ctx, q, s, forUpdate)
	if err != nil {
		return cand, err
	}
	applySupersedeParamRows(&cand, rows)
	return cand, nil
}

// applySupersedeParamRows fills counts, affected products and warnings. Locked products
// (cpm_is_locked) are skipped and reported; a shared ORION code skips all text-only rows.
func applySupersedeParamRows(cand *mbspin.SupersedeCandidate, rows *supersedeParamRows) {
	byFilled := map[string]int{}
	locked := 0
	for _, list := range [][]supersedeParamRow{rows.Pointer, rows.TextOnly} {
		for _, r := range list {
			if r.IsLocked {
				locked++
				continue
			}
			if r.Kind == mbspin.SupersedeRepointPointer {
				cand.ParamRowsPointer++
			} else {
				cand.ParamRowsTextOnly++
			}
			byFilled[r.FilledBy]++
			cand.AffectedProducts = append(cand.AffectedProducts, mbspin.SupersedeAffectedProduct{
				ProductSysID: r.ProductSysID, ProductCode: r.ProductCode, ProductName: r.ProductName,
				FilledBy: r.FilledBy, RepointKind: r.Kind,
			})
		}
	}
	cand.FilledByCounts = sortedFilledByCounts(byFilled)
	if rows.TextShared {
		cand.Warnings = append(cand.Warnings, fmt.Sprintf(
			"%d text-only MB_SP_CODE row(s) not repointed: ORION code %s is shared by more than one spin",
			rows.SharedTextRows, cand.OldSpinCode))
	}
	if locked > 0 {
		cand.Warnings = append(cand.Warnings, fmt.Sprintf(
			"%d MB_SP_CODE row(s) on locked products not repointed", locked))
	}
}

func sortedFilledByCounts(m map[string]int) []mbspin.SupersedeFilledByCount {
	out := make([]mbspin.SupersedeFilledByCount, 0, len(m))
	for k, v := range m {
		out = append(out, mbspin.SupersedeFilledByCount{FilledBy: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FilledBy < out[j].FilledBy })
	return out
}
