package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// MBComparisonFill reads the product snapshot and writes/reverts cost_product_parameter
// (+ cost_product_applicable_param) for the mb-comparison-fill tool.
type MBComparisonFill struct{ db *DB }

var _ fill.Writer = (*MBComparisonFill)(nil)

// NewMBComparisonFill creates the adapter.
func NewMBComparisonFill(db *DB) *MBComparisonFill { return &MBComparisonFill{db: db} }

func fillCodes() []string {
	cs := make([]string, len(fill.Params))
	for i, p := range fill.Params {
		cs[i] = p.Code
	}
	return cs
}

// Snapshot loads every ACTIVE product with an ERP item code, keyed by fill.ProductKey,
// with the current values / CAPP presence of the written params. Read-only.
func (m *MBComparisonFill) Snapshot(ctx context.Context) (res map[string][]fill.Product, err error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	rs, err := tx.QueryContext(ctx, `SELECT cpm_product_sys_id, cpm_erp_item_code, COALESCE(cpm_shade_code, ''), cpm_is_locked
		FROM cost_product_master WHERE cpm_is_active AND cpm_erp_item_code IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer closeRows(rs)
	byID := map[int64]*fill.Product{}
	res = map[string][]fill.Product{}
	var order []int64
	for rs.Next() {
		p := &fill.Product{Values: map[string]string{}, HasCAPP: map[string]bool{}}
		if err = rs.Scan(&p.SysID, &p.ItemCode, &p.Shade, &p.Locked); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		byID[p.SysID] = p
		order = append(order, p.SysID)
	}
	if err = finishRows(rs); err != nil {
		return nil, err
	}
	if err = snapshotParams(ctx, tx, byID); err != nil {
		return nil, err
	}
	for _, id := range order {
		p := byID[id]
		k := fill.ProductKey(p.ItemCode, p.Shade)
		res[k] = append(res[k], *p)
	}
	return res, nil
}

func finishRows(rs *sql.Rows) error {
	if err := rs.Err(); err != nil {
		return fmt.Errorf("iterate rows: %w", err)
	}
	return nil
}

func snapshotParams(ctx context.Context, tx *sql.Tx, byID map[int64]*fill.Product) error {
	rs, err := tx.QueryContext(ctx, `SELECT c.cpp_product_sys_id, p.param_code,
		COALESCE(CAST(c.cpp_value_numeric AS text), c.cpp_value_text, '')
		FROM cost_product_parameter c JOIN mst_parameter p ON p.id = c.cpp_param_id
		WHERE p.param_code = ANY($1)`, pq.Array(fillCodes()))
	if err != nil {
		return fmt.Errorf("query cpp: %w", err)
	}
	defer closeRows(rs)
	for rs.Next() {
		var id int64
		var code, v string
		if err = rs.Scan(&id, &code, &v); err != nil {
			return fmt.Errorf("scan cpp: %w", err)
		}
		if p := byID[id]; p != nil {
			p.Values[code] = v
		}
	}
	if err = finishRows(rs); err != nil {
		return err
	}
	if err = snapshotSpinPtr(ctx, tx, byID); err != nil {
		return err
	}
	rs, err = tx.QueryContext(ctx, `SELECT a.capp_product_sys_id, p.param_code
		FROM cost_product_applicable_param a JOIN mst_parameter p ON p.id = a.capp_param_id
		WHERE p.param_code = ANY($1)`, pq.Array(fillCodes()))
	if err != nil {
		return fmt.Errorf("query capp: %w", err)
	}
	defer closeRows(rs)
	for rs.Next() {
		var id int64
		var code string
		if err = rs.Scan(&id, &code); err != nil {
			return fmt.Errorf("scan capp: %w", err)
		}
		if p := byID[id]; p != nil {
			p.HasCAPP[code] = true
		}
	}
	return finishRows(rs)
}

func snapshotSpinPtr(ctx context.Context, tx *sql.Tx, byID map[int64]*fill.Product) error {
	rs, err := tx.QueryContext(ctx, `SELECT c.cpp_product_sys_id, CAST(c.cpp_value_mb_spin_id AS text)
		FROM cost_product_parameter c JOIN mst_parameter p ON p.id = c.cpp_param_id
		WHERE p.param_code = 'MB_SP_CODE' AND c.cpp_value_mb_spin_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("query spin pointers: %w", err)
	}
	defer closeRows(rs)
	for rs.Next() {
		var id int64
		var ptr string
		if err = rs.Scan(&id, &ptr); err != nil {
			return fmt.Errorf("scan spin pointer: %w", err)
		}
		if p := byID[id]; p != nil {
			p.SpinPtr = ptr
		}
	}
	return finishRows(rs)
}

