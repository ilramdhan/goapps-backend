package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// MBLegacyFillWriter applies APPLY rows: one transaction per head (head rows
// first, then the rows of its spins), a SAVEPOINT per row, optimistic guard
// re-checked under FOR UPDATE. Classes are pluggable via SetClassHandler.
type MBLegacyFillWriter struct {
	db       *DB
	handlers map[mblegacyfill.Class]lfClassHandler
}

var _ mblegacyfill.Writer = (*MBLegacyFillWriter)(nil)

// lfOutcome is what a class handler reports for one row.
type lfOutcome struct {
	before, after map[string]string
	skipReason    string // non-empty: row skipped (e.g. already_in_target)
	rejectReason  string // non-empty: row REJECTed (uniqueness clash); nothing written
}

// lfClassHandler writes one row inside the head tx (after the guard passed).
type lfClassHandler func(ctx context.Context, tx *sql.Tx, g *lfGroup, row mblegacyfill.PlanRow, cur string) (lfOutcome, error)

// NewMBLegacyFillWriter creates the writer with classes L and S; D is a stub (T4).
func NewMBLegacyFillWriter(db *DB) *MBLegacyFillWriter {
	return &MBLegacyFillWriter{db: db, handlers: map[mblegacyfill.Class]lfClassHandler{
		mblegacyfill.ClassL: lfHandleL,
		mblegacyfill.ClassS: lfHandleS,
		mblegacyfill.ClassD: lfHandleD,
	}}
}

// SetClassHandler plugs a handler for a class (used by class D).
func (w *MBLegacyFillWriter) SetClassHandler(c mblegacyfill.Class, h lfClassHandler) {
	w.handlers[c] = h
}

// lfGroup is the set of rows applied in one transaction.
type lfGroup struct {
	actor string
	rows  []mblegacyfill.PlanRow // head rows first
	idx   []int                  // positions in the input slice
	// pre holds spin guard values read (and locked) before the head cascade, so
	// the optimistic guard of spin rows is not fooled by the cascade itself.
	pre map[string]string
	// dApplied marks that a head class-D column was written (cascade needed).
	dApplied bool
	// headChanges holds before/after of each head class-D column applied in this tx
	// (drives the targeted cascade).
	headChanges map[string]lfChange
}

func lfPreKey(r mblegacyfill.Row) string {
	return string(r.Entity) + "|" + strings.ToLower(r.ID) + "|" + r.Spec.Column
}

// Apply implements mblegacyfill.Writer. Results are returned in input order
// (APPLY rows only).
func (w *MBLegacyFillWriter) Apply(ctx context.Context, actor string, rows []mblegacyfill.PlanRow) ([]mblegacyfill.RowResult, error) {
	var apply []mblegacyfill.PlanRow
	for _, r := range rows {
		if r.Decision == mblegacyfill.DecisionApply {
			apply = append(apply, r)
		}
	}
	results := make([]mblegacyfill.RowResult, len(apply))
	groups, err := w.buildGroups(ctx, actor, apply, results)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		for i, rr := range w.applyGroup(ctx, g) {
			results[g.idx[i]] = rr
		}
	}
	return results, nil
}

func lfFail(r mblegacyfill.PlanRow, reason string) mblegacyfill.RowResult {
	return mblegacyfill.RowResult{Row: r.Row, Decision: string(r.Decision), Result: mblegacyfill.ResultFailed, Reason: reason}
}

// buildGroups resolves spin->head and orders rows head-first per head.
func (w *MBLegacyFillWriter) buildGroups(ctx context.Context, actor string, apply []mblegacyfill.PlanRow, results []mblegacyfill.RowResult) ([]*lfGroup, error) {
	spinHead, err := w.spinHeads(ctx, apply)
	if err != nil {
		return nil, err
	}
	type placed struct {
		i int
		r mblegacyfill.PlanRow
	}
	perHead := map[string][2][]placed{} // [0]=head rows, [1]=spin rows
	var keys []string
	for i, r := range apply {
		hid := strings.ToLower(r.Row.ID)
		if r.Row.Entity == mblegacyfill.EntitySpin {
			var ok bool
			if hid, ok = spinHead[strings.ToLower(r.Row.ID)]; !ok {
				results[i] = lfFail(r, mblegacyfill.ReasonNotFound)
				continue
			}
		} else if _, err := uuid.Parse(r.Row.ID); err != nil {
			results[i] = lfFail(r, "invalid id")
			continue
		}
		if _, seen := perHead[hid]; !seen {
			keys = append(keys, hid)
		}
		p := perHead[hid]
		k := 0
		if r.Row.Entity == mblegacyfill.EntitySpin {
			k = 1
		}
		p[k] = append(p[k], placed{i, r})
		perHead[hid] = p
	}
	order := make([]*lfGroup, 0, len(keys))
	for _, hid := range keys {
		g := &lfGroup{actor: actor}
		for _, k := range []int{0, 1} {
			for _, p := range perHead[hid][k] {
				g.rows = append(g.rows, p.r)
				g.idx = append(g.idx, p.i)
			}
		}
		order = append(order, g)
	}
	return order, nil
}

