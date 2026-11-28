package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
)

// MBComparisonSpinLDR is the DB adapter of `mb-comparison-fill spin-ldr`: it sets a spin LDR through
// the domain SetActualLDR + the existing spin updateOn, and reverts with the mbas image helpers.
type MBComparisonSpinLDR struct {
	db   *DB
	spin *MBSpinRepository
}

// reasonAlreadyTarget is the internal "nothing to do" signal of applyRow (not a failure).
const (
	reasonAlreadyTarget    = "already_target"
	reasonChangedSincePlan = "changed_since_plan"
	reasonSpinNotFound     = "spin_not_found"
)

var _ fill.SpinStore = (*MBComparisonSpinLDR)(nil)

// NewMBComparisonSpinLDR creates the adapter.
func NewMBComparisonSpinLDR(db *DB) *MBComparisonSpinLDR {
	return &MBComparisonSpinLDR{db: db, spin: NewMBSpinRepository(db)}
}

// Live reads the current LDR state of the spins (read-only, no lock).
func (m *MBComparisonSpinLDR) Live(ctx context.Context, ids []string) (res map[string]fill.SpinLive, err error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	res = make(map[string]fill.SpinLive, len(ids))
	for _, id := range ids {
		u, perr := uuid.Parse(id)
		if perr != nil {
			continue // not found -> REPORT
		}
		img, ok, rerr := readMBASImage(ctx, tx, u, false)
		if rerr != nil {
			return nil, rerr
		}
		if ok {
			res[id] = spinLiveOf(img.Spin)
		}
	}
	return res, nil
}

func spinLiveOf(s mbasSpinImage) fill.SpinLive {
	return fill.SpinLive{Found: true, RunLDR: s.RunLDRPct, Adj: s.LDRAdjustmentPct, IsActual: s.LDRIsActual}
}

// spinPreCheck returns a reason when the locked spin must not be written (or needs nothing).
func spinPreCheck(r fill.SpinRow, img *mbasImage) string {
	if img.Spin.superseded {
		return "spin_superseded"
	}
	if img.Head.deleted {
		return "head_deleted"
	}
	if l := spinLiveOf(img.Spin); l.Adj == nil && l.IsActual && l.RunLDR != nil && fill.Fix4(*l.RunLDR) == fill.Fix4(r.Target) {
		return reasonAlreadyTarget
	}
	return spinGuard(r, img.Spin)
}

// spinGuard returns a reason when the locked live spin differs from the plan's CSV snapshot.
func spinGuard(r fill.SpinRow, s mbasSpinImage) string {
	l := spinLiveOf(s)
	if !eqF4Ptr(l.RunLDR, r.RunLDR) || !eqF4Ptr(l.Adj, r.Adj) || l.IsActual != r.IsActual {
		return reasonChangedSincePlan
	}
	return ""
}

func eqF4Ptr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return fill.Fix4(*a) == fill.Fix4(*b)
}

// ApplyBatch writes the rows in one tx (savepoint per row). commit=false rolls the tx back.
func (m *MBComparisonSpinLDR) ApplyBatch(ctx context.Context, actor string, rows []fill.SpinRow, commit bool) (out []fill.SpinResult, err error) {
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

func (m *MBComparisonSpinLDR) applyRowSP(ctx context.Context, tx *sql.Tx, actor string, r fill.SpinRow, okRes string) (fill.SpinResult, error) {
	rr := fill.SpinResult{SpinRow: r, Result: okRes}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT spin_sp"); err != nil {
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
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT spin_sp"); err != nil {
			return rr, fmt.Errorf("rollback to savepoint: %w", err)
		}
	default:
		rr.Before, rr.After = before, after
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT spin_sp"); err != nil {
		return rr, fmt.Errorf("release savepoint: %w", err)
	}
	return rr, nil
}

func (m *MBComparisonSpinLDR) applyRow(ctx context.Context, tx *sql.Tx, actor string, r fill.SpinRow) (before, after, reason string, err error) {
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
	if reason = spinPreCheck(r, img); reason != "" {
		return "", "", reason, nil
	}
	entity, err := loadSpinForRepointOn(ctx, tx, id)
	if err != nil {
		return "", "", "", err
	}
	if serr := entity.SetActualLDR(r.Target, actor); serr != nil {
		return "", "", "set_actual_ldr: " + serr.Error(), nil //nolint:nilerr // reported as a FAILED row
	}
	if err = m.spin.updateOn(ctx, tx, entity); err != nil {
		return "", "", "", err
	}
	post, _, err := readMBASImage(ctx, tx, id, false)
	if err != nil || post == nil {
		return "", "", "", errors.Join(err, errors.New("re-read after write"))
	}
	return img.json(), post.json(), "", nil
}

// Revert restores the LDR group of one APPLIED row in its own tx, only while the live spin still equals the after-image.
func (m *MBComparisonSpinLDR) Revert(ctx context.Context, actor string, r fill.SpinResult) (res fill.SpinResult, err error) {
	res = r
	var before, after mbasImage
	if err = json.Unmarshal([]byte(r.Before), &before); err != nil {
		return r, fmt.Errorf("before_json: %w", err)
	}
	if err = json.Unmarshal([]byte(r.After), &after); err != nil {
		return r, fmt.Errorf("after_json: %w", err)
	}
	id, err := uuid.Parse(r.ID)
	if err != nil {
		return r, fmt.Errorf("spin id: %w", err)
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
	cur, ok, err := readMBASImage(ctx, tx, id, true)
	if err != nil {
		return r, err
	}
	if !ok {
		res.Result, res.Detail = fill.ResultFailed, reasonSpinNotFound
		return res, nil
	}
	switch {
	case mbasSpinEq(cur.Spin, before.Spin).ldr:
		res.Result = fill.ResultAlready
	case !mbasSpinEq(cur.Spin, after.Spin).ldr:
		res.Result = fill.ResultChanged
	default:
		if err = mbasRestoreSpin(ctx, tx, id, before.Spin, actor, mbasSpinGroups{ldr: true}); err != nil {
			return r, err
		}
		res.Result = fill.ResultReverted
	}
	if err = tx.Commit(); err != nil {
		return r, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}