// readSpinPtr returns the MB_SP_CODE cpp_value_mb_spin_id ("" = NULL / no row).
func readSpinPtr(ctx context.Context, tx *sql.Tx, pid int64, paramID string) (string, error) {
	var ptr sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT CAST(cpp_value_mb_spin_id AS text) FROM cost_product_parameter
		WHERE cpp_product_sys_id = $1 AND cpp_param_id = CAST($2 AS uuid)`, pid, paramID).Scan(&ptr)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read spin pointer: %w", err)
	}
	return ptr.String, nil
}

// Preflight requires the three formulas to carry the migration 000616 marker.
func (m *MBComparisonFill) Preflight(ctx context.Context) error {
	var n int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mst_formula
		WHERE formula_code = ANY($1) AND deleted_at IS NULL AND updated_by = $2`,
		pq.Array(fill.PreflightFormulas), fill.MigrationMarker).Scan(&n)
	if err != nil {
		return fmt.Errorf("preflight query: %w", err)
	}
	if n != len(fill.PreflightFormulas) {
		return fmt.Errorf("preflight: only %d/%d formulas (%s) have updated_by=%q: migration 000616 not applied or formulas edited since; aborting",
			n, len(fill.PreflightFormulas), strings.Join(fill.PreflightFormulas, ","), fill.MigrationMarker)
	}
	return nil
}

func paramIDs(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rs, err := tx.QueryContext(ctx, `SELECT param_code, CAST(id AS text) FROM mst_parameter
		WHERE param_code = ANY($1) AND deleted_at IS NULL`, pq.Array(fillCodes()))
	if err != nil {
		return nil, fmt.Errorf("query params: %w", err)
	}
	defer closeRows(rs)
	ids := map[string]string{}
	for rs.Next() {
		var c, id string
		if err = rs.Scan(&c, &id); err != nil {
			return nil, fmt.Errorf("scan param: %w", err)
		}
		ids[c] = id
	}
	if err = finishRows(rs); err != nil {
		return nil, err
	}
	for _, c := range fillCodes() {
		if ids[c] == "" {
			return nil, fmt.Errorf("mst_parameter %s missing (migration 000616 not applied?)", c)
		}
	}
	return ids, nil
}

// lockProduct locks an active, unlocked product; returns a reason when it is not writable.
func lockProduct(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	var active, locked bool
	err := tx.QueryRowContext(ctx, `SELECT cpm_is_active, cpm_is_locked FROM cost_product_master
		WHERE cpm_product_sys_id = $1 FOR UPDATE`, id).Scan(&active, &locked)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "product_not_found", nil
	case err != nil:
		return "", fmt.Errorf("lock product: %w", err)
	case !active:
		return "product_inactive", nil
	case locked:
		return "product_locked", nil
	}
	return "", nil
}