func (w *MBLegacyFillWriter) spinHeads(ctx context.Context, apply []mblegacyfill.PlanRow) (m map[string]string, err error) {
	var ids []string
	for _, r := range apply {
		if r.Row.Entity != mblegacyfill.EntitySpin {
			continue
		}
		if _, perr := uuid.Parse(r.Row.ID); perr == nil {
			ids = append(ids, r.Row.ID)
		}
	}
	m = map[string]string{}
	if len(ids) == 0 {
		return m, nil
	}
	rs, err := w.db.QueryContext(ctx, `SELECT CAST(mbs_id AS text), CAST(mbs_mbh_id AS text) FROM mst_mb_spin
		WHERE mbs_id = ANY(CAST($1 AS uuid[])) AND deleted_at IS NULL`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("resolve spin heads: %w", err)
	}
	defer closeLFRows(rs, &err)
	for rs.Next() {
		var s, h string
		if err = rs.Scan(&s, &h); err != nil {
			return nil, fmt.Errorf("scan spin head: %w", err)
		}
		m[strings.ToLower(s)] = strings.ToLower(h)
	}
	return m, rs.Err()
}

// SpinHeads resolves the head id (lower-case) of each live spin among the rows' spin ids.
// The CLI uses it to keep a head and all its spin rows in the same batch.
func (w *MBLegacyFillWriter) SpinHeads(ctx context.Context, rows []mblegacyfill.PlanRow) (map[string]string, error) {
	return w.spinHeads(ctx, rows)
}

// applyGroup runs one head tx; a tx-level error fails every row not yet decided.
func (w *MBLegacyFillWriter) applyGroup(ctx context.Context, g *lfGroup) []mblegacyfill.RowResult {
	out := make([]mblegacyfill.RowResult, len(g.rows))
	failAll := func(reason string) []mblegacyfill.RowResult {
		for i, r := range g.rows {
			out[i] = lfFail(r, reason)
		}
		return out
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return failAll("begin tx: " + err.Error())
	}
	if err := w.applyRows(ctx, tx, g, out); err != nil {
		msg := err.Error()
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			msg += "; rollback: " + rbErr.Error()
		}
		return failAll(msg)
	}
	if err := tx.Commit(); err != nil {
		return failAll("commit: " + err.Error())
	}
	return out
}

const lfSavepoint = "lf_row"

func (w *MBLegacyFillWriter) applyRow(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow) mblegacyfill.RowResult {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+lfSavepoint); err != nil {
		return lfFail(r, "savepoint: "+err.Error())
	}
	res, err := w.applyRowInner(ctx, tx, g, r)
	if err != nil {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+lfSavepoint); rbErr != nil {
			return lfFail(r, "rollback savepoint: "+rbErr.Error())
		}
		return lfFail(r, err.Error())
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+lfSavepoint); err != nil {
		return lfFail(r, "release savepoint: "+err.Error())
	}
	return res
}

