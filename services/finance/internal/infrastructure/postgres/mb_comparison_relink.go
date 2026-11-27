package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBComparisonRelink is the DB adapter of `mb-comparison-fill relink`. It only ever writes
// cost_product_parameter.cpp_value_mb_spin_id of MB_SP_CODE, plus child spins through DuplicateSpin
// (which owns its tx, so children are created in a separate step BEFORE the pointer batch).
type MBComparisonRelink struct {
	db   *DB
	spin *MBSpinRepository
}

var _ fill.RelinkStore = (*MBComparisonRelink)(nil)

// NewMBComparisonRelink creates the adapter.
func NewMBComparisonRelink(db *DB) *MBComparisonRelink {
	return &MBComparisonRelink{db: db, spin: NewMBSpinRepository(db)}
}

const (
	relinkSelPtr = `SELECT CAST(c.cpp_value_mb_spin_id AS text), COALESCE(c.cpp_value_text, '')
		FROM cost_product_parameter c JOIN mst_parameter p ON p.id = c.cpp_param_id
		WHERE c.cpp_product_sys_id = $1 AND p.param_code = 'MB_SP_CODE'`
)

func (m *MBComparisonRelink) readTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only tx: %w", err)
	}
	return tx, nil
}

// Products reads the MB_SP_CODE state of the products (read-only, no lock).
func (m *MBComparisonRelink) Products(ctx context.Context, ids []int64) (res map[int64]fill.RelinkProduct, err error) {
	tx, err := m.readTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	res = make(map[int64]fill.RelinkProduct, len(ids))
	for _, id := range ids {
		var p fill.RelinkProduct
		qerr := tx.QueryRowContext(ctx, `SELECT cpm_is_active, cpm_is_locked FROM cost_product_master WHERE cpm_product_sys_id = $1`, id).Scan(&p.Active, &p.Locked)
		if errors.Is(qerr, sql.ErrNoRows) {
			continue
		}
		if qerr != nil {
			return nil, fmt.Errorf("read product: %w", qerr)
		}
		p.Found = true
		var ptr sql.NullString
		switch qerr = tx.QueryRowContext(ctx, relinkSelPtr, id).Scan(&ptr, &p.Text); {
		case errors.Is(qerr, sql.ErrNoRows):
		case qerr != nil:
			return nil, fmt.Errorf("read pointer: %w", qerr)
		default:
			p.HasRow, p.Ptr = true, ptr.String
		}
		res[id] = p
	}
	return res, nil
}

// Spins reads the live (non-deleted) spins among ids.
func (m *MBComparisonRelink) Spins(ctx context.Context, ids []string) (map[string]fill.RelinkSpin, error) {
	var valid []string
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil {
			valid = append(valid, id)
		}
	}
	rs, err := m.db.QueryContext(ctx, `SELECT CAST(mbs_id AS text), COALESCE(CAST(mbs_parent_spin_id AS text), ''),
		COALESCE(mbs_denier, 0), COALESCE(mbs_filament, 0), COALESCE(mbs_run_ldr_pct, 0), COALESCE(mbs_ldr_is_actual, FALSE)
		FROM mst_mb_spin WHERE deleted_at IS NULL AND mbs_id = ANY(CAST($1 AS uuid[]))`, pq.Array(valid))
	if err != nil {
		return nil, fmt.Errorf("query spins: %w", err)
	}
	defer closeRows(rs)
	out := map[string]fill.RelinkSpin{}
	for rs.Next() {
		var id string
		var s fill.RelinkSpin
		if err = rs.Scan(&id, &s.Parent, &s.Den, &s.Fil, &s.LDR, &s.Actual); err != nil {
			return nil, fmt.Errorf("scan spin: %w", err)
		}
		out[id] = s
	}
	return out, finishRows(rs)
}