func readCPP(ctx context.Context, tx *sql.Tx, pid int64, paramID string) (string, bool, error) {
	var v string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(CAST(cpp_value_numeric AS text), cpp_value_text, '')
		FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = CAST($2 AS uuid)`, pid, paramID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read cpp: %w", err)
	}
	return v, true, nil
}

const (
	cppUpsertNum = `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		VALUES ($1, CAST($2 AS uuid), CAST($3 AS numeric), $4, $4)
		ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET cpp_value_numeric = EXCLUDED.cpp_value_numeric,
		cpp_filled_at = NOW(), cpp_filled_by = $4, cpp_updated_at = NOW(), cpp_updated_by = $4`
	cppUpsertText = `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
		VALUES ($1, CAST($2 AS uuid), $3, $4, $4)
		ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET cpp_value_text = EXCLUDED.cpp_value_text,
		cpp_filled_at = NOW(), cpp_filled_by = $4, cpp_updated_at = NOW(), cpp_updated_by = $4`
)

func writeCPP(ctx context.Context, tx *sql.Tx, actor string, pid int64, code, paramID, val string) error {
	q := cppUpsertText
	if k, _ := fill.ParamKind(code); k == mblegacyfill.KindNum {
		q = cppUpsertNum
	}
	if _, err := tx.ExecContext(ctx, q, pid, paramID, val, actor); err != nil {
		return fmt.Errorf("write %s: %w", code, err)
	}
	return nil
}

func sortedParamCodes(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ApplyBatch writes the rows in one tx (savepoint per row). commit=false rolls the whole tx back.
func (m *MBComparisonFill) ApplyBatch(ctx context.Context, actor string, rows []fill.PlanRow, commit bool) (out []fill.RowResult, err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	done := false
	defer func() {
		if !done {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
				err = fmt.Errorf("rollback: %w", rbErr)
			}
		}
	}()
	ids, err := paramIDs(ctx, tx)
	if err != nil {
		return nil, err
	}
	okRes := fill.ResultApplied
	if !commit {
		okRes = fill.ResultWouldApply
	}
	for _, r := range rows {
		rr, rerr := m.applyRowSP(ctx, tx, actor, r, ids, okRes)
		if rerr != nil {
			return nil, rerr
		}
		out = append(out, rr)
	}
	done = true
	if !commit {
		return out, rollbackIgnoringDone(tx)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

// applyRowSP runs applyRow inside a savepoint; a skipped/failed row is rolled back to it.
func (m *MBComparisonFill) applyRowSP(ctx context.Context, tx *sql.Tx, actor string, r fill.PlanRow, ids map[string]string, okRes string) (fill.RowResult, error) {
	rr := fill.RowResult{PlanRow: r, Result: okRes}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT row_sp"); err != nil {
		return rr, fmt.Errorf("savepoint: %w", err)
	}
	ins, reason, werr := m.applyRow(ctx, tx, actor, r, ids)
	if werr != nil && reason == "" {
		reason = werr.Error()
	}
	if reason != "" {
		rr.Result, rr.Detail = fill.ResultFailed, reason
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT row_sp"); err != nil {
			return rr, fmt.Errorf("rollback to savepoint: %w", err)
		}
	} else {
		rr.CAPPInserted = ins
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT row_sp"); err != nil {
		return rr, fmt.Errorf("release savepoint: %w", err)
	}
	return rr, nil
}

func rollbackIgnoringDone(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rollback dry-run: %w", err)
	}
	return nil
}

// applyRow returns the CAPP params inserted, or a non-empty reason when the row must be skipped (FAILED).
// guardRow is the optimistic check: every written param (and the spin pointer) still equals the plan's before-image.
func guardRow(ctx context.Context, tx *sql.Tx, r fill.PlanRow, ids map[string]string) (string, error) {
	for _, code := range sortedParamCodes(r.After) {
		cur, has, err := readCPP(ctx, tx, r.ProductID, ids[code])
		if err != nil {
			return "", err
		}
		k, _ := fill.ParamKind(code)
		if has != r.BeforeExisted[code] || !mblegacyfill.Equal(k, cur, r.Before[code]) {
			return "changed_since_plan:" + code, nil
		}
	}
	if _, ok := r.After["MB_SP_CODE"]; ok {
		ptr, err := readSpinPtr(ctx, tx, r.ProductID, ids["MB_SP_CODE"])
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(ptr, r.SpinPtr) {
			return "changed_since_plan:spin_ptr", nil
		}
	}
	return "", nil
}

func (m *MBComparisonFill) applyRow(ctx context.Context, tx *sql.Tx, actor string, r fill.PlanRow, ids map[string]string) (inserted []string, reason string, err error) {
	if reason, err = lockProduct(ctx, tx, r.ProductID); reason != "" || err != nil {
		return nil, reason, err
	}
	if reason, err = guardRow(ctx, tx, r, ids); reason != "" || err != nil {
		return nil, reason, err
	}
	_, codeChanges := r.After["MB_SP_CODE"]
	for _, code := range sortedParamCodes(r.After) {
		if err = writeCPP(ctx, tx, actor, r.ProductID, code, ids[code], r.After[code]); err != nil {
			return nil, "", err
		}
	}
	if codeChanges && r.SpinPtr != "" {
		// A lookup sync lets the pointer win over the code text: drop it so the new code is resolved.
		if _, err = tx.ExecContext(ctx, `UPDATE cost_product_parameter SET cpp_value_mb_spin_id = NULL
			WHERE cpp_product_sys_id = $1 AND cpp_param_id = CAST($2 AS uuid)`, r.ProductID, ids["MB_SP_CODE"]); err != nil {
			return nil, "", fmt.Errorf("clear spin pointer: %w", err)
		}
	}
	for _, code := range r.CAPP {
		var n int
		err = tx.QueryRowContext(ctx, `WITH i AS (INSERT INTO cost_product_applicable_param
				(capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
			VALUES ($1, CAST($2 AS uuid), FALSE, $3)
			ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING RETURNING 1) SELECT COUNT(*) FROM i`,
			r.ProductID, ids[code], actor).Scan(&n)
		if err != nil {
			return nil, "", fmt.Errorf("insert capp %s: %w", code, err)
		}
		if n == 1 {
			inserted = append(inserted, code)
		}
	}
	return inserted, "", nil
}

// Revert restores one APPLIED row in its own tx: a param is restored only while it still holds the
// after-image (before empty = CPP row absent before -> deleted); CAPP rows this run inserted are deleted
// unless their param changed since.
func (m *MBComparisonFill) Revert(ctx context.Context, actor string, r fill.RowResult) (res fill.RowResult, err error) {
	res = r
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return r, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	ids, err := paramIDs(ctx, tx)
	if err != nil {
		return r, err
	}
	reason, err := lockProduct(ctx, tx, r.ProductID)
	if err != nil {
		return r, err
	}
	if reason == "product_locked" {
		res.Result, res.Detail = fill.ResultFailed, reason
		return res, nil
	}
	notes, changed, restored, err := revertParams(ctx, tx, actor, r, ids)
	if err != nil {
		return r, err
	}
	if err = deleteInsertedCAPP(ctx, tx, r, ids, changed); err != nil {
		return r, err
	}
	if err = tx.Commit(); err != nil {
		return r, fmt.Errorf("commit: %w", err)
	}
	res.Detail = strings.Join(notes, ";")
	switch {
	case len(changed) > 0:
		res.Result = fill.ResultChanged
	case restored == 0 && len(r.After) > 0:
		res.Result = fill.ResultAlready
	default:
		res.Result = fill.ResultReverted
	}
	return res, nil
}

// deleteInsertedCAPP deletes the CAPP rows this run inserted, except those of params changed since apply.
func deleteInsertedCAPP(ctx context.Context, tx *sql.Tx, r fill.RowResult, ids map[string]string, changed map[string]bool) error {
	for _, code := range r.CAPPInserted {
		if changed[code] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM cost_product_applicable_param
			WHERE capp_product_sys_id = $1 AND capp_param_id = CAST($2 AS uuid)`, r.ProductID, ids[code]); err != nil {
			return fmt.Errorf("delete capp %s: %w", code, err)
		}
	}
	return nil
}