func (w *MBLegacyFillWriter) applyRowInner(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow) (mblegacyfill.RowResult, error) {
	res := mblegacyfill.RowResult{Row: r.Row, Decision: string(r.Decision)}
	cur, found, err := lfGuardCurrent(ctx, tx, g, r.Row)
	if err != nil {
		return res, err
	}
	spec := r.Row.Spec
	switch {
	case !found:
		return lfFail(r, mblegacyfill.ReasonNotFound), nil
	case mblegacyfill.Equal(spec.Kind, cur, r.Row.LegacyValue):
		return lfSkip(res, mblegacyfill.DecisionAlready, mblegacyfill.ReasonAlreadyInTarget, cur), nil
	case !mblegacyfill.Equal(spec.Kind, cur, r.Row.GoappsValue):
		return lfSkip(res, mblegacyfill.DecisionChanged, mblegacyfill.ReasonChangedSince, cur), nil
	}
	h, ok := w.handlers[r.Class]
	if !ok {
		return res, fmt.Errorf("no handler for class %s", r.Class)
	}
	out, err := h(ctx, tx, g, r, cur)
	if err != nil {
		return res, err
	}
	if out.rejectReason != "" {
		res.Decision, res.Result, res.Reason = string(mblegacyfill.DecisionReject), mblegacyfill.ResultSkipped, out.rejectReason
		return res, nil
	}
	if r.Class == mblegacyfill.ClassD && r.Row.Entity == mblegacyfill.EntityHead && out.skipReason == "" {
		g.dApplied = true
		if g.headChanges == nil {
			g.headChanges = map[string]lfChange{}
		}
		col := r.Row.Spec.Column
		g.headChanges[col] = lfChange{before: out.before[col], after: out.after[col]}
	}
	if out.skipReason != "" {
		res.Result, res.Reason = mblegacyfill.ResultSkipped, out.skipReason
		return res, nil
	}
	res.Result, res.Reason = mblegacyfill.ResultApplied, ""
	res.BeforeJSON, res.AfterJSON = lfJSON(out.before), lfJSON(out.after)
	return res, nil
}

func lfSkip(res mblegacyfill.RowResult, d mblegacyfill.Decision, reason, cur string) mblegacyfill.RowResult {
	res.Decision, res.Result, res.Reason = string(d), mblegacyfill.ResultSkipped, reason
	res.BeforeJSON = lfJSON(map[string]string{res.Row.Spec.Column: mblegacyfill.Normalize(cur)})
	return res
}

func lfJSON(m map[string]string) string {
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// lfGuardCurrent returns the pre-cascade value for spin rows when captured, else locks and reads.
func lfGuardCurrent(ctx context.Context, tx *sql.Tx, g *lfGroup, row mblegacyfill.Row) (string, bool, error) {
	if v, ok := g.pre[lfPreKey(row)]; ok {
		return v, true, nil
	}
	return lfLockCurrent(ctx, tx, row)
}

// lfLockCurrent reads the current text value under FOR UPDATE.
func lfLockCurrent(ctx context.Context, tx *sql.Tx, row mblegacyfill.Row) (string, bool, error) {
	return lfReadColumn(ctx, tx, row.Entity, row.ID, row.Spec.Column, " FOR UPDATE")
}

func lfReadColumn(ctx context.Context, tx *sql.Tx, e mblegacyfill.Entity, id, col, suffix string) (string, bool, error) {
	t := lfTables[e]
	//nolint:gosec // identifiers come from the static registry
	q := "SELECT COALESCE(CAST(" + col + " AS text), '') FROM " + t.table +
		" WHERE " + t.idCol + " = CAST($1 AS uuid) AND deleted_at IS NULL" + suffix
	var v string
	err := tx.QueryRowContext(ctx, q, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", col, err)
	}
	return v, true, nil
}

// lfWriteColumn UPDATEs one column (+ audit) and returns the stored value.
func lfWriteColumn(ctx context.Context, tx *sql.Tx, g *lfGroup, e mblegacyfill.Entity, id string, spec mblegacyfill.FieldSpec, col, value string) error {
	t := lfTables[e]
	typ := map[mblegacyfill.Kind]string{mblegacyfill.KindText: "text", mblegacyfill.KindNum: "numeric", mblegacyfill.KindInt: "numeric"}[spec.Kind]
	expr := "CAST($2 AS " + typ + ")"
	if spec.Kind == mblegacyfill.KindInt {
		expr = "CAST(" + expr + " AS integer)"
	}
	//nolint:gosec // identifiers come from the static registry, values are bound
	q := "UPDATE " + t.table + " SET " + col + " = " + expr + ", updated_at = NOW(), updated_by = $3 WHERE " +
		t.idCol + " = CAST($1 AS uuid) AND deleted_at IS NULL"
	if _, err := tx.ExecContext(ctx, q, id, mblegacyfill.Normalize(value), g.actor); err != nil {
		return fmt.Errorf("update %s: %w", col, err)
	}
	return nil
}

// lfHandleL is the plain class-L write.
func lfHandleL(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, cur string) (lfOutcome, error) {
	spec := r.Row.Spec
	if err := lfWriteColumn(ctx, tx, g, r.Row.Entity, r.Row.ID, spec, spec.Column, r.Row.LegacyValue); err != nil {
		return lfOutcome{}, err
	}
	after, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, spec.Column, "")
	if err != nil {
		return lfOutcome{}, err
	}
	return lfOutcome{before: map[string]string{spec.Column: mblegacyfill.Normalize(cur)}, after: map[string]string{spec.Column: after}}, nil
}