// EnsureChild reuses a live child of c.Parent with the same den/fil/LDR (ACTUAL), else creates it via DuplicateSpin.
func (m *MBComparisonRelink) EnsureChild(ctx context.Context, actor string, c fill.RelinkChildSpec, commit bool) (id string, created bool, reason string, err error) {
	pid, perr := uuid.Parse(c.Parent)
	if perr != nil {
		return "", false, "bad_parent_spin_id", nil //nolint:nilerr // reported as a FAILED row
	}
	var found sql.NullString
	err = m.db.QueryRowContext(ctx, `SELECT CAST(mbs_id AS text) FROM mst_mb_spin
		WHERE mbs_parent_spin_id = $1 AND deleted_at IS NULL AND mbs_is_active AND COALESCE(mbs_ldr_is_actual, FALSE)
		  AND mbs_filament = $2 AND mbs_denier = ROUND(CAST($3 AS numeric), 2) AND mbs_run_ldr_pct = ROUND(CAST($4 AS numeric), 4)
		ORDER BY created_at, mbs_id LIMIT 1`, pid, c.Fil, c.Den, c.LDR).Scan(&found)
	switch {
	case err == nil:
		return found.String, false, "", nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", false, "", fmt.Errorf("find child: %w", err)
	}
	if !commit {
		return "", true, "", nil
	}
	den, ldr, fil := c.Den, c.LDR, c.Fil
	out, derr := m.spin.DuplicateSpin(ctx, mbspin.DuplicateInput{
		SourceSpinID: pid, Denier: &den, Filament: &fil, RunLDRPct: &ldr, ActorUserID: actor, RequireLiveHead: false,
	})
	if derr != nil {
		return "", false, "duplicate_spin: " + derr.Error(), nil //nolint:nilerr // reported as a FAILED row
	}
	return out.NewSpinID.String(), true, "", nil
}

// ApplyBatch points the products in one tx (savepoint per row). commit=false rolls the tx back.
func (m *MBComparisonRelink) ApplyBatch(ctx context.Context, actor string, rows []fill.RelinkRow, commit bool) (out []fill.RelinkResult, err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	okRes := fill.ResultApplied
	if !commit {
		okRes = fill.ResultWouldApply
	}
	for _, r := range rows {
		rr, rerr := m.applyRowSP(ctx, tx, actor, r, okRes)
		if rerr != nil {
			return out, rerr
		}
		out = append(out, rr)
	}
	if !commit {
		return out, rollbackIgnoringDone(tx)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

func (m *MBComparisonRelink) applyRowSP(ctx context.Context, tx *sql.Tx, actor string, r fill.RelinkRow, okRes string) (fill.RelinkResult, error) {
	rr := fill.RelinkResult{Row: r.Row, Product: r.Product, Decision: r.Decision, Status: okRes}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT relink_sp"); err != nil {
		return rr, fmt.Errorf("savepoint: %w", err)
	}
	before, after, reason, werr := m.applyRow(ctx, tx, actor, r)
	if werr != nil && reason == "" {
		reason = werr.Error()
	}
	switch {
	case reason == reasonAlreadyTarget:
		rr.Status, rr.Reason = fill.ResultAlreadyApplied, reason
	case reason != "":
		rr.Status, rr.Reason = fill.ResultFailed, reason
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT relink_sp"); err != nil {
			return rr, fmt.Errorf("rollback to savepoint: %w", err)
		}
	default:
		rr.Before, rr.After = before, after
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT relink_sp"); err != nil {
		return rr, fmt.Errorf("release savepoint: %w", err)
	}
	return rr, nil
}

func (m *MBComparisonRelink) applyRow(ctx context.Context, tx *sql.Tx, actor string, r fill.RelinkRow) (before, after, reason string, err error) {
	if reason, err = lockProduct(ctx, tx, r.Product); reason != "" || err != nil {
		return "", "", reason, err
	}
	var ptr sql.NullString
	var text string
	// FOR UPDATE OF c: the CPP row is locked together with the product.
	qerr := tx.QueryRowContext(ctx, relinkSelPtr+` FOR UPDATE OF c`, r.Product).Scan(&ptr, &text)
	if errors.Is(qerr, sql.ErrNoRows) {
		return "", "", "no_mb_sp_code_row", nil
	}
	if qerr != nil {
		return "", "", "", fmt.Errorf("lock cpp: %w", qerr)
	}
	if r.Resolved != "" && strings.EqualFold(ptr.String, r.Resolved) {
		return "", "", reasonAlreadyTarget, nil
	}
	if !strings.EqualFold(ptr.String, r.SnapPtr) || text != r.SnapText {
		return "", "", "changed_since_plan", nil
	}
	if r.Resolved == "" { // dry-run child that would be created: guards passed, nothing to point at yet
		return EncodeImage(ptr.String, text), "", "", nil
	}
	var live bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mst_mb_spin WHERE mbs_id = CAST($1 AS uuid) AND deleted_at IS NULL)`, r.Resolved).Scan(&live); err != nil {
		return "", "", "", fmt.Errorf("check target: %w", err)
	}
	if !live {
		return "", "", "target_spin_missing_or_deleted", nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_product_parameter SET cpp_value_mb_spin_id = CAST($2 AS uuid), cpp_updated_at = NOW(), cpp_updated_by = $3
		WHERE cpp_product_sys_id = $1 AND cpp_param_id = (SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL)`, r.Product, r.Resolved, actor); err != nil {
		return "", "", "", fmt.Errorf("update pointer: %w", err)
	}
	return EncodeImage(ptr.String, text), EncodeImage(r.Resolved, text), "", nil
}

