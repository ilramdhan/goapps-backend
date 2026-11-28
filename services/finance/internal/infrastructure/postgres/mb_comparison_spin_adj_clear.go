package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
)

// MBComparisonSpinAdjClear is the DB adapter of `mb-comparison-fill spin-adj-clear`. It writes through
// the same domain SetActualLDR + updateOn as spin-ldr and reverts through the spin-ldr Revert.
type MBComparisonSpinAdjClear struct {
	db  *DB
	ldr *MBComparisonSpinLDR
}

var _ fill.AdjStore = (*MBComparisonSpinAdjClear)(nil)

// NewMBComparisonSpinAdjClear creates the adapter.
func NewMBComparisonSpinAdjClear(db *DB) *MBComparisonSpinAdjClear {
	return &MBComparisonSpinAdjClear{db: db, ldr: NewMBComparisonSpinLDR(db)}
}

// adjScopeSQL: live spins (active or not) whose LDR group is not a clean ACTUAL.
const adjScopeSQL = `
	SELECT mbs_id, COALESCE(mbs_orion_item_code, ''), mbs_run_ldr_pct, mbs_ldr_calculated_pct, mbs_ldr_adjustment_pct,
	       COALESCE(mbs_ldr_type, ''), COALESCE(mbs_ldr_is_actual, FALSE)
	  FROM mst_mb_spin
	 WHERE deleted_at IS NULL
	   AND (mbs_ldr_adjustment_pct IS NOT NULL
	     OR COALESCE(mbs_ldr_is_actual, FALSE) = FALSE
	     OR COALESCE(mbs_ldr_type, '') <> 'ACTUAL'
	     OR ROUND(mbs_ldr_calculated_pct, 4) IS DISTINCT FROM ROUND(mbs_run_ldr_pct, 4))
	 ORDER BY mbs_id`

// Candidates reads the scope in a read-only tx.
func (m *MBComparisonSpinAdjClear) Candidates(ctx context.Context) (out []fill.AdjRow, err error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	rows, err := tx.QueryContext(ctx, adjScopeSQL)
	if err != nil {
		return nil, fmt.Errorf("list spins: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cErr)
		}
	}()
	for rows.Next() {
		var r fill.AdjRow
		var run, calc, adj sql.NullFloat64
		if err = rows.Scan(&r.ID, &r.Code, &run, &calc, &adj, &r.LDRType, &r.IsActual); err != nil {
			return nil, fmt.Errorf("scan spin: %w", err)
		}
		r.RunLDR, r.Calc, r.Adj = nullF(run), nullF(calc), nullF(adj)
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullF(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	return &n.Float64
}

// adjGuard returns a reason when the locked live LDR group differs from the plan snapshot.
func adjGuard(r fill.AdjRow, s mbasSpinImage) string {
	if !eqF4Ptr(s.RunLDRPct, r.RunLDR) || !eqF4Ptr(s.LDRCalculatedPct, r.Calc) || !eqF4Ptr(s.LDRAdjustmentPct, r.Adj) ||
		s.LDRType != r.LDRType || s.LDRIsActual != r.IsActual {
		return reasonChangedSincePlan
	}
	return ""
}

func adjRowOf(s mbasSpinImage, r fill.AdjRow) fill.AdjRow {
	r.RunLDR, r.Calc, r.Adj, r.LDRType, r.IsActual = s.RunLDRPct, s.LDRCalculatedPct, s.LDRAdjustmentPct, s.LDRType, s.LDRIsActual
	return r
}

// ApplyBatch writes the rows in one tx (savepoint per row). commit=false rolls the tx back.
func (m *MBComparisonSpinAdjClear) ApplyBatch(ctx context.Context, actor string, rows []fill.AdjRow, commit bool) (out []fill.AdjResult, err error) {
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
			return nil, rerr
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

func (m *MBComparisonSpinAdjClear) applyRowSP(ctx context.Context, tx *sql.Tx, actor string, r fill.AdjRow, okRes string) (fill.AdjResult, error) {
	rr := fill.AdjResult{AdjRow: r, Result: okRes}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT adj_sp"); err != nil {
		return rr, fmt.Errorf("savepoint: %w", err)
	}
	before, after, reason, werr := m.applyRow(ctx, tx, actor, r)
	if werr != nil && reason == "" {
		reason = werr.Error()
	}
	switch {
	case reason == reasonAlreadyTarget:
		rr.Result, rr.Detail = fill.ResultAlreadyApplied, reason
	case reason != "":
		rr.Result, rr.Detail = fill.ResultFailed, reason
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT adj_sp"); err != nil {
			return rr, fmt.Errorf("rollback to savepoint: %w", err)
		}
	default:
		rr.Before, rr.After = before, after
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT adj_sp"); err != nil {
		return rr, fmt.Errorf("release savepoint: %w", err)
	}
	return rr, nil
}

func (m *MBComparisonSpinAdjClear) applyRow(ctx context.Context, tx *sql.Tx, actor string, r fill.AdjRow) (before, after, reason string, err error) {
	id, perr := uuid.Parse(r.ID)
	if perr != nil {
		return "", "", "bad_spin_id", nil //nolint:nilerr // reported as a FAILED row
	}
	img, ok, err := readMBASImage(ctx, tx, id, true) // FOR UPDATE
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return "", "", reasonSpinNotFound, nil
	}
	cur := adjRowOf(img.Spin, r)
	if fill.AdjClean(cur) && eqF4Ptr(cur.RunLDR, &r.Target) {
		return "", "", reasonAlreadyTarget, nil
	}
	if reason = adjGuard(r, img.Spin); reason != "" {
		return "", "", reason, nil
	}
	entity, err := loadSpinForRepointOn(ctx, tx, id)
	if err != nil {
		return "", "", "", err
	}
	if serr := entity.SetActualLDR(r.Target, actor); serr != nil {
		return "", "", "set_actual_ldr: " + serr.Error(), nil //nolint:nilerr // reported as a FAILED row
	}
	if err = m.ldr.spin.updateOn(ctx, tx, entity); err != nil {
		return "", "", "", err
	}
	post, _, err := readMBASImage(ctx, tx, id, false)
	if err != nil || post == nil {
		return "", "", "", errors.Join(err, errors.New("re-read after write"))
	}
	return img.json(), post.json(), "", nil
}

// Revert restores the LDR group where the spin still equals the after-image (spin-ldr logic).
func (m *MBComparisonSpinAdjClear) Revert(ctx context.Context, actor string, r fill.AdjResult) (fill.AdjResult, error) {
	res, err := m.ldr.Revert(ctx, actor, fill.SpinResult{SpinRow: fill.SpinRow{ID: r.ID}, Before: r.Before, After: r.After})
	if err != nil {
		return r, err
	}
	r.Result, r.Detail = res.Result, res.Detail
	return r, nil
}