// revertParams restores each written param still holding its after-image.
func revertParams(ctx context.Context, tx *sql.Tx, actor string, r fill.RowResult, ids map[string]string) (notes []string, changed map[string]bool, restored int, err error) {
	changed = map[string]bool{}
	for _, code := range sortedParamCodes(r.After) {
		cur, has, rerr := readCPP(ctx, tx, r.ProductID, ids[code])
		if rerr != nil {
			return nil, nil, 0, rerr
		}
		k, _ := fill.ParamKind(code)
		switch {
		case has && mblegacyfill.Equal(k, cur, r.After[code]):
			if err = restoreCPP(ctx, tx, actor, r.ProductID, code, ids[code], r.Before[code], r.BeforeExisted[code]); err != nil {
				return nil, nil, 0, err
			}
			if err = restoreSpinPtr(ctx, tx, r, code, ids[code]); err != nil {
				return nil, nil, 0, err
			}
			restored++
		case (!has && !r.BeforeExisted[code]) || (has && mblegacyfill.Equal(k, cur, r.Before[code])):
			notes = append(notes, code+":already")
		default:
			changed[code] = true
			notes = append(notes, code+":changed_since_apply")
		}
	}
	return notes, changed, restored, nil
}

// restoreSpinPtr puts the cleared MB_SP_CODE pointer back (only if the spin still exists).
func restoreSpinPtr(ctx context.Context, tx *sql.Tx, r fill.RowResult, code, paramID string) error {
	if code != "MB_SP_CODE" || r.SpinPtr == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cost_product_parameter
		SET cpp_value_mb_spin_id = (SELECT mbs_id FROM mst_mb_spin WHERE mbs_id = CAST($3 AS uuid))
		WHERE cpp_product_sys_id = $1 AND cpp_param_id = CAST($2 AS uuid)`, r.ProductID, paramID, r.SpinPtr); err != nil {
		return fmt.Errorf("restore spin pointer: %w", err)
	}
	return nil
}

func restoreCPP(ctx context.Context, tx *sql.Tx, actor string, pid int64, code, paramID, before string, existed bool) error {
	if !existed {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cost_product_parameter
			WHERE cpp_product_sys_id = $1 AND cpp_param_id = CAST($2 AS uuid)`, pid, paramID); err != nil {
			return fmt.Errorf("delete cpp %s: %w", code, err)
		}
		return nil
	}
	return writeCPP(ctx, tx, actor, pid, code, paramID, before)
}