// EncodeImage renders the before/after JSON of a pointer row.
func EncodeImage(ptr, text string) string {
	return fill.EncodeRelinkImage(fill.RelinkImage{Ptr: ptr, Text: text})
}

// RevertPointer restores the before-pointer only while the live pointer still equals the after-pointer.
func (m *MBComparisonRelink) RevertPointer(ctx context.Context, actor string, r fill.RelinkResult) (res fill.RelinkResult, err error) {
	res = r
	var before, after fill.RelinkImage
	if err = errors.Join(json.Unmarshal([]byte(r.Before), &before), json.Unmarshal([]byte(r.After), &after)); err != nil {
		return r, fmt.Errorf("before/after json: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return r, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	if reason, lerr := lockProduct(ctx, tx, r.Product); lerr != nil || reason != "" {
		res.Status, res.Reason = fill.ResultFailed, reason
		return res, lerr
	}
	var ptr sql.NullString
	var text string
	qerr := tx.QueryRowContext(ctx, relinkSelPtr+` FOR UPDATE OF c`, r.Product).Scan(&ptr, &text)
	if errors.Is(qerr, sql.ErrNoRows) {
		res.Status, res.Reason = fill.ResultFailed, "no_mb_sp_code_row"
		return res, nil
	}
	if qerr != nil {
		return r, fmt.Errorf("lock cpp: %w", qerr)
	}
	switch {
	case strings.EqualFold(ptr.String, before.Ptr):
		res.Status = fill.ResultAlready
	case !strings.EqualFold(ptr.String, after.Ptr):
		res.Status = fill.ResultChanged
	default:
		if _, err = tx.ExecContext(ctx, `UPDATE cost_product_parameter SET cpp_value_mb_spin_id = CAST(NULLIF($2, '') AS uuid), cpp_updated_at = NOW(), cpp_updated_by = $3
			WHERE cpp_product_sys_id = $1 AND cpp_param_id = (SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL)`,
			r.Product, before.Ptr, actor); err != nil {
			return r, fmt.Errorf("restore pointer: %w", err)
		}
		res.Status = fill.ResultReverted
	}
	if err = tx.Commit(); err != nil {
		return r, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}

// DeleteChild soft-deletes a child this run created, unless a CPP row or a child spin still references it.
func (m *MBComparisonRelink) DeleteChild(ctx context.Context, actor, id string) (status, detail string, err error) {
	cid, perr := uuid.Parse(id)
	if perr != nil {
		return fill.ResultFailed, "bad child id", nil //nolint:nilerr // reported as a FAILED row
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	var dupBy sql.NullString
	switch qerr := tx.QueryRowContext(ctx, `SELECT mbs_duplicated_by FROM mst_mb_spin WHERE mbs_id = $1 AND deleted_at IS NULL FOR UPDATE`, cid).Scan(&dupBy); {
	case errors.Is(qerr, sql.ErrNoRows):
		return fill.ResultAlready, "child_already_deleted", nil
	case qerr != nil:
		return "", "", fmt.Errorf("lock child: %w", qerr)
	}
	if dupBy.String != actor {
		return fill.StatusKept, "not_created_by_this_actor", nil
	}
	var refs, kids int
	if err = tx.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM cost_product_parameter WHERE cpp_value_mb_spin_id = $1),
		(SELECT COUNT(*) FROM mst_mb_spin WHERE mbs_parent_spin_id = $1 AND deleted_at IS NULL)`, cid).Scan(&refs, &kids); err != nil {
		return "", "", fmt.Errorf("count references: %w", err)
	}
	if refs > 0 || kids > 0 {
		return fill.StatusKept, fmt.Sprintf("cpp_refs=%d child_spins=%d", refs, kids), nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE mst_mb_spin SET deleted_at = $2, deleted_by = $3, mbs_is_active = FALSE WHERE mbs_id = $1 AND deleted_at IS NULL`,
		cid, time.Now(), actor); err != nil {
		return "", "", fmt.Errorf("soft delete child: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return "", "", fmt.Errorf("commit: %w", err)
	}
	return fill.ResultReverted, "child_soft_deleted", nil
}
